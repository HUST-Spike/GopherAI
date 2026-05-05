package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type dictEntry struct {
	Word     string `json:"word"`
	Phonetic string `json:"phonetic"`
	Meanings []struct {
		PartOfSpeech string `json:"partOfSpeech"`
		Definitions  []struct {
			Definition string `json:"definition"`
			Example    string `json:"example,omitempty"`
		} `json:"definitions"`
		Synonyms []string `json:"synonyms"`
	} `json:"meanings"`
}

func registerDictionaryLookup(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"dictionary_lookup",
		mcp.WithDescription("查英文单词词典：词性 / 释义 / 例句 / 同义词。仅支持英文。"),
		mcp.WithString("word",
			mcp.Description("要查的英文单词，如 ephemeral"),
			mcp.Required(),
		),
	)
	reg.Register(tool, handleDictionaryLookup)
}

func handleDictionaryLookup(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	word, err := stringArg(req, "word")
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.dictionaryapi.dev/api/v2/entries/en/%s", word)
	var entries []dictEntry
	if err := httpGetJSON(ctx, url, &entries); err != nil {
		return nil, fmt.Errorf("dictionary_lookup: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("dictionary_lookup: no entry for %q", word)
	}

	var sb strings.Builder
	first := entries[0]
	fmt.Fprintf(&sb, "单词: %s\n音标: %s\n", first.Word, emptyToDash(first.Phonetic))
	for _, m := range first.Meanings {
		fmt.Fprintf(&sb, "\n[%s]\n", m.PartOfSpeech)
		for i, d := range m.Definitions {
			if i >= 3 {
				fmt.Fprintf(&sb, "  ... 还有 %d 条释义\n", len(m.Definitions)-i)
				break
			}
			fmt.Fprintf(&sb, "  %d. %s\n", i+1, d.Definition)
			if d.Example != "" {
				fmt.Fprintf(&sb, "     例: %s\n", d.Example)
			}
		}
		if len(m.Synonyms) > 0 {
			limit := len(m.Synonyms)
			if limit > 8 {
				limit = 8
			}
			fmt.Fprintf(&sb, "  同义词: %s\n", strings.Join(m.Synonyms[:limit], ", "))
		}
	}
	return textResult(sb.String()), nil
}
