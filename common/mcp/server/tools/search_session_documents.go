package tools

import (
	mcpconv "GopherAI/common/mcp"
	mcpserver "GopherAI/common/mcp/server"
	"GopherAI/common/rag"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
)

// retrieverOnce / sharedRetriever lazily build a single Retriever the first
// time the tool is invoked. Building a Retriever is non-trivial (constructs
// embedder + reranker), so we reuse it across calls. The Retriever's per-call
// dependency on the live Milvus client is reopened inside Retrieve, so no
// long-lived connection state escapes.
var (
	retrieverOnce sync.Once
	sharedRetr    *rag.Retriever
	retrErr       error
)

func getRetriever() (*rag.Retriever, error) {
	retrieverOnce.Do(func() {
		sharedRetr, retrErr = rag.NewRetriever(rag.LoadConfigFromEnv())
	})
	return sharedRetr, retrErr
}

func registerSearchSessionDocuments(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"search_session_documents",
		mcp.WithDescription("在当前用户当前会话上传的文档中检索与 query 最相关的片段。"+
			"严格按 user_name + session_id 隔离，不会跨用户或跨会话召回。"),
		mcp.WithString("query",
			mcp.Description("要检索的自然语言问题或关键词"),
			mcp.Required(),
		),
		mcp.WithNumber("top_k",
			mcp.Description("返回最相关的 N 段，默认 5，最大 10"),
		),
	)
	reg.Register(tool, handleSearchSessionDocuments)
}

func handleSearchSessionDocuments(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := stringArg(req, "query")
	if err != nil {
		return nil, err
	}
	tc, ok := mcpconv.ExtractToolCtx(req)
	if !ok || tc.UserName == "" || tc.SessionID == "" {
		return nil, fmt.Errorf("search_session_documents: missing user/session context")
	}
	topK := optionalIntArg(req, "top_k", 5)
	if topK <= 0 {
		topK = 5
	}
	if topK > 10 {
		topK = 10
	}

	retriever, err := getRetriever()
	if err != nil {
		return nil, fmt.Errorf("search_session_documents: retriever init failed: %w", err)
	}

	chunks, err := retriever.Retrieve(ctx, tc.UserName, tc.SessionID, query)
	if err != nil {
		return nil, fmt.Errorf("search_session_documents: %w", err)
	}
	if len(chunks) > topK {
		chunks = chunks[:topK]
	}
	if len(chunks) == 0 {
		return textResult(fmt.Sprintf("未在当前会话的文档中找到与 %q 相关的内容。", query)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "查询: %s\n命中 %d 段:\n", query, len(chunks))
	for i, c := range chunks {
		preview := previewChunk(c.Content, 400)
		fmt.Fprintf(&sb,
			"\n[%d] 文档: %s | chunk_index=%d | score=%.4f | chunk_id=%s\n%s\n",
			i+1,
			emptyToDash(c.OriginalFilename),
			c.ChunkIndex,
			c.Score,
			c.ChunkID,
			preview,
		)
	}
	return textResult(sb.String()), nil
}

// previewChunk gives the LLM a tight excerpt rather than the full chunk so
// 5 hits don't blow the result budget. Whole chunks are still in Milvus and
// the upstream RAG mode (Step 9 SmartModel) can still inject them verbatim.
func previewChunk(content string, max int) string {
	content = strings.TrimSpace(content)
	if len([]rune(content)) <= max {
		return content
	}
	runes := []rune(content)
	return string(runes[:max]) + "..."
}
