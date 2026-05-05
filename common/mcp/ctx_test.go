package mcpconv

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestInjectCtxIntoArgs_PreservesOriginal(t *testing.T) {
	original := map[string]any{"query": "milvus"}
	tc := ToolCtx{UserName: "alice", SessionID: "s-1"}

	merged := InjectCtxIntoArgs(original, tc)

	if _, ok := original["_ctx"]; ok {
		t.Fatalf("InjectCtxIntoArgs must not mutate the input map")
	}
	if got := merged["query"]; got != "milvus" {
		t.Fatalf("user fields must be preserved, got %v", got)
	}
	ctxMap, ok := merged["_ctx"].(map[string]any)
	if !ok {
		t.Fatalf("expected _ctx to be a map, got %T", merged["_ctx"])
	}
	if ctxMap["user_name"] != "alice" || ctxMap["session_id"] != "s-1" {
		t.Fatalf("ctx fields not propagated, got %+v", ctxMap)
	}
	if _, hit := ctxMap["trace_id"]; hit {
		t.Fatalf("empty trace_id must be dropped, got %+v", ctxMap)
	}
}

func TestInjectCtxIntoArgs_NilArgs(t *testing.T) {
	merged := InjectCtxIntoArgs(nil, ToolCtx{UserName: "u"})
	if merged["_ctx"] == nil {
		t.Fatalf("expected _ctx to be present when args was nil")
	}
}

func TestInjectCtxIntoArgs_PreExistingCtxMerged(t *testing.T) {
	args := map[string]any{
		"_ctx": map[string]any{"trace_id": "t-pre"},
		"x":    1,
	}
	merged := InjectCtxIntoArgs(args, ToolCtx{UserName: "alice"})
	ctxMap := merged["_ctx"].(map[string]any)
	if ctxMap["user_name"] != "alice" {
		t.Fatalf("new user_name should win, got %+v", ctxMap)
	}
	if ctxMap["trace_id"] != "t-pre" {
		t.Fatalf("pre-existing trace_id should survive when ToolCtx.TraceID empty, got %+v", ctxMap)
	}
}

func TestExtractToolCtx_RoundTrip(t *testing.T) {
	src := ToolCtx{UserName: "bob", SessionID: "s-2", TraceID: "t-9"}
	args := InjectCtxIntoArgs(map[string]any{"k": "v"}, src)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "dummy",
			Arguments: args,
		},
	}

	got, ok := ExtractToolCtx(req)
	if !ok {
		t.Fatalf("ExtractToolCtx should report ok=true for non-empty ctx")
	}
	if got != src {
		t.Fatalf("round trip mismatch, got %+v want %+v", got, src)
	}
}

func TestExtractToolCtx_Missing(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "dummy",
			Arguments: map[string]any{"k": "v"},
		},
	}
	got, ok := ExtractToolCtx(req)
	if ok {
		t.Fatalf("expected ok=false when _ctx absent, got %+v", got)
	}
	if !got.IsZero() {
		t.Fatalf("expected zero ToolCtx, got %+v", got)
	}
}

func TestExtractToolCtx_WrongType(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "dummy",
			Arguments: map[string]any{"_ctx": "not-a-map"},
		},
	}
	if _, ok := ExtractToolCtx(req); ok {
		t.Fatalf("expected ok=false when _ctx is wrong type")
	}
}

func TestWithToolCtx_RoundTrip(t *testing.T) {
	tc := ToolCtx{UserName: "carol", SessionID: "s-3"}
	ctx := WithToolCtx(context.Background(), tc)
	got, ok := ToolCtxFrom(ctx)
	if !ok {
		t.Fatalf("ToolCtxFrom should hit when WithToolCtx was used")
	}
	if got != tc {
		t.Fatalf("ctx round trip mismatch, got %+v want %+v", got, tc)
	}
}

func TestToolCtxFrom_Empty(t *testing.T) {
	if _, ok := ToolCtxFrom(context.Background()); ok {
		t.Fatalf("ToolCtxFrom should miss on bare context")
	}
}
