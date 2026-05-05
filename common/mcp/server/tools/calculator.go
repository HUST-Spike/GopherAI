package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"context"
	"fmt"
	"go/token"
	"go/types"

	"github.com/mark3labs/mcp-go/mcp"
)

func registerCalculator(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"calculator",
		mcp.WithDescription("数学表达式计算器。支持 + - * / % ()、整数与浮点、位运算、按 Go 表达式语法解析。"),
		mcp.WithString("expression",
			mcp.Description("要计算的表达式，例如 (1+2)*3.14、2**10、100/3.0"),
			mcp.Required(),
		),
	)
	reg.Register(tool, handleCalculator)
}

// handleCalculator leverages the standard library's go/types constant
// evaluator. This avoids pulling a third-party expression engine for the
// limited subset of expressions a chat user would normally ask about.
//
// Trade-off: only constant expressions work (no variables, no function
// calls). For our demo this is exactly what we want — a safe, dependency
// free arithmetic sandbox.
func handleCalculator(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	expr, err := stringArg(req, "expression")
	if err != nil {
		return nil, err
	}

	tv, err := types.Eval(token.NewFileSet(), nil, token.NoPos, expr)
	if err != nil {
		return nil, fmt.Errorf("calculator: invalid expression %q: %w", expr, err)
	}

	text := fmt.Sprintf("表达式: %s\n类型: %s\n结果: %s", expr, tv.Type, tv.Value)
	return textResult(text), nil
}
