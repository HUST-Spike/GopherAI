package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

const tavilyEndpoint = "https://api.tavily.com/search"

type tavilyRequest struct {
	APIKey            string   `json:"api_key"`
	Query             string   `json:"query"`
	SearchDepth       string   `json:"search_depth,omitempty"`
	MaxResults        int      `json:"max_results,omitempty"`
	IncludeAnswer     bool     `json:"include_answer,omitempty"`
	IncludeRawContent bool     `json:"include_raw_content,omitempty"`
	IncludeDomains    []string `json:"include_domains,omitempty"`
	ExcludeDomains    []string `json:"exclude_domains,omitempty"`
}

type tavilyResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type tavilyResponse struct {
	Query   string         `json:"query"`
	Answer  string         `json:"answer"`
	Results []tavilyResult `json:"results"`
}

func registerWebSearch(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"web_search",
		mcp.WithDescription("使用 Tavily 引擎进行实时联网搜索，返回若干个最相关的网页摘要。"+
			"适用于新闻、政策、最新版本号等会随时间变化的事实问题。"),
		mcp.WithString("query",
			mcp.Description("搜索关键词或自然语言问题"),
			mcp.Required(),
		),
		mcp.WithNumber("max_results",
			mcp.Description("返回结果数量，默认 5，最大 10"),
		),
		mcp.WithString("depth",
			mcp.Description("搜索深度: basic (默认, 快) 或 advanced (慢但更全)"),
		),
	)
	reg.Register(tool, handleWebSearch)
}

func handleWebSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	apiKey := strings.TrimSpace(os.Getenv("TAVILY_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("web_search: TAVILY_API_KEY is not configured on the server")
	}
	query, err := stringArg(req, "query")
	if err != nil {
		return nil, err
	}
	maxResults := optionalIntArg(req, "max_results", 5)
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 10 {
		maxResults = 10
	}
	depth := optionalStringArg(req, "depth", "basic")
	if depth != "basic" && depth != "advanced" {
		depth = "basic"
	}

	body, err := json.Marshal(tavilyRequest{
		APIKey:        apiKey,
		Query:         query,
		SearchDepth:   depth,
		MaxResults:    maxResults,
		IncludeAnswer: true,
	})
	if err != nil {
		return nil, fmt.Errorf("web_search: build request failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilyEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("web_search: build request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("web_search: http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("web_search: tavily returned %d: %s", resp.StatusCode, string(snippet))
	}

	var data tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("web_search: decode response failed: %w", err)
	}
	if len(data.Results) == 0 {
		return textResult(fmt.Sprintf("Tavily 未返回与 %q 相关的结果。", query)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "查询: %s\n", query)
	if strings.TrimSpace(data.Answer) != "" {
		fmt.Fprintf(&sb, "Tavily 摘要: %s\n", strings.TrimSpace(data.Answer))
	}
	fmt.Fprintf(&sb, "命中 %d 条结果:\n", len(data.Results))
	for i, r := range data.Results {
		preview := strings.TrimSpace(r.Content)
		if len([]rune(preview)) > 400 {
			runes := []rune(preview)
			preview = string(runes[:400]) + "..."
		}
		fmt.Fprintf(&sb,
			"\n[%d] %s\n    URL: %s\n    score=%.3f\n    %s\n",
			i+1,
			emptyToDash(r.Title),
			r.URL,
			r.Score,
			preview,
		)
	}
	return textResult(sb.String()), nil
}
