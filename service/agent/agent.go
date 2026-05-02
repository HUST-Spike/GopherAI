package agent

import (
	"GopherAI/common/agent"
	"GopherAI/common/code"
	mcpclient "GopherAI/common/mcp/client"
	"GopherAI/config"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/openai"
)

var (
	agentsMu sync.RWMutex
	agents   = make(map[string]*agent.Agent)
)

func getOrCreateAgent(ctx context.Context, sessionID string) (*agent.Agent, error) {
	agentsMu.RLock()
	if a, ok := agents[sessionID]; ok {
		agentsMu.RUnlock()
		return a, nil
	}
	agentsMu.RUnlock()

	agentsMu.Lock()
	defer agentsMu.Unlock()

	if a, ok := agents[sessionID]; ok {
		return a, nil
	}

	conf := config.GetConfig()
	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: conf.RagModelConfig.RagBaseUrl,
		Model:   conf.RagModelConfig.RagChatModelName,
		APIKey:  os.Getenv("OPENAI_API_KEY"),
	})
	if err != nil {
		return nil, fmt.Errorf("create agent LLM failed: %v", err)
	}

	client, err := mcpclient.Dial(ctx, conf.MCPServerURL(), "GopherAI-Agent")
	if err != nil {
		return nil, fmt.Errorf("agent mcp dial failed: %v", err)
	}

	toolsResult, err := client.ListTools(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("agent list tools failed: %v", err)
	}

	a, err := agent.NewAgent(agent.AgentConfig{
		Name:        "GopherAI Agent",
		Description: "一个能够自主规划任务、调用工具并完成复杂多步骤任务的智能代理。",
		LLM:         llm,
		MCPClient:   client,
		Tools:       toolsResult.Tools,
		MaxSteps:    15,
	})
	if err != nil {
		return nil, err
	}

	agents[sessionID] = a
	return a, nil
}

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

// sseEvent is a properly typed SSE data payload for agent events.
type sseEvent struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Step    int    `json:"step"`
}

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

	writeSSE := func(evt sseEvent) {
		data, _ := json.Marshal(evt)
		writer.Write([]byte("data: " + string(data) + "\n\n"))
		flusher.Flush()
	}

	cb := func(event agent.AgentEvent) {
		writeSSE(sseEvent{Type: event.Type, Content: event.Content, Step: event.Step})
	}

	result, err := a.StreamExecute(ctx, task, cb)
	if err != nil {
		log.Printf("StreamExecuteTask error: %v", err)
		writeSSE(sseEvent{Type: "error", Content: err.Error(), Step: 0})
		return code.AIModelFail
	}

	writeSSE(sseEvent{Type: "done", Content: result.FinalAnswer, Step: result.TotalSteps})
	writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()

	return code.CodeSuccess
}
