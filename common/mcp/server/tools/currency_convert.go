package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type erApiResponse struct {
	Result    string             `json:"result"`
	BaseCode  string             `json:"base_code"`
	TimeLast  string             `json:"time_last_update_utc"`
	Rates     map[string]float64 `json:"rates"`
	ErrorType string             `json:"error-type,omitempty"`
}

func registerCurrencyConvert(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"currency_convert",
		mcp.WithDescription("法币汇率换算。基于 open.er-api.com 的实时汇率。"),
		mcp.WithString("from",
			mcp.Description("源币种，如 CNY、USD、JPY"),
			mcp.Required(),
		),
		mcp.WithString("to",
			mcp.Description("目标币种，如 USD"),
			mcp.Required(),
		),
		mcp.WithNumber("amount",
			mcp.Description("源币种金额。默认 1。"),
		),
	)
	reg.Register(tool, handleCurrencyConvert)
}

func handleCurrencyConvert(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	from, err := stringArg(req, "from")
	if err != nil {
		return nil, err
	}
	to, err := stringArg(req, "to")
	if err != nil {
		return nil, err
	}
	from = strings.ToUpper(from)
	to = strings.ToUpper(to)
	amount := optionalFloatArg(req, "amount", 1.0)

	url := fmt.Sprintf("https://open.er-api.com/v6/latest/%s", from)
	var data erApiResponse
	if err := httpGetJSON(ctx, url, &data); err != nil {
		return nil, fmt.Errorf("currency_convert: %w", err)
	}
	if data.Result != "success" {
		return nil, fmt.Errorf("currency_convert: upstream error: %s", data.ErrorType)
	}

	rate, ok := data.Rates[to]
	if !ok {
		return nil, fmt.Errorf("currency_convert: unknown target currency %q", to)
	}

	converted := amount * rate
	text := fmt.Sprintf(
		"汇率 (%s → %s): %.6f\n%.4f %s = %.4f %s\n汇率更新时间 (UTC): %s",
		from, to, rate,
		amount, from, converted, to,
		data.TimeLast,
	)
	return textResult(text), nil
}
