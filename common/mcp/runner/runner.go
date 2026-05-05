// Package runner is the single entry point through which every LLM-initiated
// MCP tool call goes. Centralizing here means timeout, retry/backoff,
// observability and the contract for `tool_invocations` are written once and
// shared by both MCPModel and Agent.
//
// Design choices baked in:
//   - One `tool_invocations` row per ATTEMPT, sharing a single `tool_call_id`.
//     This way the dashboard can reconstruct a flaky call exactly as it
//     happened — first 502, then 502 again, then success — rather than
//     collapsing it into a single row that hides the retries.
//   - Persistence failures are logged but do NOT propagate. The point of the
//     ledger is observability; making it a hard dependency on every tool
//     call would let one MySQL hiccup break the demo.
//   - The runner refuses to look at args after they've been merged with
//     `_ctx`: it serializes them as the LLM sent them. Whoever wants to log
//     the injected context can read it from the trace_id column instead.
package runner

import (
	mcpconv "GopherAI/common/mcp"
	mcpclient "GopherAI/common/mcp/client"
	tooldao "GopherAI/dao/tool_invocation"
	"GopherAI/model"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

const (
	defaultTimeout     = 10 * time.Second
	defaultMaxAttempts = 3
	defaultBaseBackoff = 500 * time.Millisecond
	maxResultPreview   = 1900 // leaves headroom under the column's varchar(2000)
)

// Options configures one logical tool invocation. Fields left empty fall
// back to defaults that are reasonable for the demo and described above.
type Options struct {
	// ToolCallID is the LLM-issued ID that ties retries together. Falls back
	// to a synthetic one if empty.
	ToolCallID string
	// TraceID, ActiveSkills and ModelType are persisted alongside each
	// attempt for cross-cutting analysis. They are pure metadata; the runner
	// does not interpret them.
	TraceID      string
	ActiveSkills []string
	ModelType    string

	Timeout     time.Duration
	MaxAttempts int
	BaseBackoff time.Duration
}

func (o *Options) normalize() {
	if o.Timeout <= 0 {
		o.Timeout = defaultTimeout
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = defaultMaxAttempts
	}
	if o.BaseBackoff <= 0 {
		o.BaseBackoff = defaultBaseBackoff
	}
}

// Result is the structured outcome of a tool invocation. Callers usually
// only need Text for the LLM, but Status and Attempts are surfaced so the
// SSE layer (Step 7) can render the visual state without re-querying the DB.
type Result struct {
	Text     string
	Status   model.ToolInvocationStatus
	Attempts int
	LastErr  error
}

// Run executes a single LLM-issued tool call with timeout/retry semantics
// and writes one tool_invocations row per attempt. The returned Result.Text
// is what should be fed back to the LLM; on failure it contains a human-
// readable diagnostic string instead of an empty payload.
func Run(
	ctx context.Context,
	client *mcpclient.MCPClient,
	toolName string,
	args map[string]any,
	opts Options,
) Result {
	opts.normalize()

	tc, _ := mcpconv.ToolCtxFrom(ctx)
	argsJSON := encodeArgs(args)
	activeSkillsCSV := strings.Join(opts.ActiveSkills, ",")
	callID := opts.ToolCallID
	if callID == "" {
		callID = fmt.Sprintf("synthetic-%d", time.Now().UnixNano())
	}

	var lastErr error
	var lastStatus model.ToolInvocationStatus

	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		start := time.Now()
		result, err := client.CallTool(attemptCtx, toolName, args)
		duration := time.Since(start)
		cancel()

		status := classify(err, attemptCtx)
		text := ""
		if err == nil {
			text = mcpconv.ExtractToolResultText(result)
		}

		persistInvocation(record{
			TraceID:      opts.TraceID,
			UserName:     tc.UserName,
			SessionID:    tc.SessionID,
			ToolCallID:   callID,
			ToolName:     toolName,
			ArgsJSON:     argsJSON,
			Result:       text,
			Status:       status,
			Err:          err,
			Attempt:      attempt,
			MaxAttempts:  opts.MaxAttempts,
			DurationMs:   int(duration.Milliseconds()),
			ActiveSkills: activeSkillsCSV,
			ModelType:    opts.ModelType,
		})

		if status == model.ToolInvocationStatusSuccess {
			return Result{
				Text:     text,
				Status:   status,
				Attempts: attempt,
			}
		}

		lastErr = err
		lastStatus = status

		// Cancellation is the user/host pulling the plug — never retry.
		if status == model.ToolInvocationStatusCancelled {
			break
		}

		if attempt < opts.MaxAttempts {
			sleep := backoffDuration(opts.BaseBackoff, attempt)
			select {
			case <-time.After(sleep):
			case <-ctx.Done():
				lastErr = ctx.Err()
				lastStatus = model.ToolInvocationStatusCancelled
				return Result{
					Text:     formatFailure(toolName, lastStatus, lastErr, opts.MaxAttempts),
					Status:   lastStatus,
					Attempts: attempt,
					LastErr:  lastErr,
				}
			}
		}
	}

	return Result{
		Text:     formatFailure(toolName, lastStatus, lastErr, opts.MaxAttempts),
		Status:   lastStatus,
		Attempts: opts.MaxAttempts,
		LastErr:  lastErr,
	}
}

