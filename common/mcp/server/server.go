package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
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

// DefaultRegistry is the global tool registry. Register tools here before calling StartServer.
var DefaultRegistry = NewToolRegistry()

func init() {
	registerBuiltinTools(DefaultRegistry)
}

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

// =================== Builtin Tools ===================

func registerBuiltinTools(reg *ToolRegistry) {
	registerWeatherTool(reg)
	registerEmailTool(reg)
}

// --- Weather Tool ---

type WttrResponse struct {
	CurrentCondition []struct {
		TempC         string `json:"temp_C"`
		Humidity      string `json:"humidity"`
		WindspeedKmph string `json:"windspeedKmph"`
		WeatherDesc   []struct {
			Value string `json:"value"`
		} `json:"weatherDesc"`
	} `json:"current_condition"`
	NearestArea []struct {
		AreaName []struct {
			Value string `json:"value"`
		} `json:"areaName"`
	} `json:"nearest_area"`
}

func registerWeatherTool(reg *ToolRegistry) {
	tool := mcp.NewTool(
		"get_weather",
		mcp.WithDescription("获取指定城市的实时天气信息，包括温度、湿度、风速等"),
		mcp.WithString("city", mcp.Description("城市名称，如 Beijing、上海"), mcp.Required()),
	)
	reg.Register(tool, handleGetWeather)
}

func handleGetWeather(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	city, ok := args["city"].(string)
	if !ok || city == "" {
		return nil, fmt.Errorf("invalid city argument")
	}

	apiURL := fmt.Sprintf("https://wttr.in/%s?format=j1&lang=zh", city)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	var wttrResp WttrResponse
	if err := json.Unmarshal(body, &wttrResp); err != nil {
		return nil, fmt.Errorf("json parse failed: %w", err)
	}

	if len(wttrResp.CurrentCondition) == 0 {
		return nil, fmt.Errorf("no weather data for city: %s", city)
	}

	cc := wttrResp.CurrentCondition[0]
	temp, _ := strconv.ParseFloat(cc.TempC, 64)
	humidity, _ := strconv.Atoi(cc.Humidity)
	wind, _ := strconv.ParseFloat(cc.WindspeedKmph, 64)

	location := city
	if len(wttrResp.NearestArea) > 0 && len(wttrResp.NearestArea[0].AreaName) > 0 {
		location = wttrResp.NearestArea[0].AreaName[0].Value
	}
	condition := "未知"
	if len(cc.WeatherDesc) > 0 {
		condition = cc.WeatherDesc[0].Value
	}

	resultText := fmt.Sprintf(
		"城市: %s\n温度: %.1f°C\n天气: %s\n湿度: %d%%\n风速: %.1f km/h",
		location, temp, condition, humidity, wind,
	)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: resultText},
		},
	}, nil
}

// --- Email Tool (stub: uses project's existing email infrastructure) ---

func registerEmailTool(reg *ToolRegistry) {
	tool := mcp.NewTool(
		"send_email",
		mcp.WithDescription("发送邮件给指定收件人"),
		mcp.WithString("to", mcp.Description("收件人邮箱地址"), mcp.Required()),
		mcp.WithString("subject", mcp.Description("邮件主题"), mcp.Required()),
		mcp.WithString("body", mcp.Description("邮件正文内容"), mcp.Required()),
	)
	reg.Register(tool, handleSendEmail)
}

func handleSendEmail(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	to, _ := args["to"].(string)
	subject, _ := args["subject"].(string)
	body, _ := args["body"].(string)

	if to == "" || subject == "" {
		return nil, fmt.Errorf("to and subject are required")
	}

	// TODO: integrate with common/email when SMTP is configured
	resultText := fmt.Sprintf("邮件已准备发送\n收件人: %s\n主题: %s\n正文: %s\n(注意: 需要配置 SMTP 后才能实际发送)", to, subject, body)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: resultText},
		},
	}, nil
}
