package mcpconv

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// CtxArgKey is the JSON key under which tool-call context is injected into
// the arguments map sent to MCP servers. The corresponding field is
// intentionally NOT declared in any tool's InputSchema so the LLM never sees
// or fills it. Server-side handlers that care about user/session identity
// extract it via ExtractToolCtx.
const CtxArgKey = "_ctx"

// ToolCtx is the per-call context that the MCP client implicitly forwards to
// the server alongside the user-facing arguments. It is the single channel
// through which user/session identity reaches a tool handler.
type ToolCtx struct {
	UserName  string `json:"user_name,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
}

// IsZero reports whether the ToolCtx carries no useful identity. Handlers can
// short-circuit user-bound logic when this is true rather than reading empty
// strings field by field.
func (c ToolCtx) IsZero() bool {
	return c.UserName == "" && c.SessionID == "" && c.TraceID == ""
}

// ToMap renders the ToolCtx into the shape expected inside CallTool arguments.
// Empty fields are dropped so the server receives only what was set.
func (c ToolCtx) ToMap() map[string]any {
	m := make(map[string]any, 3)
	if c.UserName != "" {
		m["user_name"] = c.UserName
	}
	if c.SessionID != "" {
		m["session_id"] = c.SessionID
	}
	if c.TraceID != "" {
		m["trace_id"] = c.TraceID
	}
	return m
}

type toolCtxKey struct{}

// WithToolCtx returns a new context.Context carrying the given ToolCtx. The
// MCP client reads it during CallTool to auto-inject `_ctx` into args.
func WithToolCtx(ctx context.Context, tc ToolCtx) context.Context {
	return context.WithValue(ctx, toolCtxKey{}, tc)
}

// ToolCtxFrom returns the ToolCtx attached to ctx, if any. The boolean is
// false when no ToolCtx was set; callers can decide whether to treat that as
// an error (production) or fall through with a zero value (debug paths).
func ToolCtxFrom(ctx context.Context) (ToolCtx, bool) {
	v, ok := ctx.Value(toolCtxKey{}).(ToolCtx)
	return v, ok
}

// InjectCtxIntoArgs merges the given ToolCtx into a copy of args under the
// reserved key CtxArgKey. The original map is not mutated. Pass an existing
// args map (possibly nil) to support both "first injection" and "re-injection
// with merge" semantics: if args already contains a `_ctx`, non-empty fields
// from `tc` overwrite, and empty fields preserve whatever was there.
func InjectCtxIntoArgs(args map[string]any, tc ToolCtx) map[string]any {
	out := make(map[string]any, len(args)+1)
	for k, v := range args {
		out[k] = v
	}
	merged := tc.ToMap()
	if existing, ok := out[CtxArgKey].(map[string]any); ok {
		for k, v := range existing {
			if _, has := merged[k]; !has {
				merged[k] = v
			}
		}
	}
	out[CtxArgKey] = merged
	return out
}

// ExtractToolCtx pulls the ToolCtx from a CallToolRequest. It tolerates the
// field being absent (returns a zero-value ToolCtx + false), being a wrong
// type, or carrying partial data. Handlers should treat the returned ToolCtx
// as best-effort and validate fields they actually depend on.
func ExtractToolCtx(req mcp.CallToolRequest) (ToolCtx, bool) {
	args := req.GetArguments()
	raw, ok := args[CtxArgKey]
	if !ok {
		return ToolCtx{}, false
	}
	asMap, ok := raw.(map[string]any)
	if !ok {
		return ToolCtx{}, false
	}
	tc := ToolCtx{}
	if v, ok := asMap["user_name"].(string); ok {
		tc.UserName = v
	}
	if v, ok := asMap["session_id"].(string); ok {
		tc.SessionID = v
	}
	if v, ok := asMap["trace_id"].(string); ok {
		tc.TraceID = v
	}
	return tc, !tc.IsZero()
}
