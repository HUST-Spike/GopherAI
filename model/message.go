package model

import (
	"time"
)

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"
)

type Message struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID string    `gorm:"index;not null;type:varchar(36)" json:"session_id"`
	UserName  string    `gorm:"type:varchar(20)" json:"username"`
	Content   string    `gorm:"type:text" json:"content"`
	IsUser    bool      `gorm:"not null" json:"is_user"`
	Role      string    `gorm:"type:varchar(20);default:''" json:"role"`
	ToolName  string    `gorm:"type:varchar(100);default:''" json:"tool_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// GetRole returns the resolved role. Falls back to IsUser for backward compatibility.
func (m *Message) GetRole() string {
	if m.Role != "" {
		return m.Role
	}
	if m.IsUser {
		return RoleUser
	}
	return RoleAssistant
}

type History struct {
	IsUser   bool   `json:"is_user"`
	Role     string `json:"role"`
	Content  string `json:"content"`
	ToolName string `json:"tool_name,omitempty"`
}
