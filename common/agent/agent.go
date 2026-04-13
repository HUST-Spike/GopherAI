package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	defaultMaxSteps = 15
	roleThought     = "thought" // internal marker, not sent to LLM
)

// Agent implements a ReAct-style (Reason + Act) autonomous loop.
// It plans, executes tools, observes results, and iterates until the task is done.
type Agent struct {
	Name        string
	Description string
	LLM         model.ToolCallingChatModel
	MCPClient   *mcpclient.Client
	Tools       []mcp.Tool
	MaxSteps    int
	Memory      *Memory
}

// AgentConfig holds parameters for constructing an Agent.
type AgentConfig struct {
	Name        string
	Description string
	LLM         model.ToolCallingChatModel
	MCPClient   *mcpclient.Client
	Tools       []mcp.Tool
	MaxSteps    int
}

func NewAgent(cfg AgentConfig) (*Agent, error) {
	maxSteps := cfg.MaxSteps
	if maxSteps <= 0 {
		maxSteps = defaultMaxSteps
	}
	return &Agent{
		Name:        cfg.Name,
		Description: cfg.Description,
		LLM:         cfg.LLM,
		MCPClient:   cfg.MCPClient,
		Tools:       cfg.Tools,
		MaxSteps:    maxSteps,
		Memory:      NewMemory(),
	}, nil
}

// StepResult captures one iteration of the ReAct loop.
type StepResult struct {
	Step      int      `json:"step"`
	Action    string   `json:"action"`
	ToolName  string   `json:"tool_name,omitempty"`
	ToolArgs  string   `json:"tool_args,omitempty"`
	ToolResult string  `json:"tool_result,omitempty"`
	Thought   string   `json:"thought,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// AgentResult is the final outcome of an agent execution.
type AgentResult struct {
	FinalAnswer string       `json:"final_answer"`
	Steps       []StepResult `json:"steps"`
	TotalSteps  int          `json:"total_steps"`
}

// StreamCallback is called for each text chunk during streaming execution.
type StreamCallback func(event AgentEvent)

// AgentEvent represents a real-time event emitted during agent execution.
type AgentEvent struct {
	Type    string `json:"type"` // "thought", "tool_call", "tool_result", "answer", "error"
	Content string `json:"content"`
	Step    int    `json:"step"`
}

// Execute runs the agent's ReAct loop to completion.
func (a *Agent) Execute(ctx context.Context, task string) (*AgentResult, error) {
	return a.executeWithCallback(ctx, task, nil)
}

// StreamExecute runs the agent loop, emitting events via callback for real-time UI.
func (a *Agent) StreamExecute(ctx context.Context, task string, cb StreamCallback) (*AgentResult, error) {
	return a.executeWithCallback(ctx, task, cb)
}

func (a *Agent) executeWithCallback(ctx context.Context, task string, cb StreamCallback) (*AgentResult, error) {
	conversation := a.buildInitialMessages(task)
	var steps []StepResult

	toolInfos := convertMCPToolsToEino(a.Tools)
	llmWithTools, err := a.LLM.WithTools(toolInfos)
	if err != nil {
		return nil, fmt.Errorf("agent bind tools failed: %v", err)
	}

	for step := 1; step <= a.MaxSteps; step++ {
		// Generate
		resp, err := llmWithTools.Generate(ctx, conversation)
		if err != nil {
			return nil, fmt.Errorf("agent generate failed at step %d: %v", step, err)
		}

		// No tool calls → final answer
		if len(resp.ToolCalls) == 0 {
			if cb != nil {
				cb(AgentEvent{Type: "answer", Content: resp.Content, Step: step})
			}
			a.Memory.AddEntry(MemoryEntry{
				Role:    "assistant",
				Content: resp.Content,
				Time:    time.Now(),
			})
			return &AgentResult{
				FinalAnswer: resp.Content,
				Steps:       steps,
				TotalSteps:  step,
			}, nil
		}

		// Has tool calls → execute them
		conversation = append(conversation, resp)

		for _, tc := range resp.ToolCalls {
			if cb != nil {
				cb(AgentEvent{
					Type:    "tool_call",
					Content: fmt.Sprintf("调用工具 %s: %s", tc.Function.Name, tc.Function.Arguments),
					Step:    step,
				})
			}

			toolResult, err := a.callTool(ctx, tc)
			resultContent := ""
			if err != nil {
				resultContent = fmt.Sprintf("工具调用失败: %v", err)
				log.Printf("agent tool call %s failed: %v", tc.Function.Name, err)
			} else {
				resultContent = toolResult
			}

			conversation = append(conversation, &schema.Message{
				Role:       schema.Tool,
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    resultContent,
			})

			if cb != nil {
				cb(AgentEvent{
					Type:    "tool_result",
					Content: resultContent,
					Step:    step,
				})
			}

			steps = append(steps, StepResult{
				Step:       step,
				Action:     "tool_call",
				ToolName:   tc.Function.Name,
				ToolArgs:   tc.Function.Arguments,
				ToolResult: resultContent,
				Timestamp:  time.Now(),
			})

			a.Memory.AddEntry(MemoryEntry{
				Role:     "tool",
				Content:  resultContent,
				ToolName: tc.Function.Name,
				Time:     time.Now(),
			})
		}
	}

	return nil, fmt.Errorf("agent exceeded max steps (%d)", a.MaxSteps)
}

func (a *Agent) buildInitialMessages(task string) []*schema.Message {
	systemPrompt := a.buildSystemPrompt()
	messages := []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
	}

	// Inject long-term memory context if available
	memoryContext := a.Memory.GetSummary()
	if memoryContext != "" {
		messages = append(messages, &schema.Message{
			Role:    schema.System,
			Content: "历史记忆摘要:\n" + memoryContext,
		})
	}

	messages = append(messages, &schema.Message{
		Role:    schema.User,
		Content: task,
	})
	return messages
}

func (a *Agent) buildSystemPrompt() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是 %s，一个智能代理（Agent）。\n", a.Name))
	if a.Description != "" {
		sb.WriteString(a.Description + "\n\n")
	}
	sb.WriteString(`你的工作方式：
1. 分析用户的任务需求
2. 制定执行计划
3. 使用可用的工具逐步完成任务
4. 每一步都要思考结果是否符合预期
5. 如果工具调用失败，尝试其他方案
6. 任务完成后，给出完整的最终回答

注意事项：
- 每次只执行一个工具调用
- 工具结果会返回给你，据此决定下一步
- 如果无需工具即可回答，直接回答即可
`)

	if len(a.Tools) > 0 {
		sb.WriteString("\n可用工具:\n")
		for _, t := range a.Tools {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Name, t.Description))
		}
	}

	return sb.String()
}

func (a *Agent) callTool(ctx context.Context, tc schema.ToolCall) (string, error) {
	var args map[string]interface{}
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse args failed: %v", err)
		}
	}

	callReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      tc.Function.Name,
			Arguments: args,
		},
	}
	result, err := a.MCPClient.CallTool(ctx, callReq)
	if err != nil {
		return "", err
	}

	var text strings.Builder
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			text.WriteString(textContent.Text)
			text.WriteByte('\n')
		}
	}
	return text.String(), nil
}

// convertMCPToolsToEino mirrors the conversion in aihelper/model.go
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
