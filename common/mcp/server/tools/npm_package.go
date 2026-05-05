package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type npmPackage struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Homepage    string            `json:"homepage"`
	License     string            `json:"license"`
	Keywords    []string          `json:"keywords"`
	DistTags    map[string]string `json:"dist-tags"`
	Repository  struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"repository"`
	Maintainers []struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"maintainers"`
}

func registerNPMPackage(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"lookup_npm_package",
		mcp.WithDescription("查询 npm 上 JavaScript / TypeScript 包的最新版本、描述、维护者。"),
		mcp.WithString("name",
			mcp.Description("包名，支持 scope，如 vue、@vue/cli"),
			mcp.Required(),
		),
	)
	reg.Register(tool, handleNPMPackage)
}

func handleNPMPackage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := stringArg(req, "name")
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://registry.npmjs.org/%s", name)
	var data npmPackage
	if err := httpGetJSON(ctx, url, &data); err != nil {
		return nil, fmt.Errorf("lookup_npm_package: %w", err)
	}

	maintainers := make([]string, 0, len(data.Maintainers))
	for _, m := range data.Maintainers {
		maintainers = append(maintainers, m.Name)
	}

	text := fmt.Sprintf(
		"npm 包: %s\n最新版本: %s\n描述: %s\n主页: %s\n仓库: %s\n许可证: %s\n维护者: %s\n关键词: %v\n项目地址: https://www.npmjs.com/package/%s",
		data.Name,
		emptyToDash(data.DistTags["latest"]),
		emptyToDash(data.Description),
		emptyToDash(data.Homepage),
		emptyToDash(data.Repository.URL),
		emptyToDash(data.License),
		strings.Join(maintainers, ", "),
		data.Keywords,
		data.Name,
	)
	return textResult(text), nil
}
