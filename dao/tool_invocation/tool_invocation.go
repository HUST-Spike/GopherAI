package toolinvocation

import (
	"GopherAI/common/mysql"
	"GopherAI/model"
)

// Insert persists a single tool invocation attempt.
// Failure to insert is the caller's concern: the main chat flow must NOT
// abort on logging errors, so callers typically just log the error.
func Insert(record *model.ToolInvocation) error {
	return mysql.DB.Create(record).Error
}

// ListBySession returns the most recent invocations for a session, newest first.
// Useful for an admin / debug page; not used by hot path.
func ListBySession(sessionID string, limit int) ([]model.ToolInvocation, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []model.ToolInvocation
	err := mysql.DB.
		Where("session_id = ?", sessionID).
		Order("created_at desc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// ListByTrace returns all invocations for a given trace_id, oldest first.
// This makes it easy to reconstruct the tool-call timeline of a single request.
func ListByTrace(traceID string) ([]model.ToolInvocation, error) {
	var rows []model.ToolInvocation
	err := mysql.DB.
		Where("trace_id = ?", traceID).
		Order("created_at asc, attempt asc").
		Find(&rows).Error
	return rows, err
}
