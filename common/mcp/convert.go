package mcpconv

import (
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/mcp"
)

// ConvertToolsToEino converts MCP tool definitions to Eino ToolInfo for LLM binding.
func ConvertToolsToEino(mcpTools []mcp.Tool) []*schema.ToolInfo {
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

// ExtractToolResultText extracts text content from an MCP CallToolResult.
func ExtractToolResultText(result *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			sb.WriteString(textContent.Text)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
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
