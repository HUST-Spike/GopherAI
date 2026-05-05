package aihelper

import (
	mcpconv "GopherAI/common/mcp"
	mcpclient "GopherAI/common/mcp/client"
	mcprunner "GopherAI/common/mcp/runner"
	mcpserver "GopherAI/common/mcp/server"
	"GopherAI/common/rag"
	"GopherAI/common/skill"
	"GopherAI/config"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/mcp"
)

// StreamCallback is the unified callback every streaming model invokes.
// Backends emit StreamEvent values (see event.go) so the SSE wire format is
// the single source of truth: token chunks, tool_call markers, tool_result
// markers, error and done events all share one envelope.
type StreamCallback func(event StreamEvent)

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
	// allTools is the raw set of tools the MCP server exposes (cached after
	// first ListTools). Filtering happens per-call against active skills.
	allTools []mcp.Tool
	// boundSig is a stable signature of the tool-name set currently bound
	// to toolLLM. Used to skip a redundant WithTools when the visible set
	// hasn't changed between calls.
	boundSig string
	mu       sync.Mutex
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

	if err := m.refreshAllTools(ctx); err != nil {
		log.Printf("MCP tool discovery failed (will retry lazily): %v", err)
	}

	return m, nil
}

// refreshAllTools repopulates m.allTools by calling MCP tools/list. This is
// the authoritative list of every tool the server exposes, regardless of
// session-level skill activation. Per-call filtering uses VisibleTools.
func (m *MCPModel) refreshAllTools(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	toolsResult, err := m.client.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("mcp list tools failed: %v", err)
	}
	m.allTools = toolsResult.Tools
	m.boundSig = ""
	m.toolLLM = nil
	log.Printf("MCP: discovered %d tools", len(m.allTools))
	return nil
}

// ensureTools makes sure toolLLM is bound to exactly the set of tools
// visible under the session's *current* active skills. It is cheap on the
// hot path: when the skill set hasn't changed it just returns the cached
// toolLLM without touching the upstream model.
func (m *MCPModel) ensureTools(ctx context.Context) model.ToolCallingChatModel {
	m.mu.Lock()
	if len(m.allTools) == 0 {
		m.mu.Unlock()
		if err := m.refreshAllTools(ctx); err != nil {
			log.Printf("MCP lazy tool discovery failed: %v", err)
			return m.baseLLM
		}
		m.mu.Lock()
	}

	activeSkills := skill.GetGlobalSkillManager().GetActiveSkillNames(m.sessionID)
	visible := mcpserver.DefaultRegistry.VisibleTools(m.allTools, activeSkills)
	sig := toolsSignature(visible)

	if m.toolLLM != nil && sig == m.boundSig {
		llm := m.toolLLM
		m.mu.Unlock()
		return llm
	}
	m.mu.Unlock()

	toolInfos := mcpconv.ConvertToolsToEino(visible)
	toolLLM, err := m.baseLLM.WithTools(toolInfos)
	if err != nil {
		log.Printf("MCP bind tools failed: %v (falling back to baseLLM)", err)
		return m.baseLLM
	}

	m.mu.Lock()
	m.toolLLM = toolLLM
	m.boundSig = sig
	m.mu.Unlock()
	log.Printf("MCP: bound %d/%d tools (active_skills=%v)", len(visible), len(m.allTools), activeSkills)
	return toolLLM
}

// toolsSignature builds a stable, order-independent signature from a tool
// set's names so we can compare two binds without diffing whole structs.
func toolsSignature(tools []mcp.Tool) string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}

func (m *MCPModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	return m.generateWithRoundCap(ctx, messages, mcpMaxToolRounds)
}

// generateWithRoundCap runs the tool-calling loop with an explicit upper
// bound. SmartModel calls this with maxRounds=1 to enforce single-shot
// function calling when ENABLE_AGENT_LOOP=false.
func (m *MCPModel) generateWithRoundCap(ctx context.Context, messages []*schema.Message, maxRounds int) (*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	if maxRounds <= 0 {
		maxRounds = 1
	}

	llm := m.ensureTools(ctx)
	conversation := make([]*schema.Message, len(messages))
	copy(conversation, messages)

	var lastResp *schema.Message
	for round := 0; round < maxRounds; round++ {
		resp, err := llm.Generate(ctx, conversation)
		if err != nil {
			return nil, fmt.Errorf("mcp generate failed (round %d): %v", round, err)
		}
		lastResp = resp
		if len(resp.ToolCalls) == 0 {
			return resp, nil
		}
		conversation = append(conversation, resp)
		toolMsgs := m.executeToolCallsWithCallback(ctx, resp.ToolCalls, nil)
		conversation = append(conversation, toolMsgs...)
	}
	// Round budget exhausted with the LLM still trying to call a tool: hand
	// back the last assistant message rather than erroring out, so the
	// caller can still surface partial reasoning. Single-round mode hits
	// this every time the LLM picks a tool, which is exactly the desired
	// "single function call, no iteration" behavior.
	if lastResp != nil {
		return lastResp, nil
	}
	return nil, fmt.Errorf("mcp tool calling exceeded max rounds (%d)", maxRounds)
}

