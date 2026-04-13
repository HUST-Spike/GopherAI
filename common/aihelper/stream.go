package aihelper

import (
	"io"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// drainStream reads a streaming response to completion, calling cb for each text chunk.
// Returns the fully assembled Message (including possible ToolCalls) and the accumulated text.
func drainStream(stream *schema.StreamReader[*schema.Message], cb StreamCallback) (*schema.Message, string, error) {
	defer stream.Close()

	var fullText strings.Builder
	var allToolCalls []schema.ToolCall

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fullText.String(), err
		}
		if len(chunk.Content) > 0 {
			fullText.WriteString(chunk.Content)
			if cb != nil {
				cb(chunk.Content)
			}
		}
		if len(chunk.ToolCalls) > 0 {
			allToolCalls = mergeToolCalls(allToolCalls, chunk.ToolCalls)
		}
	}

	assembled := &schema.Message{
		Role:      schema.Assistant,
		Content:   fullText.String(),
		ToolCalls: allToolCalls,
	}
	return assembled, fullText.String(), nil
}

// mergeToolCalls accumulates streaming tool call chunks into complete tool calls.
// Each chunk may carry partial function name / arguments keyed by Index.
func mergeToolCalls(existing []schema.ToolCall, incoming []schema.ToolCall) []schema.ToolCall {
	for _, tc := range incoming {
		idx := 0
		if tc.Index != nil {
			idx = *tc.Index
		}
		for idx >= len(existing) {
			existing = append(existing, schema.ToolCall{})
		}
		if tc.ID != "" {
			existing[idx].ID = tc.ID
		}
		if tc.Type != "" {
			existing[idx].Type = tc.Type
		}
		if tc.Index != nil {
			existing[idx].Index = tc.Index
		}
		existing[idx].Function.Name += tc.Function.Name
		existing[idx].Function.Arguments += tc.Function.Arguments
	}
	return existing
}
