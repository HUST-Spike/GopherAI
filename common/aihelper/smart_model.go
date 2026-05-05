package aihelper

import (
	"GopherAI/config"
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
)

// SmartModel is the unified "all-in-one" chat backend (modelType=5).
//
// It is a thin policy layer over MCPModel: tools, skill prompt injection
// and the multi-round agent loop are each governed by an independent
// runtime switch (see config.SmartChatRuntime). This lets the demo prove
// each concern works in isolation by flipping a single env var, without
// having to maintain four separate model implementations.
//
// When ENABLE_TOOLS=false the model falls back to the bare LLM and never
// even calls ListTools, so a "pure chat" smoke test costs zero MCP traffic.
type SmartModel struct {
	mcp     *MCPModel
	rt      config.SmartChatRuntime
	session string
}

// NewSmartModel builds a SmartModel. The underlying MCPModel is always
// constructed (even when tools are disabled) so toggling ENABLE_TOOLS at
// runtime doesn't require restarting the process — the next message just
// observes the new switch.
func NewSmartModel(ctx context.Context, username string, sessionID string) (*SmartModel, error) {
	mcp, err := NewMCPModel(ctx, username, sessionID)
	if err != nil {
		return nil, fmt.Errorf("smart model: bootstrap mcp failed: %w", err)
	}
	return &SmartModel{
		mcp:     mcp,
		rt:      config.GetConfig().SmartChat(),
		session: sessionID,
	}, nil
}

func (s *SmartModel) GetModelType() string { return "5" }

func (s *SmartModel) Close() error { return s.mcp.Close() }

// Skill prompt injection is handled one layer up in AIHelper.{Generate,
// Stream}Response so every modelType picks up the same toggle. SmartModel
// only needs to gate tools and the agent loop here.

func (s *SmartModel) maxRounds() int {
	if !s.rt.EnableAgentLoop {
		return 1
	}
	if s.rt.ToolCallMaxRounds <= 0 {
		return mcpMaxToolRounds
	}
	return s.rt.ToolCallMaxRounds
}

func (s *SmartModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	if !s.rt.EnableTools {
		// Tools-off path: bypass MCP entirely and call the base LLM. We
		// reach into MCPModel.baseLLM rather than rebuilding an OpenAI
		// client so we keep using the exact same upstream config.
		return s.mcp.baseLLM.Generate(ctx, messages)
	}
	return s.mcp.generateWithRoundCap(ctx, messages, s.maxRounds())
}

func (s *SmartModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	if !s.rt.EnableTools {
		stream, err := s.mcp.baseLLM.Stream(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("smart stream failed: %w", err)
		}
		_, text, err := drainStream(stream, cb)
		return text, err
	}
	return s.mcp.streamWithRoundCap(ctx, messages, cb, s.maxRounds())
}
