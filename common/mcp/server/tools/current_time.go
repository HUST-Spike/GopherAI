package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func registerCurrentTime(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"get_current_time",
		mcp.WithDescription("获取当前时间。返回 ISO8601 字符串与人类可读格式。可选时区参数。"),
		mcp.WithString("timezone",
			mcp.Description("IANA 时区名称（如 Asia/Shanghai、UTC、America/New_York）。默认 Asia/Shanghai。"),
		),
	)
	reg.Register(tool, handleGetCurrentTime)
}

func handleGetCurrentTime(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tz := optionalStringArg(req, "timezone", "Asia/Shanghai")
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("get_current_time: invalid timezone %q: %w", tz, err)
	}

	now := time.Now().In(loc)
	text := fmt.Sprintf(
		"时区: %s\n当前时间: %s\nISO8601: %s\nUnix 时间戳: %d",
		tz,
		now.Format("2006-01-02 15:04:05 MST"),
		now.Format(time.RFC3339),
		now.Unix(),
	)
	return textResult(text), nil
}
