package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const maxZhipuRerankDocuments = 128

type Reranker struct {
	apiKey          string
	baseURL         string
	model           string
	provider        string
	maxDocumentChar int
	httpClient      *http.Client
}

func NewReranker(cfg Config) (*Reranker, error) {
	if cfg.RerankProvider == "" {
		cfg.RerankProvider = defaultRerankProvider
	}
	if cfg.RerankProvider != "zhipu" {
		return nil, fmt.Errorf("unsupported rerank provider: %s", cfg.RerankProvider)
	}
	if cfg.RerankAPIKey == "" || strings.HasPrefix(cfg.RerankAPIKey, "replace_with_") {
		return nil, fmt.Errorf("RAG_RERANK_API_KEY is required when rerank is enabled")
	}
	if cfg.RerankBaseURL == "" {
		cfg.RerankBaseURL = defaultRerankBaseURL
	}
	if cfg.RerankModel == "" {
		cfg.RerankModel = defaultRerankModel
	}
	if cfg.RerankTimeoutSec <= 0 {
		cfg.RerankTimeoutSec = defaultRerankTimeoutSec
	}
	if cfg.RerankScoreMode != "" && cfg.RerankScoreMode != defaultRerankScoreMode {
		return nil, fmt.Errorf("unsupported rerank score mode: %s", cfg.RerankScoreMode)
	}

	return &Reranker{
		apiKey:          cfg.RerankAPIKey,
		baseURL:         strings.TrimRight(cfg.RerankBaseURL, "/"),
		model:           cfg.RerankModel,
		provider:        cfg.RerankProvider,
		maxDocumentChar: cfg.RerankMaxDocChars,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.RerankTimeoutSec) * time.Second,
		},
	}, nil
}

func (r *Reranker) Rerank(ctx context.Context, query string, chunks []RetrievedChunk) ([]RetrievedChunk, error) {
	if len(chunks) == 0 {
		return nil, nil
	}
	if len(chunks) > maxZhipuRerankDocuments {
		return nil, fmt.Errorf("zhipu rerank supports at most %d documents, got %d", maxZhipuRerankDocuments, len(chunks))
	}

	documents := make([]string, len(chunks))
	for i, chunk := range chunks {
		document := chunk.Content
		if r.maxDocumentChar > 0 {
			document = truncateRunes(document, r.maxDocumentChar)
		}
		documents[i] = document
	}

	requestBody := zhipuRerankRequest{
		Model:     r.model,
		Query:     query,
		Documents: documents,
		TopN:      len(documents),
	}
	payload, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/rerank", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create rerank request failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	var responseBody zhipuRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		return nil, fmt.Errorf("decode rerank response failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := responseBody.Error.Message
		if message == "" {
			message = responseBody.Message
		}
		return nil, fmt.Errorf("rerank request returned status=%d error=%s", resp.StatusCode, message)
	}
	if len(responseBody.Results) != len(chunks) {
		return nil, fmt.Errorf("rerank response result count mismatch: actual=%d expected=%d", len(responseBody.Results), len(chunks))
	}

	scores := make(map[int]float64, len(responseBody.Results))
	for _, result := range responseBody.Results {
		if result.Index < 0 || result.Index >= len(chunks) {
			return nil, fmt.Errorf("rerank response contains invalid index: %d", result.Index)
		}
		scores[result.Index] = result.RelevanceScore
	}
	if len(scores) != len(chunks) {
		return nil, fmt.Errorf("rerank response did not score every candidate")
	}
	return applyRerankScores(chunks, scores), nil
}

func applyRerankScores(chunks []RetrievedChunk, scores map[int]float64) []RetrievedChunk {
	ranked := make([]RetrievedChunk, len(chunks))
	copy(ranked, chunks)
	for i := range ranked {
		ranked[i].RerankScore = scores[i]
		ranked[i].Score = ranked[i].RerankScore
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].RerankScore == ranked[j].RerankScore {
			return bestRank(ranked[i]) < bestRank(ranked[j])
		}
		return ranked[i].RerankScore > ranked[j].RerankScore
	})
	for i := range ranked {
		ranked[i].RerankRank = i + 1
	}
	return ranked
}

type zhipuRerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

type zhipuRerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
	Message string `json:"message"`
}
