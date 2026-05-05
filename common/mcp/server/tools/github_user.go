package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

type githubUser struct {
	Login       string `json:"login"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	Company     string `json:"company"`
	Blog        string `json:"blog"`
	Location    string `json:"location"`
	Email       string `json:"email"`
	Bio         string `json:"bio"`
	PublicRepos int    `json:"public_repos"`
	PublicGists int    `json:"public_gists"`
	Followers   int    `json:"followers"`
	Following   int    `json:"following"`
	CreatedAt   string `json:"created_at"`
}

func registerGitHubUser(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"query_github_user",
		mcp.WithDescription("查询 GitHub 用户简介：仓库数 / follower / 公司 / 所在地 / 个人主页。"),
		mcp.WithString("login",
			mcp.Description("GitHub 用户名（不带 @），如 torvalds"),
			mcp.Required(),
		),
	)
	reg.Register(tool, handleGitHubUser)
}

func handleGitHubUser(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	login, err := stringArg(req, "login")
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.github.com/users/%s", login)
	var data githubUser
	if err := httpGetJSON(ctx, url, &data); err != nil {
		return nil, fmt.Errorf("query_github_user: %w", err)
	}

	text := fmt.Sprintf(
		"用户: %s%s\n地址: %s\n简介: %s\n公司: %s\n所在地: %s\n博客: %s\n公开仓库: %d  Gist: %d\nFollower: %d  Following: %d\n注册时间: %s",
		data.Login, formatDisplayName(data.Name),
		data.HTMLURL,
		emptyToDash(data.Bio),
		emptyToDash(data.Company),
		emptyToDash(data.Location),
		emptyToDash(data.Blog),
		data.PublicRepos, data.PublicGists,
		data.Followers, data.Following,
		emptyToDash(data.CreatedAt),
	)
	return textResult(text), nil
}

func formatDisplayName(name string) string {
	if name == "" {
		return ""
	}
	return " (" + name + ")"
}
