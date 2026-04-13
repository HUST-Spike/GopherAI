package mcpclient

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPClient wraps the mcp-go client with convenience methods.
type MCPClient struct {
	c *client.Client
}

func NewMCPClient(httpURL string) (*MCPClient, error) {
	httpTransport, err := transport.NewStreamableHTTP(httpURL)
	if err != nil {
		return nil, fmt.Errorf("create HTTP transport failed: %w", err)
	}

	c := client.NewClient(httpTransport)
	return &MCPClient{c: c}, nil
}

func (m *MCPClient) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "GopherAI-MCPClient",
		Version: "2.0.0",
	}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	return m.c.Initialize(ctx, initReq)
}

func (m *MCPClient) Ping(ctx context.Context) error {
	return m.c.Ping(ctx)
}

func (m *MCPClient) ListTools(ctx context.Context) (*mcp.ListToolsResult, error) {
	return m.c.ListTools(ctx, mcp.ListToolsRequest{})
}

func (m *MCPClient) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	callReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	}
	return m.c.CallTool(ctx, callReq)
}

func (m *MCPClient) GetToolResultText(result *mcp.CallToolResult) string {
	var text string
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			text += textContent.Text + "\n"
		}
	}
	return text
}

func (m *MCPClient) Close() {
	if m.c != nil {
		m.c.Close()
	}
}
