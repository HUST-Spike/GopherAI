package agent

import (
	"GopherAI/common/agent"
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	mcpclient "GopherAI/common/mcp/client"
	"GopherAI/config"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/google/uuid"
)

var (
	agentsMu sync.RWMutex
	agents   = make(map[string]*agent.Agent)
)

func getOrCreateAgent(ctx context.Context, userName string, sessionID string) (*agent.Agent, error) {
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
		UserName:    userName,
		SessionID:   sessionID,
	})
	if err != nil {
		return nil, err
	}

	agents[sessionID] = a
	return a, nil
}

func ExecuteTask(userName string, sessionID string, task string) (*agent.AgentResult, code.Code) {
	ctx := context.Background()
	a, err := getOrCreateAgent(ctx, userName, sessionID)
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

// StreamExecuteTask drives the agent loop over SSE using the shared
// aihelper.StreamEvent envelope. Mapping from agent.AgentEvent → StreamEvent
// happens here so the frontend doesn't need a separate parser for /agent
// endpoints versus /chat endpoints — one renderer handles both.
func StreamExecuteTask(userName string, sessionID string, task string, writer http.ResponseWriter) code.Code {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return code.CodeServerBusy
	}

	traceID := uuid.New().String()
	ctx := agent.WithTraceID(context.Background(), traceID)

	a, err := getOrCreateAgent(ctx, userName, sessionID)
	if err != nil {
		log.Printf("StreamExecuteTask getOrCreateAgent error: %v", err)
		return code.AIModelFail
	}

	writeEvent := func(evt aihelper.StreamEvent) {
		if evt.TraceID == "" {
			evt.TraceID = traceID
		}
		writer.Write(append([]byte("data: "), append(evt.Encode(), []byte("\n\n")...)...))
		flusher.Flush()
	}

	writeEvent(aihelper.SessionStartEvent(sessionID, traceID))

	cb := func(event agent.AgentEvent) {
		writeEvent(agentEventToStreamEvent(event))
	}

	result, err := a.StreamExecute(ctx, task, cb)
	if err != nil {
		log.Printf("StreamExecuteTask error: %v", err)
		writeEvent(aihelper.ErrorEvent(err.Error()))
		return code.AIModelFail
	}

	if result != nil && result.FinalAnswer != "" {
		writeEvent(aihelper.StreamEvent{Type: "answer", Data: result.FinalAnswer, Step: result.TotalSteps})
	}
	writeEvent(aihelper.DoneEvent())
	return code.CodeSuccess
}

// agentEventToStreamEvent maps the legacy AgentEvent enum onto the unified
// StreamEvent shape. Tool events lose the human-readable preamble ("调用工具
// X:") because the frontend now renders them from structured fields, so we
// pass the raw content through as Args / Preview rather than re-formatting.
func agentEventToStreamEvent(e agent.AgentEvent) aihelper.StreamEvent {
	switch e.Type {
	case "thinking":
		return aihelper.StreamEvent{Type: "thinking", Data: e.Content, Step: e.Step}
	case "tool_call":
		return aihelper.StreamEvent{Type: "tool_call", Tool: "", Data: e.Content, Step: e.Step}
	case "tool_result":
		return aihelper.StreamEvent{Type: "tool_result", Preview: e.Content, Step: e.Step}
	case "answer":
		return aihelper.StreamEvent{Type: "answer", Data: e.Content, Step: e.Step}
	case "error":
		return aihelper.ErrorEvent(e.Content)
	default:
		return aihelper.StreamEvent{Type: e.Type, Data: e.Content, Step: e.Step}
	}
}