// classify decides which terminal status to record for an attempt. The order
// matters: a context cancellation that hit *before* the deadline shows up
// here as Cancelled; a timeout from the per-attempt context shows up as
// Timeout; everything else is bucketed as Error.
func classify(err error, attemptCtx context.Context) model.ToolInvocationStatus {
	if err == nil {
		return model.ToolInvocationStatusSuccess
	}
	if errors.Is(err, context.Canceled) {
		return model.ToolInvocationStatusCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
		return model.ToolInvocationStatusTimeout
	}
	return model.ToolInvocationStatusError
}

// backoffDuration grows exponentially (base, 2*base, 4*base, ...) and is
// capped at 5s to keep the demo responsive even when something goes wrong.
func backoffDuration(base time.Duration, attempt int) time.Duration {
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d > 5*time.Second {
			d = 5 * time.Second
			break
		}
	}
	return d
}

// formatFailure produces the textual payload returned to the LLM when every
// attempt failed. It is intentionally short and unambiguous so the LLM can
// either give up gracefully or pick a different tool.
func formatFailure(name string, status model.ToolInvocationStatus, err error, max int) string {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	switch status {
	case model.ToolInvocationStatusTimeout:
		return fmt.Sprintf("[tool %s 失败] 已重试 %d 次仍超时: %s", name, max, reason)
	case model.ToolInvocationStatusCancelled:
		return fmt.Sprintf("[tool %s 已取消] %s", name, reason)
	default:
		return fmt.Sprintf("[tool %s 失败] 已重试 %d 次仍报错: %s", name, max, reason)
	}
}

func encodeArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	// `_ctx` is implementation detail injected by mcpclient; logging it leaks
	// nothing useful but bloats the column, so drop it.
	stripped := make(map[string]any, len(args))
	for k, v := range args {
		if k == mcpconv.CtxArgKey {
			continue
		}
		stripped[k] = v
	}
	b, err := json.Marshal(stripped)
	if err != nil {
		return ""
	}
	return string(b)
}

type record struct {
	TraceID      string
	UserName     string
	SessionID    string
	ToolCallID   string
	ToolName     string
	ArgsJSON     string
	Result       string
	Status       model.ToolInvocationStatus
	Err          error
	Attempt      int
	MaxAttempts  int
	DurationMs   int
	ActiveSkills string
	ModelType    string
}

func persistInvocation(r record) {
	preview := r.Result
	if len(preview) > maxResultPreview {
		preview = preview[:maxResultPreview]
	}

	errMsg := ""
	if r.Err != nil {
		errMsg = r.Err.Error()
		if len(errMsg) > 1000 {
			errMsg = errMsg[:1000]
		}
	}

	row := &model.ToolInvocation{
		TraceID:       r.TraceID,
		UserName:      r.UserName,
		SessionID:     r.SessionID,
		ToolCallID:    r.ToolCallID,
		ToolName:      r.ToolName,
		ArgsJSON:      r.ArgsJSON,
		ArgsSize:      len(r.ArgsJSON),
		ResultPreview: preview,
		ResultSize:    len(r.Result),
		Status:        r.Status,
		ErrorMsg:      errMsg,
		Attempt:       r.Attempt,
		MaxAttempts:   r.MaxAttempts,
		DurationMs:    r.DurationMs,
		ActiveSkills:  r.ActiveSkills,
		ModelType:     r.ModelType,
	}

	if err := tooldao.Insert(row); err != nil {
		log.Printf("tool_invocation persist failed (tool=%s attempt=%d status=%s): %v",
			r.ToolName, r.Attempt, r.Status, err)
	}
}
