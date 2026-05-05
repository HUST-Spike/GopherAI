package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

type quotableResponse struct {
	Content string   `json:"content"`
	Author  string   `json:"author"`
	Tags    []string `json:"tags"`
	Length  int      `json:"length"`
}

func registerRandomQuote(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"random_quote",
		mcp.WithDescription("返回一段随机的英文名言（含作者）。无入参。"),
	)
	reg.Register(tool, handleRandomQuote)
}

func handleRandomQuote(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	url := "https://api.quotable.io/random"
	var data quotableResponse
	if err := httpGetJSON(ctx, url, &data); err != nil {
		return nil, fmt.Errorf("random_quote: %w", err)
	}

	text := fmt.Sprintf(
		"\"%s\"\n  — %s\nTags: %v",
		data.Content, data.Author, data.Tags,
	)
	return textResult(text), nil
}
