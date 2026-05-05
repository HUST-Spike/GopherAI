package aihelper

import "encoding/json"

// StreamEvent is the structured envelope every streaming AI backend emits.
//
// All chat SSE endpoints share this shape so the frontend has exactly one
// schema to render. Fields are populated based on Type:
//
//   - "session"      → {SessionID}
//   - "token"        → {Data}
//   - "tool_call"    → {Tool, CallID, Args, Step?}
//   - "tool_result"  → {Tool, CallID, Status, Preview, Attempts, DurationMs, Step?}
//   - "answer"       → {Data} (final assistant reply, used by Agent)
//   - "thinking"     → {Data} (Agent reasoning step)
//   - "error"        → {Data}
//   - "done"         → no fields
//
// Unused fields are omitted via `omitempty` so the wire payload stays small.
type StreamEvent struct {
	Type       string `json:"type"`
	Data       string `json:"data,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
	Tool       string `json:"tool,omitempty"`
	CallID     string `json:"call_id,omitempty"`
	Args       string `json:"args,omitempty"`
	Preview    string `json:"preview,omitempty"`
	Status     string `json:"status,omitempty"`
	Attempts   int    `json:"attempts,omitempty"`
	DurationMs int    `json:"duration_ms,omitempty"`
	Step       int    `json:"step,omitempty"`
}

// MarshalJSON cannot easily omit zero ints; the omitempty tag handles that
// for us. We expose Encode for the common SSE write path so callers don't
// reimplement it. On encoding error we return an error event payload rather
// than failing the whole stream — the caller can decide whether to surface.
func (e StreamEvent) Encode() []byte {
	b, err := json.Marshal(e)
	if err != nil {
		fallback, _ := json.Marshal(StreamEvent{Type: "error", Data: "encode failed: " + err.Error()})
		return fallback
	}
	return b
}

// Constructors keep call sites readable; they all set Type so callers don't
// have to remember the exact magic string.
func TokenEvent(text string) StreamEvent {
	return StreamEvent{Type: "token", Data: text}
}

func ToolCallEvent(tool, callID, args string) StreamEvent {
	return StreamEvent{Type: "tool_call", Tool: tool, CallID: callID, Args: args}
}

func ToolResultEvent(tool, callID, status, preview string, attempts, durationMs int) StreamEvent {
	return StreamEvent{
		Type: "tool_result", Tool: tool, CallID: callID,
		Status: status, Preview: preview, Attempts: attempts, DurationMs: durationMs,
	}
}

func ErrorEvent(message string) StreamEvent {
	return StreamEvent{Type: "error", Data: message}
}

func DoneEvent() StreamEvent {
	return StreamEvent{Type: "done"}
}

func SessionEvent(sessionID string) StreamEvent {
	return StreamEvent{Type: "session", SessionID: sessionID}
}

// SessionStartEvent is the very first SSE frame for any streaming chat. It
// carries both session_id and trace_id so the frontend can label every
// downstream event (including failures and tool calls) with the originating
// request, which is essential for correlating UI state with backend logs.
func SessionStartEvent(sessionID, traceID string) StreamEvent {
	return StreamEvent{Type: "session", SessionID: sessionID, TraceID: traceID}
}
