package model

import "time"

// ToolInvocationStatus represents the terminal status of a single tool call attempt.
type ToolInvocationStatus string

const (
	ToolInvocationStatusSuccess   ToolInvocationStatus = "success"
	ToolInvocationStatusError     ToolInvocationStatus = "error"
	ToolInvocationStatusTimeout   ToolInvocationStatus = "timeout"
	ToolInvocationStatusCancelled ToolInvocationStatus = "cancelled"
)

// ToolInvocation records one attempt of an MCP tool call. Multiple retry attempts
// of the same logical call share `tool_call_id` and differ in `attempt`.
type ToolInvocation struct {
	ID uint64 `gorm:"primaryKey;autoIncrement" json:"id"`

	TraceID    string `gorm:"type:varchar(64);not null;index:idx_trace"            json:"trace_id"`
	UserName   string `gorm:"type:varchar(64);not null;index:idx_user_time,priority:1" json:"user_name"`
	SessionID  string `gorm:"type:varchar(64);not null;index:idx_session_time,priority:1" json:"session_id"`
	MessageID  uint   `gorm:"index"                                                json:"message_id,omitempty"`
	ToolCallID string `gorm:"type:varchar(128);not null;index:idx_call_id"         json:"tool_call_id"`

	ToolName      string               `gorm:"type:varchar(64);not null;index:idx_tool_status,priority:1" json:"tool_name"`
	ArgsJSON      string               `gorm:"type:text"                                                  json:"args_json,omitempty"`
	ArgsSize      int                  `gorm:"not null;default:0"                                         json:"args_size"`
	ResultPreview string               `gorm:"type:varchar(2000)"                                         json:"result_preview,omitempty"`
	ResultSize    int                  `gorm:"not null;default:0"                                         json:"result_size"`
	ResultPath    string               `gorm:"type:varchar(255)"                                          json:"result_path,omitempty"`
	Status        ToolInvocationStatus `gorm:"type:varchar(16);not null;index:idx_tool_status,priority:2" json:"status"`
	ErrorMsg      string               `gorm:"type:varchar(1000)"                                         json:"error_msg,omitempty"`
	Attempt       int                  `gorm:"not null;default:1"                                         json:"attempt"`
	MaxAttempts   int                  `gorm:"not null;default:1"                                         json:"max_attempts"`
	DurationMs    int                  `gorm:"not null;default:0"                                         json:"duration_ms"`

	ActiveSkills string `gorm:"type:varchar(255)" json:"active_skills,omitempty"`
	ModelType    string `gorm:"type:varchar(8)"   json:"model_type,omitempty"`

	CreatedAt time.Time `gorm:"not null;index:idx_session_time,priority:2;index:idx_user_time,priority:2;index:idx_tool_status,priority:3" json:"created_at"`
}

func (ToolInvocation) TableName() string {
	return "tool_invocations"
}
