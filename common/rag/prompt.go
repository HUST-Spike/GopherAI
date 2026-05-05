package rag

import (
	"fmt"
	"strings"
)

func BuildPrompt(query string, chunks []RetrievedChunk, maxContextChars int) (string, error) {
	if len(chunks) == 0 {
		return "", fmt.Errorf("no retrieved chunks")
	}
	if maxContextChars <= 0 {
		maxContextChars = defaultMaxContextChars
	}

	var builder strings.Builder
	used := 0
	for i, chunk := range chunks {
		content := strings.TrimSpace(chunk.Content)
		if content == "" {
			continue
		}

		header := fmt.Sprintf("[参考资料 %d | 文件: %s | chunk: %d | score: %.4f]\n", i+1, chunk.OriginalFilename, chunk.ChunkIndex, chunk.Score)
		remaining := maxContextChars - used - runeLen(header) - 2
		if remaining <= 0 {
			break
		}
		content = truncateRunes(content, remaining)

		builder.WriteString(header)
		builder.WriteString(content)
		builder.WriteString("\n\n")
		used += runeLen(header) + runeLen(content) + 2
	}

	contextText := strings.TrimSpace(builder.String())
	if contextText == "" {
		return "", fmt.Errorf("retrieved chunks are empty after context trimming")
	}

	return fmt.Sprintf(`你是 GopherAI 的 RAG 问答助手。请严格基于“参考资料”回答用户问题。

要求：
1. 如果参考资料中没有足够依据，请直接说明没有找到相关信息。
2. 不要编造参考资料之外的事实。
3. 回答尽量简洁、准确。

参考资料：
%s

用户问题：
%s`, contextText, query), nil
}

func runeLen(value string) int {
	return len([]rune(value))
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
