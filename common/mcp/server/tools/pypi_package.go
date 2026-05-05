package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

type pypiInfo struct {
	Info struct {
		Name        string   `json:"name"`
		Version     string   `json:"version"`
		Summary     string   `json:"summary"`
		Author      string   `json:"author"`
		HomePage    string   `json:"home_page"`
		ProjectURL  string   `json:"project_url"`
		ProjectURLs map[string]string `json:"project_urls"`
		License     string   `json:"license"`
		Keywords    string   `json:"keywords"`
		Classifiers []string `json:"classifiers"`
	} `json:"info"`
}

func registerPyPIPackage(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"lookup_pypi_package",
		mcp.WithDescription("查询 PyPI 上 Python 包的最新版本、简介、作者与项目主页。"),
		mcp.WithString("name",
			mcp.Description("包名，如 numpy、llama-index"),
			mcp.Required(),
		),
	)
	reg.Register(tool, handlePyPIPackage)
}

func handlePyPIPackage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := stringArg(req, "name")
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", name)
	var data pypiInfo
	if err := httpGetJSON(ctx, url, &data); err != nil {
		return nil, fmt.Errorf("lookup_pypi_package: %w", err)
	}

	homepage := data.Info.HomePage
	if homepage == "" {
		homepage = data.Info.ProjectURL
	}

	text := fmt.Sprintf(
		"PyPI 包: %s\n最新版本: %s\n简介: %s\n作者: %s\n主页: %s\n项目地址: https://pypi.org/project/%s/\n许可证: %s\n关键词: %s",
		data.Info.Name,
		data.Info.Version,
		emptyToDash(data.Info.Summary),
		emptyToDash(data.Info.Author),
		emptyToDash(homepage),
		data.Info.Name,
		emptyToDash(data.Info.License),
		emptyToDash(data.Info.Keywords),
	)
	return textResult(text), nil
}
