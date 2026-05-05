package tools

import (
	mcpconv "GopherAI/common/mcp"
	mcpserver "GopherAI/common/mcp/server"
	docdao "GopherAI/dao/document"
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func registerListMyDocuments(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"list_my_documents",
		mcp.WithDescription("列出当前用户在当前会话上传的文档及其索引状态。"+
			"返回原始文件名、状态、片段数和上传时间，便于让用户确认 RAG 可用范围。"),
	)
	reg.Register(tool, handleListMyDocuments)
}

func handleListMyDocuments(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tc, ok := mcpconv.ExtractToolCtx(req)
	if !ok || tc.UserName == "" || tc.SessionID == "" {
		return nil, fmt.Errorf("list_my_documents: missing user/session context")
	}

	docs, err := docdao.ListDocumentsByUserAndSession(tc.UserName, tc.SessionID)
	if err != nil {
		return nil, fmt.Errorf("list_my_documents: query failed: %w", err)
	}
	if len(docs) == 0 {
		return textResult("当前会话还没有上传任何文档。"), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "当前会话共有 %d 个文档:\n", len(docs))
	for i, d := range docs {
		fmt.Fprintf(&sb,
			"\n[%d] %s\n    状态: %s | 片段: %d | 大小: %d bytes | 上传: %s\n    document_id=%s\n",
			i+1,
			d.OriginalFilename,
			d.Status,
			d.ChunkCount,
			d.FileSize,
			d.CreatedAt.Format("2006-01-02 15:04:05"),
			d.ID,
		)
	}
	return textResult(sb.String()), nil
}