func (m *MCPModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	return m.streamWithRoundCap(ctx, messages, cb, mcpMaxToolRounds)
}

// streamWithRoundCap mirrors generateWithRoundCap for the streaming path.
// Single-round mode is what SmartModel uses when ENABLE_AGENT_LOOP=false.
func (m *MCPModel) streamWithRoundCap(ctx context.Context, messages []*schema.Message, cb StreamCallback, maxRounds int) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages provided")
	}
	if maxRounds <= 0 {
		maxRounds = 1
	}

	llm := m.ensureTools(ctx)
	conversation := make([]*schema.Message, len(messages))
	copy(conversation, messages)

	var lastText string
	for round := 0; round < maxRounds; round++ {
		stream, err := llm.Stream(ctx, conversation)
		if err != nil {
			return "", fmt.Errorf("mcp stream failed (round %d): %v", round, err)
		}

		assembled, text, err := drainStream(stream, cb)
		if err != nil {
			return text, fmt.Errorf("mcp stream recv failed (round %d): %v", round, err)
		}
		lastText = text
		if len(assembled.ToolCalls) == 0 {
			return text, nil
		}

		conversation = append(conversation, assembled)
		toolMsgs := m.executeToolCallsWithCallback(ctx, assembled.ToolCalls, cb)
		conversation = append(conversation, toolMsgs...)
	}
	return lastText, nil
}

// executeToolCalls calls each tool on the MCP server and returns Tool-role messages.
// Errors are captured per-tool rather than aborting the whole batch.
//
// The caller's ctx is enriched with a ToolCtx so MCP server-side handlers can
// identify the originating user/session. The trace_id field is left empty
// here; Step 8 of the overhaul plan wires per-request trace_id end-to-end.
// executeToolCallsWithCallback runs every tool call the LLM emitted in this
// round through the runner and surfaces tool_call / tool_result events to
// cb if one is provided. cb may be nil (non-streaming Generate path); the
// MySQL-backed tool_invocations ledger is written either way.
func (m *MCPModel) executeToolCallsWithCallback(
	ctx context.Context,
	toolCalls []schema.ToolCall,
	cb StreamCallback,
) []*schema.Message {
	traceID, _ := ctx.Value(traceIDCtxKey{}).(string)

	callCtx := mcpconv.WithToolCtx(ctx, mcpconv.ToolCtx{
		UserName:  m.username,
		SessionID: m.sessionID,
		TraceID:   traceID,
	})

	activeSkills := skill.GetGlobalSkillManager().GetActiveSkillNames(m.sessionID)

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

		if cb != nil {
			cb(ToolCallEvent(tc.Function.Name, tc.ID, tc.Function.Arguments))
		}

		start := time.Now()
		res := mcprunner.Run(callCtx, m.client, tc.Function.Name, args, mcprunner.Options{
			ToolCallID:   tc.ID,
			TraceID:      traceID,
			ActiveSkills: activeSkills,
			ModelType:    m.GetModelType(),
		})
		duration := int(time.Since(start).Milliseconds())

		if cb != nil {
			cb(ToolResultEvent(
				tc.Function.Name,
				tc.ID,
				string(res.Status),
				summarizeText(res.Text, 200),
				res.Attempts,
				duration,
			))
		}

		results = append(results, &schema.Message{
			Role: schema.Tool, ToolCallID: tc.ID, ToolName: tc.Function.Name,
			Content: res.Text,
		})
	}
	return results
}

// summarizeText returns a leading slice of s suitable for the SSE preview
// field. We measure by runes so multi-byte characters aren't truncated mid-
// codepoint, which would otherwise produce garbage in the browser.
func summarizeText(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

// traceIDCtxKey is a private type used to carry a per-request trace_id down
// the call stack without exporting a key string. Step 8 of the plan threads
// trace_id all the way from the HTTP layer; for now this keeps the runner
// happy with whatever is on ctx and falls back to "" otherwise.
type traceIDCtxKey struct{}

// WithTraceID returns ctx tagged with traceID so MCPModel.executeToolCalls
// can forward it to the runner. Exported for service-layer wiring (Step 8).
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDCtxKey{}, traceID)
}

func (m *MCPModel) GetModelType() string { return "3" }

func (m *MCPModel) Close() error {
	if m.client != nil {
		m.client.Close()
	}
	return nil
}
