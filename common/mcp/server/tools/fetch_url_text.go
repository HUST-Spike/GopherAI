package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/net/html"
)

const fetchURLMaxBytes = 256 * 1024

func registerFetchURLText(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"fetch_url_text",
		mcp.WithDescription("抓取一个 HTTP/HTTPS 网页并返回标题与正文文本，剥离 HTML 标签、脚本和样式。"),
		mcp.WithString("url",
			mcp.Description("完整 URL，必须以 http:// 或 https:// 开头"),
			mcp.Required(),
		),
	)
	reg.Register(tool, handleFetchURLText)
}

func handleFetchURLText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	raw, err := stringArg(req, "url")
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("fetch_url_text: invalid url %q", raw)
	}

	body, err := httpGetBytes(ctx, raw, fetchURLMaxBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch_url_text: %w", err)
	}

	title, text := extractTitleAndText(body)
	out := fmt.Sprintf("URL: %s\n标题: %s\n\n正文:\n%s",
		raw, emptyToDash(title), text,
	)
	return textResult(out), nil
}

// extractTitleAndText walks the HTML, collecting the document title and the
// visible body text. Script / style / noscript / template / iframe content is
// dropped because for an LLM consumer it is pure noise.
func extractTitleAndText(body []byte) (title string, text string) {
	tokenizer := html.NewTokenizer(bytes.NewReader(body))
	var (
		sb        strings.Builder
		skipDepth int
		inTitle   bool
	)
	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return strings.TrimSpace(title), normalizeWhitespace(sb.String())
		case html.StartTagToken, html.SelfClosingTagToken:
			tn, _ := tokenizer.TagName()
			tag := string(tn)
			if isSkippableTag(tag) {
				skipDepth++
				continue
			}
			if tag == "title" {
				inTitle = true
			}
		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			tag := string(tn)
			if isSkippableTag(tag) && skipDepth > 0 {
				skipDepth--
				continue
			}
			if tag == "title" {
				inTitle = false
			}
		case html.TextToken:
			if skipDepth > 0 {
				continue
			}
			t := string(tokenizer.Text())
			if inTitle {
				title += t
				continue
			}
			sb.WriteString(t)
			sb.WriteByte(' ')
		}
	}
}

func isSkippableTag(tag string) bool {
	switch tag {
	case "script", "style", "noscript", "template", "iframe", "svg", "canvas":
		return true
	}
	return false
}

// normalizeWhitespace collapses runs of whitespace into single spaces and
// double-newline-trims paragraph-like sequences. Without this, html bodies
// produce wide ragged whitespace that wastes LLM context.
func normalizeWhitespace(s string) string {
	var sb strings.Builder
	prevSpace := true
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				sb.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		sb.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(sb.String())
}
