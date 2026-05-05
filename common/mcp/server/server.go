package mcpserver

import (
	"context"
	"log"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// =================== Tool Registry ===================

type ToolHandler func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

// ToolScope decides who can see a tool. Only ScopeGlobal tools appear by default;
// ScopeSkillRestricted tools require at least one of their AllowedSkills to be
// active in the current session before the tool is bound to the LLM.
type ToolScope int

const (
	ScopeGlobal ToolScope = iota
	ScopeSkillRestricted
)

// ToolMeta carries authorization-style metadata that lives alongside a tool.
// It is intentionally NOT part of the MCP protocol: filtering happens on the
// in-process client side (see MCPModel.discoverAndBindTools).
type ToolMeta struct {
	Scope         ToolScope
	AllowedSkills []string
}

func defaultToolMeta() ToolMeta {
	return ToolMeta{Scope: ScopeGlobal}
}

type ToolEntry struct {
	Tool    mcp.Tool
	Handler ToolHandler
	Meta    ToolMeta
}

type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]ToolEntry
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]ToolEntry),
	}
}

// Register adds a tool. If meta is omitted the tool is treated as ScopeGlobal.
// Variadic meta keeps existing call sites compiling unchanged.
func (r *ToolRegistry) Register(tool mcp.Tool, handler ToolHandler, meta ...ToolMeta) {
	m := defaultToolMeta()
	if len(meta) > 0 {
		m = meta[0]
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name] = ToolEntry{Tool: tool, Handler: handler, Meta: m}
}

func (r *ToolRegistry) Apply(s *server.MCPServer) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.tools {
		s.AddTool(entry.Tool, server.ToolHandlerFunc(entry.Handler))
	}
}

func (r *ToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// GetMeta returns the meta for a registered tool. Used by in-process clients
// to decide which tools to actually bind to the LLM for the current session.
func (r *ToolRegistry) GetMeta(name string) (ToolMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.tools[name]
	if !ok {
		return ToolMeta{}, false
	}
	return e.Meta, true
}

// VisibleTools filters a tool list using activeSkills. A tool is visible if
// either its Scope is Global, or its AllowedSkills intersects activeSkills.
// Tools not registered in the registry are passed through (treated as global)
// to stay forward-compatible with externally-supplied tool lists.
func (r *ToolRegistry) VisibleTools(all []mcp.Tool, activeSkills []string) []mcp.Tool {
	skillSet := make(map[string]struct{}, len(activeSkills))
	for _, s := range activeSkills {
		skillSet[s] = struct{}{}
	}
	out := make([]mcp.Tool, 0, len(all))
	for _, t := range all {
		meta, ok := r.GetMeta(t.Name)
		if !ok || meta.Scope == ScopeGlobal {
			out = append(out, t)
			continue
		}
		for _, s := range meta.AllowedSkills {
			if _, hit := skillSet[s]; hit {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// DefaultRegistry is the global tool registry. The concrete tools live in
// the sibling `tools` package and are installed at boot time by the caller
// (typically main.go) via tools.RegisterAll(DefaultRegistry). Splitting the
// registry from the tool definitions avoids an import cycle and makes the
// list of available tools an explicit, top-level decision.
var DefaultRegistry = NewToolRegistry()

// =================== MCP Server ===================

func NewMCPServer() *server.MCPServer {
	mcpServer := server.NewMCPServer(
		"GopherAI-MCP-Server",
		"2.0.0",
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)
	DefaultRegistry.Apply(mcpServer)
	return mcpServer
}

func StartServer(httpAddr string) error {
	mcpServer := NewMCPServer()
	httpServer := server.NewStreamableHTTPServer(mcpServer)
	log.Printf("MCP server listening on %s/mcp (%d tools registered)", httpAddr, DefaultRegistry.Count())
	return httpServer.Start(httpAddr)
}
