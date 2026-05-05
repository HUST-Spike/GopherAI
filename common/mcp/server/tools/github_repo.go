package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

type githubRepo struct {
	FullName        string `json:"full_name"`
	Description     string `json:"description"`
	HTMLURL         string `json:"html_url"`
	Language        string `json:"language"`
	StargazersCount int    `json:"stargazers_count"`
	ForksCount      int    `json:"forks_count"`
	OpenIssues      int    `json:"open_issues_count"`
	Topics          []string `json:"topics"`
	License         struct {
		SPDXID string `json:"spdx_id"`
	} `json:"license"`
	UpdatedAt   string `json:"updated_at"`
	PushedAt    string `json:"pushed_at"`
	Archived    bool   `json:"archived"`
	Disabled    bool   `json:"disabled"`
	Fork        bool   `json:"fork"`
	HomepageURL string `json:"homepage"`
}

func registerGitHubRepo(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"query_github_repo",
		mcp.WithDescription("查询 GitHub 仓库信息：star / fork / 主语言 / 描述 / 最近推送时间。"),
		mcp.WithString("owner",
			mcp.Description("仓库所属用户名或组织名，如 cloudwego"),
			mcp.Required(),
		),
		mcp.WithString("repo",
			mcp.Description("仓库名，如 eino"),
			mcp.Required(),
		),
	)
	reg.Register(tool, handleGitHubRepo)
}

func handleGitHubRepo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	owner, err := stringArg(req, "owner")
	if err != nil {
		return nil, err
	}
	repo, err := stringArg(req, "repo")
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	var data githubRepo
	if err := httpGetJSON(ctx, url, &data); err != nil {
		return nil, fmt.Errorf("query_github_repo: %w", err)
	}

	flags := ""
	if data.Archived {
		flags += " [archived]"
	}
	if data.Fork {
		flags += " [fork]"
	}
	if data.Disabled {
		flags += " [disabled]"
	}

	text := fmt.Sprintf(
		"仓库: %s%s\n地址: %s\n描述: %s\n主页: %s\n主语言: %s\nStar: %d  Fork: %d  Open Issue: %d\n许可证: %s\nTopics: %v\n最近推送: %s\n更新时间: %s",
		data.FullName, flags,
		data.HTMLURL,
		emptyToDash(data.Description),
		emptyToDash(data.HomepageURL),
		emptyToDash(data.Language),
		data.StargazersCount, data.ForksCount, data.OpenIssues,
		emptyToDash(data.License.SPDXID),
		data.Topics,
		emptyToDash(data.PushedAt),
		emptyToDash(data.UpdatedAt),
	)
	return textResult(text), nil
}

func emptyToDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
