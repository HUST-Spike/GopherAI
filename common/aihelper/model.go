package aihelper

import (
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
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
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

// =================== RAG ===================

type AliRAGModel struct {
	llm      model.ToolCallingChatModel
	username string
}

func NewAliRAGModel(ctx context.Context, username string) (*AliRAGModel, error) {
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
		return nil, fmt.Errorf("create ali rag model failed: %v", err)
	}
	return &AliRAGModel{llm: llm, username: username}, nil
}

func (o *AliRAGModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	ragMessages, err := o.buildRAGMessages(ctx, messages)
	if err != nil {
		log.Printf("RAG enrichment failed, falling back to plain LLM: %v", err)
		ragMessages = messages
	}
	resp, err := o.llm.Generate(ctx, ragMessages)
	if err != nil {
		return nil, fmt.Errorf("ali rag generate failed: %v", err)
	}
	return resp, nil
}

func (o *AliRAGModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	ragMessages, err := o.buildRAGMessages(ctx, messages)
	if err != nil {
		log.Printf("RAG enrichment failed, falling back to plain LLM: %v", err)
		ragMessages = messages
	}
	stream, err := o.llm.Stream(ctx, ragMessages)
	if err != nil {
		return "", fmt.Errorf("ali rag stream failed: %v", err)
	}
	_, text, err := drainStream(stream, cb)
	if err != nil {
		return text, fmt.Errorf("ali rag stream recv failed: %v", err)
	}
	return text, nil
}

// buildRAGMessages retrieves documents and injects them into the last user message.
func (o *AliRAGModel) buildRAGMessages(ctx context.Context, messages []*schema.Message) ([]*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	ragQuery, err := rag.NewRAGQuery(ctx, o.username)
	if err != nil {
		return nil, err
	}

	lastMessage := messages[len(messages)-1]
	query := lastMessage.Content

	docs, err := ragQuery.RetrieveDocuments(ctx, query)
	if err != nil {
		return nil, err
	}

	ragPrompt := rag.BuildRAGPrompt(query, docs)
	enriched := make([]*schema.Message, len(messages))
	copy(enriched, messages)
	enriched[len(enriched)-1] = &schema.Message{
		Role:    schema.User,
		Content: ragPrompt,
	}
	return enriched, nil
}

func (o *AliRAGModel) GetModelType() string { return "2" }
func (o *AliRAGModel) Close() error         { return nil }

// =================== MCP (Native Function Calling) ===================

const mcpMaxToolRounds = 10

type MCPModel struct {
	baseLLM   model.ToolCallingChatModel // LLM without tools bound
	toolLLM   model.ToolCallingChatModel // LLM with MCP tools bound
	mcpClient *mcpclient.Client
	mcpURL    string
	username  string
	tools     []mcp.Tool // cached tool definitions from MCP server
	mu        sync.Mutex
}

func NewMCPModel(ctx context.Context, username string) (*MCPModel, error) {
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
		return nil, fmt.Errorf("create mcp model failed: %v", err)
	}

	mcpURL := conf.MCPServerURL()

	m := &MCPModel{
		baseLLM:  llm,
		mcpURL:   mcpURL,
		username: username,
	}

	if err := m.initMCPClient(ctx); err != nil {
		return nil, fmt.Errorf("mcp client init failed: %v", err)
	}

	if err := m.discoverAndBindTools(ctx); err != nil {
		log.Printf("MCP tool discovery failed (will retry on first call): %v", err)
	}

	return m, nil
}

func (m *MCPModel) initMCPClient(ctx context.Context) error {
	httpTransport, err := transport.NewStreamableHTTP(m.mcpURL)
	if err != nil {
		return fmt.Errorf("create mcp transport failed: %v", err)
	}

	m.mcpClient = mcpclient.NewClient(httpTransport)

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "GopherAI-MCPClient",
		Version: "2.0.0",
	}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	if _, err := m.mcpClient.Initialize(ctx, initReq); err != nil {
		return fmt.Errorf("mcp initialize failed: %v", err)
	}
	return nil
}

// discoverAndBindTools calls MCP tools/list and binds them to the LLM via WithTools.
func (m *MCPModel) discoverAndBindTools(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	toolsResult, err := m.mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return fmt.Errorf("mcp list tools failed: %v", err)
	}

	m.tools = toolsResult.Tools
	toolInfos := convertMCPToolsToEino(m.tools)

	toolLLM, err := m.baseLLM.WithTools(toolInfos)
	if err != nil {
		return fmt.Errorf("bind tools to llm failed: %v", err)
	}
	m.toolLLM = toolLLM
	log.Printf("MCP: discovered and bound %d tools", len(m.tools))
	return nil
}

