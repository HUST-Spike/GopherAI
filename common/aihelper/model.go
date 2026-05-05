package aihelper

import (
	mcpconv "GopherAI/common/mcp"
	mcpclient "GopherAI/common/mcp/client"
	"GopherAI/common/rag"
	"GopherAI/config"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/mcp"
)

type StreamCallback func(msg string)

// AIModel defines the interface all AI model backends must implement.
type AIModel interface {
	GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error)
	GetModelType() string
	Close() error
}

// =================== OpenAI ===================

type OpenAIModel struct {
	llm model.ToolCallingChatModel
}

func NewOpenAIModel(ctx context.Context) (*OpenAIModel, error) {
	key := os.Getenv("OPENAI_API_KEY")
	modelName := os.Getenv("OPENAI_MODEL_NAME")
	baseURL := os.Getenv("OPENAI_BASE_URL")

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
		APIKey:  key,
	})
	if err != nil {
		return nil, fmt.Errorf("create openai model failed: %v", err)
	}
	return &OpenAIModel{llm: llm}, nil
}

func (o *OpenAIModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	resp, err := o.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("openai generate failed: %v", err)
	}
	return resp, nil
}

func (o *OpenAIModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	stream, err := o.llm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("openai stream failed: %v", err)
	}
	_, text, err := drainStream(stream, cb)
	if err != nil {
		return text, fmt.Errorf("openai stream recv failed: %v", err)
	}
	return text, nil
}

func (o *OpenAIModel) GetModelType() string { return "1" }
func (o *OpenAIModel) Close() error         { return nil }

// =================== Ollama ===================

type OllamaModel struct {
	llm model.ToolCallingChatModel
}

func NewOllamaModel(ctx context.Context, baseURL, modelName string) (*OllamaModel, error) {
	llm, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("create ollama model failed: %v", err)
	}
	return &OllamaModel{llm: llm}, nil
}

func (o *OllamaModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	resp, err := o.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("ollama generate failed: %v", err)
	}
	return resp, nil
}

func (o *OllamaModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	stream, err := o.llm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("ollama stream failed: %v", err)
	}
	_, text, err := drainStream(stream, cb)
	if err != nil {
		return text, fmt.Errorf("ollama stream recv failed: %v", err)
	}
	return text, nil
}

func (o *OllamaModel) GetModelType() string { return "4" }
func (o *OllamaModel) Close() error         { return nil }

// =================== Milvus RAG ===================

type MilvusRAGModel struct {
	llm       model.ToolCallingChatModel
	username  string
	sessionID string
	ragConfig rag.Config
}

func NewMilvusRAGModel(ctx context.Context, username string, sessionID string) (*MilvusRAGModel, error) {
	key := os.Getenv("OPENAI_API_KEY")
	conf := config.GetConfig()
	modelName := conf.RagModelConfig.RagChatModelName
	baseURL := conf.RagModelConfig.RagBaseUrl

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
		APIKey:  key,
	})
	if err != nil {
		return nil, fmt.Errorf("create milvus rag model failed: %v", err)
	}
	return &MilvusRAGModel{
		llm:       llm,
		username:  username,
		sessionID: sessionID,
		ragConfig: rag.LoadConfigFromEnv(),
	}, nil
}

func (m *MilvusRAGModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	ragMessages, err := m.buildRAGMessages(ctx, messages)
	if err != nil {
		if m.ragConfig.RetrievalFailOpen {
			log.Printf("milvus rag enrichment failed, falling back to plain LLM: %v", err)
			ragMessages = messages
		} else {
			return nil, err
		}
	}
	resp, err := m.llm.Generate(ctx, ragMessages)
	if err != nil {
		return nil, fmt.Errorf("milvus rag generate failed: %v", err)
	}
	return resp, nil
}

