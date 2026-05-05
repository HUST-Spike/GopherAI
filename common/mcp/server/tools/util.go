// Package tools hosts the concrete MCP tool implementations.
//
// Each tool lives in its own file so adding/removing one is a pure file diff.
// The single entry point is RegisterAll, which is called once at server boot
// to install every tool into mcpserver.DefaultRegistry.
//
// Conventions for individual tool files:
//   - Define a registerXxx(reg) function and a handleXxx handler.
//   - Keep handlers free of global state; they may freely use ctx for HTTP
//     deadline propagation (the per-call timeout is enforced by the caller's
//     ctx, see Step 6 ToolRunner).
//   - Always run results through truncate so a runaway upstream cannot blow
//     up the LLM context.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
)

// MaxResultBytes caps the size of any tool's textual result. Beyond this we
// truncate and append a marker so the LLM (and the user) sees something
// happened rather than a silently chopped string.
const MaxResultBytes = 8 * 1024

// userAgent is sent on every outbound HTTP request so upstream services can
// identify GopherAI in their logs.
const userAgent = "GopherAI-MCP/1.0 (+https://github.com/)"

// httpGetJSON performs a GET request and decodes the JSON body into target.
// It honors ctx (timeout / cancellation) and surfaces non-2xx responses as
// errors with the response body snippet attached, which is much easier to
// debug than a bare "unexpected status".
func httpGetJSON(ctx context.Context, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request failed: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("upstream returned %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode response failed: %w", err)
	}
	return nil
}

// httpGetBytes performs a GET request and returns the raw body, capped at
// maxBytes. Useful for tools that want to do their own parsing (HTML, XML).
func httpGetBytes(ctx context.Context, url string, maxBytes int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request failed: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned %d", resp.StatusCode)
	}

	limit := int64(maxBytes)
	if limit <= 0 {
		limit = MaxResultBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}
	return body, nil
}

// truncate caps s at MaxResultBytes and appends a clear marker if it had to
// chop. The marker uses byte counts so the consumer can spot whether they
// hit the limit rather than getting empty data with no explanation.
func truncate(s string) string {
	if len(s) <= MaxResultBytes {
		return s
	}
	return s[:MaxResultBytes] + fmt.Sprintf("\n... (truncated, original %d bytes)", len(s))
}

// textResult wraps plain text into the MCP CallToolResult shape every handler
// must return. The truncate pass keeps the contract uniform.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: truncate(text)},
		},
	}
}

// stringArg pulls a required string argument out of a CallToolRequest with
// a consistent error message. Used by every handler so error messages are
// uniform across tools.
func stringArg(req mcp.CallToolRequest, name string) (string, error) {
	args := req.GetArguments()
	v, ok := args[name].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("argument %q is required", name)
	}
	return v, nil
}

// optionalStringArg returns the argument's value or fallback if missing.
func optionalStringArg(req mcp.CallToolRequest, name, fallback string) string {
	args := req.GetArguments()
	if v, ok := args[name].(string); ok && v != "" {
		return v
	}
	return fallback
}

// optionalIntArg parses the named arg as int; falls back if absent or unparsable.
// LLMs often pass numbers as float64 (default JSON number type), so we accept
// both rather than punishing them for it.
func optionalIntArg(req mcp.CallToolRequest, name string, fallback int) int {
	args := req.GetArguments()
	switch v := args[name].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		if v == "" {
			return fallback
		}
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

// optionalFloatArg parses the named arg as float64; tolerates numeric strings.
func optionalFloatArg(req mcp.CallToolRequest, name string, fallback float64) float64 {
	args := req.GetArguments()
	switch v := args[name].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		if v == "" {
			return fallback
		}
		var f float64
		if _, err := fmt.Sscanf(v, "%f", &f); err == nil {
			return f
		}
	}
	return fallback
}
