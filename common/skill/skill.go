package skill

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// Skill represents a composable AI capability unit.
// Each Skill encapsulates a system prompt, optional tool requirements,
// and custom pre/post processing logic.
type Skill interface {
	Name() string
	Description() string
	Version() string

	// RequiredTools returns the names of MCP tools this skill depends on.
	RequiredTools() []string

	// SystemPrompt returns the system-level instruction for this skill.
	SystemPrompt() string

	// PreProcess is called before LLM generation.
	// It can modify the messages (e.g. inject context, rewrite queries).
	// Return nil to skip modification.
	PreProcess(ctx context.Context, sc *SkillContext, messages []*schema.Message) ([]*schema.Message, error)

	Init(config map[string]interface{}) error
	Close() error
}

// SkillContext carries session-scoped data into skill execution.
type SkillContext struct {
	UserName  string
	SessionID string
}