func (m *MilvusRAGModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	ragMessages, err := m.buildRAGMessages(ctx, messages)
	if err != nil {
		if m.ragConfig.RetrievalFailOpen {
			log.Printf("milvus rag enrichment failed, falling back to plain LLM: %v", err)
			ragMessages = messages
		} else {
			return "", err
		}
	}
	stream, err := m.llm.Stream(ctx, ragMessages)
	if err != nil {
		return "", fmt.Errorf("milvus rag stream failed: %v", err)
	}
	_, text, err := drainStream(stream, cb)
	if err != nil {
		return text, fmt.Errorf("milvus rag stream recv failed: %v", err)
	}
	return text, nil
}

func (m *MilvusRAGModel) buildRAGMessages(ctx context.Context, messages []*schema.Message) ([]*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	lastMessage := messages[len(messages)-1]
	query := lastMessage.Content

	retriever, err := rag.NewRetriever(m.ragConfig)
	if err != nil {
		return nil, err
	}
	chunks, err := retriever.Retrieve(ctx, m.username, m.sessionID, query)
	if err != nil {
		return nil, err
	}

	ragPrompt, err := rag.BuildPrompt(query, chunks, m.ragConfig.MaxContextChars)
	if err != nil {
		return nil, err
	}

	enriched := make([]*schema.Message, len(messages))
	copy(enriched, messages)
	enriched[len(enriched)-1] = &schema.Message{
		Role:    schema.User,
		Content: ragPrompt,
	}
	return enriched, nil
}

func (m *MilvusRAGModel) GetModelType() string { return "2" }
func (m *MilvusRAGModel) Close() error         { return nil }

// =================== MCP (Native Function Calling) ===================

const mcpMaxToolRounds = 10

type MCPModel struct {
	baseLLM   model.ToolCallingChatModel
	toolLLM   model.ToolCallingChatModel
	client    *mcpclient.MCPClient
	mcpURL    string
	username  string
	sessionID string
	tools     []mcp.Tool
	mu        sync.Mutex
}

func NewMCPModel(ctx context.Context, username string, sessionID string) (*MCPModel, error) {
	key := os.Getenv("OPENAI_API_KEY")
	conf := config.GetConfig()

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: conf.RagModelConfig.RagBaseUrl,
		Model:   conf.RagModelConfig.RagChatModelName,
		APIKey:  key,
	})
	if err != nil {
		return nil, fmt.Errorf("create mcp model failed: %v", err)
	}

	mcpURL := conf.MCPServerURL()
	client, err := mcpclient.Dial(ctx, mcpURL, "GopherAI-MCPModel")
	if err != nil {
		return nil, fmt.Errorf("mcp client dial failed: %v", err)
	}

	m := &MCPModel{
		baseLLM:   llm,
		client:    client,
		mcpURL:    mcpURL,
		username:  username,
		sessionID: sessionID,
	}

	if err := m.discoverAndBindTools(ctx); err != nil {
		log.Printf("MCP tool discovery failed (will retry lazily): %v", err)
	}

	return m, nil
}

// discoverAndBindTools calls MCP tools/list and binds them to the LLM via WithTools.
func (m *MCPModel) discoverAndBindTools(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	toolsResult, err := m.client.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("mcp list tools failed: %v", err)
	}

	m.tools = toolsResult.Tools
	toolInfos := mcpconv.ConvertToolsToEino(m.tools)

	toolLLM, err := m.baseLLM.WithTools(toolInfos)
	if err != nil {
		return fmt.Errorf("bind tools to llm failed: %v", err)
	}
	m.toolLLM = toolLLM
	log.Printf("MCP: discovered and bound %d tools", len(m.tools))
	return nil
}