func (m *MCPModel) getLLM() model.ToolCallingChatModel {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.toolLLM != nil {
		return m.toolLLM
	}
	return m.baseLLM
}

// GenerateResponse runs the tool-calling loop: generate → call tools → feed results → repeat.
func (m *MCPModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	llm := m.getLLM()
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

		toolMsgs, err := m.executeToolCalls(ctx, resp.ToolCalls)
		if err != nil {
			return nil, fmt.Errorf("mcp tool execution failed (round %d): %v", round, err)
		}
		conversation = append(conversation, toolMsgs...)
	}

	return nil, fmt.Errorf("mcp tool calling exceeded max rounds (%d)", mcpMaxToolRounds)
}

// StreamResponse runs the tool-calling loop with streaming for the final text response.
func (m *MCPModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages provided")
	}

	llm := m.getLLM()
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

		// Tool calls detected — notify frontend, execute, then loop
		if cb != nil {
			for _, tc := range assembled.ToolCalls {
				cb(fmt.Sprintf("\n[调用工具: %s]\n", tc.Function.Name))
			}
		}

		conversation = append(conversation, assembled)

		toolMsgs, err := m.executeToolCalls(ctx, assembled.ToolCalls)
		if err != nil {
			return "", fmt.Errorf("mcp tool execution failed (round %d): %v", round, err)
		}
		conversation = append(conversation, toolMsgs...)
	}

	return "", fmt.Errorf("mcp tool calling exceeded max rounds (%d)", mcpMaxToolRounds)
}

// executeToolCalls calls each tool on the MCP server and returns Tool-role messages.
func (m *MCPModel) executeToolCalls(ctx context.Context, toolCalls []schema.ToolCall) ([]*schema.Message, error) {
	results := make([]*schema.Message, 0, len(toolCalls))

	for _, tc := range toolCalls {
		var args map[string]interface{}
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				results = append(results, &schema.Message{
					Role:       schema.Tool,
					ToolCallID: tc.ID,
					ToolName:   tc.Function.Name,
					Content:    fmt.Sprintf("failed to parse arguments: %v", err),
				})
				continue
			}
		}

		callReq := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      tc.Function.Name,
				Arguments: args,
			},
		}
		result, err := m.mcpClient.CallTool(ctx, callReq)
		if err != nil {
			results = append(results, &schema.Message{
				Role:       schema.Tool,
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    fmt.Sprintf("tool call failed: %v", err),
			})
			continue
		}

		results = append(results, &schema.Message{
			Role:       schema.Tool,
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Content:    extractToolResultText(result),
		})
	}
	return results, nil
}

func (m *MCPModel) GetModelType() string { return "3" }

func (m *MCPModel) Close() error {
	if m.mcpClient != nil {
		m.mcpClient.Close()
	}
	return nil
}

// =================== MCP ↔ Eino conversion helpers ===================

// convertMCPToolsToEino converts MCP tool definitions to Eino ToolInfo for LLM binding.
func convertMCPToolsToEino(mcpTools []mcp.Tool) []*schema.ToolInfo {
	infos := make([]*schema.ToolInfo, 0, len(mcpTools))
	for _, t := range mcpTools {
		info := &schema.ToolInfo{
			Name: t.Name,
			Desc: t.Description,
		}

		if t.InputSchema.Properties != nil {
			params := make(map[string]*schema.ParameterInfo)
			requiredSet := make(map[string]bool)
			for _, r := range t.InputSchema.Required {
				requiredSet[r] = true
			}

			for name, propRaw := range t.InputSchema.Properties {
				propBytes, err := json.Marshal(propRaw)
				if err != nil {
					continue
				}
				var prop struct {
					Type        string `json:"type"`
					Description string `json:"description"`
				}
				if err := json.Unmarshal(propBytes, &prop); err != nil {
					continue
				}
				params[name] = &schema.ParameterInfo{
					Type:     mapJSONTypeToEino(prop.Type),
					Desc:     prop.Description,
					Required: requiredSet[name],
				}
			}
			info.ParamsOneOf = schema.NewParamsOneOfByParams(params)
		}

		infos = append(infos, info)
	}
	return infos
}

func mapJSONTypeToEino(jsonType string) schema.DataType {
	switch jsonType {
	case "string":
		return schema.String
	case "integer":
		return schema.Integer
	case "number":
		return schema.Number
	case "boolean":
		return schema.Boolean
	case "array":
		return schema.Array
	case "object":
		return schema.Object
	default:
		return schema.String
	}
}

func extractToolResultText(result *mcp.CallToolResult) string {
	var text string
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			text += textContent.Text + "\n"
		}
	}
	return text
}
