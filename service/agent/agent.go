package agent

import (
	"GopherAI/common/agent"
	"GopherAI/common/code"
	"GopherAI/config"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/openai"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

var (
	agentsMu sync.RWMutex
	agents   = make(map[string]*agent.Agent) // sessionID → agent
)

// getOrCreateAgent returns an existing agent or creates a new one for the session.
func getOrCreateAgent(ctx context.Context, sessionID string) (*agent.Agent, error) {
	agentsMu.RLock()
	if a, ok := agents[sessionID]; ok {
		agentsMu.RUnlock()
		return a, nil
	}
	agentsMu.RUnlock()

	agentsMu.Lock()
	defer agentsMu.Unlock()

	// Double-check after acquiring write lock
	if a, ok := agents[sessionID]; ok {
		return a, nil
	}

	conf := config.GetConfig()
	key := os.Getenv("OPENAI_API_KEY")
	modelName := conf.RagModelConfig.RagChatModelName
	baseURL := conf.RagModelConfig.RagBaseUrl

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
		APIKey:  key,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent LLM failed: %v", err)
	}

	mcpURL := conf.MCPServerURL()
	httpTransport, err := transport.NewStreamableHTTP(mcpURL)
	if err != nil {
		return nil, fmt.Errorf("create agent mcp transport failed: %v", err)
	}

	mcpClient := mcpclient.NewClient(httpTransport)
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "GopherAI-Agent", Version: "1.0.0"}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	if _, err := mcpClient.Initialize(ctx, initReq); err != nil {
		return nil, fmt.Errorf("agent mcp init failed: %v", err)
	}

	toolsResult, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("agent list tools failed: %v", err)
	}

	a, err := agent.NewAgent(agent.AgentConfig{
		Name:        "GopherAI Agent",
		Description: "一个能够自主规划任务、调用工具并完成复杂多步骤任务的智能代理。",
		LLM:         llm,
		MCPClient:   mcpClient,
		Tools:       toolsResult.Tools,
		MaxSteps:    15,
	})
	if err != nil {
		return nil, err
	}

	agents[sessionID] = a
	return a, nil
}

// ExecuteTask runs an agent task synchronously.
func ExecuteTask(userName string, sessionID string, task string) (*agent.AgentResult, code.Code) {
	ctx := context.Background()
	a, err := getOrCreateAgent(ctx, sessionID)
	if err != nil {
		log.Printf("ExecuteTask getOrCreateAgent error: %v", err)
		return nil, code.AIModelFail
	}

	result, err := a.Execute(ctx, task)
	if err != nil {
		log.Printf("ExecuteTask Execute error: %v", err)
		return nil, code.AIModelFail
	}

	return result, code.CodeSuccess
}

// StreamExecuteTask runs an agent task with SSE streaming events.
func StreamExecuteTask(userName string, sessionID string, task string, writer http.ResponseWriter) code.Code {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return code.CodeServerBusy
	}

	ctx := context.Background()
	a, err := getOrCreateAgent(ctx, sessionID)
	if err != nil {
		log.Printf("StreamExecuteTask getOrCreateAgent error: %v", err)
		return code.AIModelFail
	}

	cb := func(event agent.AgentEvent) {
		data := fmt.Sprintf(`{"type":"%s","content":"%s","step":%d}`,
			event.Type, escapeJSON(event.Content), event.Step)
		writer.Write([]byte("data: " + data + "\n\n"))
		flusher.Flush()
	}

	result, err := a.StreamExecute(ctx, task, cb)
	if err != nil {
		log.Printf("StreamExecuteTask error: %v", err)
		errData := fmt.Sprintf(`{"type":"error","content":"%s","step":0}`, escapeJSON(err.Error()))
		writer.Write([]byte("data: " + errData + "\n\n"))
		flusher.Flush()
		return code.AIModelFail
	}

	// Send final answer
	finalData := fmt.Sprintf(`{"type":"done","content":"%s","step":%d}`,
		escapeJSON(result.FinalAnswer), result.TotalSteps)
	writer.Write([]byte("data: " + finalData + "\n\n"))
	writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()

	return code.CodeSuccess
}

func escapeJSON(s string) string {
	// Simple escape for embedding in JSON string values
	s = fmt.Sprintf("%q", s)
	// Remove surrounding quotes added by %q
	if len(s) >= 2 {
		s = s[1 : len(s)-1]
	}
	return s
}