// ensureTools lazily retries tool discovery if the initial attempt failed.
func (m *MCPModel) ensureTools(ctx context.Context) model.ToolCallingChatModel {
	m.mu.Lock()
	if m.toolLLM != nil {
		llm := m.toolLLM
		m.mu.Unlock()
		return llm
	}
	m.mu.Unlock()

	if err := m.discoverAndBindTools(ctx); err != nil {
		log.Printf("MCP lazy tool discovery failed: %v", err)
		return m.baseLLM
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.toolLLM
}

func (m *MCPModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	llm := m.ensureTools(ctx)
	conversation := make([]*schema.Message, len(messages))
	copy(conversation, messages)

	for round := 0; round < mcpMaxToolRounds; round++ {
		resp, err := llm.Generate(ctx, conversation)
		if err != nil {
			return nil, fmt.Errorf("mcp generate failed (round %d): %v", round, err)
		}
		if len(resp.ToolCalls) == 0 {
			return resp, nil
		}
		conversation = append(conversation, resp)
		toolMsgs := m.executeToolCalls(ctx, resp.ToolCalls)
		conversation = append(conversation, toolMsgs...)
	}
	return nil, fmt.Errorf("mcp tool calling exceeded max rounds (%d)", mcpMaxToolRounds)
}

func (m *MCPModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages provided")
	}

	llm := m.ensureTools(ctx)
	conversation := make([]*schema.Message, len(messages))
	copy(conversation, messages)

	for round := 0; round < mcpMaxToolRounds; round++ {
		stream, err := llm.Stream(ctx, conversation)
		if err != nil {
			return "", fmt.Errorf("mcp stream failed (round %d): %v", round, err)
		}

		assembled, text, err := drainStream(stream, cb)
		if err != nil {
			return text, fmt.Errorf("mcp stream recv failed (round %d): %v", round, err)
		}
		if len(assembled.ToolCalls) == 0 {
			return text, nil
		}

		if cb != nil {
			for _, tc := range assembled.ToolCalls {
				cb(fmt.Sprintf("\n[调用工具: %s]\n", tc.Function.Name))
			}
		}

		conversation = append(conversation, assembled)
		toolMsgs := m.executeToolCalls(ctx, assembled.ToolCalls)
		conversation = append(conversation, toolMsgs...)
	}
	return "", fmt.Errorf("mcp tool calling exceeded max rounds (%d)", mcpMaxToolRounds)
}

// executeToolCalls calls each tool on the MCP server and returns Tool-role messages.
// Errors are captured per-tool rather than aborting the whole batch.
//
// The caller's ctx is enriched with a ToolCtx so MCP server-side handlers can
// identify the originating user/session. The trace_id field is left empty
// here; Step 8 of the overhaul plan wires per-request trace_id end-to-end.
func (m *MCPModel) executeToolCalls(ctx context.Context, toolCalls []schema.ToolCall) []*schema.Message {
	callCtx := mcpconv.WithToolCtx(ctx, mcpconv.ToolCtx{
		UserName:  m.username,
		SessionID: m.sessionID,
	})

	results := make([]*schema.Message, 0, len(toolCalls))
	for _, tc := range toolCalls {
		var args map[string]interface{}
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				results = append(results, &schema.Message{
					Role: schema.Tool, ToolCallID: tc.ID, ToolName: tc.Function.Name,
					Content: fmt.Sprintf("failed to parse arguments: %v", err),
				})
				continue
			}
		}

		result, err := m.client.CallTool(callCtx, tc.Function.Name, args)
		if err != nil {
			results = append(results, &schema.Message{
				Role: schema.Tool, ToolCallID: tc.ID, ToolName: tc.Function.Name,
				Content: fmt.Sprintf("tool call failed: %v", err),
			})
			continue
		}

		results = append(results, &schema.Message{
			Role: schema.Tool, ToolCallID: tc.ID, ToolName: tc.Function.Name,
			Content: mcpconv.ExtractToolResultText(result),
		})
	}
	return results
}

func (m *MCPModel) GetModelType() string { return "3" }

func (m *MCPModel) Close() error {
	if m.client != nil {
		m.client.Close()
	}
	return nil
}
