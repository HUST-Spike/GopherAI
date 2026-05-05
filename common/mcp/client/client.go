package mcpclient

import (
	mcpconv "GopherAI/common/mcp"
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPClient wraps the mcp-go client with convenience methods.
type MCPClient struct {
	c   *client.Client
	url string
}

// Dial creates a new MCPClient connected and initialized to the given MCP server URL.
// This is the single entry point — callers should NOT duplicate the init handshake.
func Dial(ctx context.Context, mcpURL string, clientName string) (*MCPClient, error) {
	httpTransport, err := transport.NewStreamableHTTP(mcpURL)
	if err != nil {
		return nil, fmt.Errorf("create MCP transport failed: %w", err)
	}

	c := client.NewClient(httpTransport)

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    clientName,
		Version: "2.0.0",
	}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	if _, err := c.Initialize(ctx, initReq); err != nil {
		c.Close()
		return nil, fmt.Errorf("MCP initialize failed: %w", err)
	}

	return &MCPClient{c: c, url: mcpURL}, nil
}

// Raw returns the underlying mcp-go client for advanced usage (e.g. Agent needs it directly).
func (m *MCPClient) Raw() *client.Client {
	return m.c
}

func (m *MCPClient) Ping(ctx context.Context) error {
	return m.c.Ping(ctx)
}

func (m *MCPClient) ListTools(ctx context.Context) (*mcp.ListToolsResult, error) {
	return m.c.ListTools(ctx, mcp.ListToolsRequest{})
}

// CallTool invokes a tool on the connected MCP server.
//
// If the caller has attached a ToolCtx to ctx via mcpconv.WithToolCtx, the
// client transparently merges it into args under the reserved `_ctx` key so
// server-side handlers can identify the originating user/session/trace
// without exposing those fields to the LLM. The original args map is not
// mutated.
func (m *MCPClient) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	if tc, ok := mcpconv.ToolCtxFrom(ctx); ok {
		args = mcpconv.InjectCtxIntoArgs(args, tc)
	}
	callReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	}
	return m.c.CallTool(ctx, callReq)
}

func (m *MCPClient) Close() {
	if m.c != nil {
		m.c.Close()
	}
}
