package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Embedder struct {
	apiKey     string
	baseURL    string
	model      string
	dimension  int
	httpClient *http.Client
}

func NewEmbedder(cfg Config) (*Embedder, error) {
	if cfg.EmbeddingAPIKey == "" {
		return nil, fmt.Errorf("EMBEDDING_API_KEY is required")
	}
	if cfg.EmbeddingBaseURL == "" {
		return nil, fmt.Errorf("EMBEDDING_BASE_URL is required")
	}
	if cfg.EmbeddingModel == "" {
		return nil, fmt.Errorf("EMBEDDING_MODEL is required")
	}
	if cfg.EmbeddingDimension <= 0 {
		return nil, fmt.Errorf("EMBEDDING_DIMENSION must be positive")
	}

	return &Embedder{
		apiKey:    cfg.EmbeddingAPIKey,
		baseURL:   strings.TrimRight(cfg.EmbeddingBaseURL, "/"),
		model:     cfg.EmbeddingModel,
		dimension: cfg.EmbeddingDimension,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (e *Embedder) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is empty")
	}

	requestBody := embeddingRequest{
		Model:      e.model,
		Input:      []string{query},
		Dimensions: e.dimension,
	}
	payload, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create embedding request failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	var responseBody embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		return nil, fmt.Errorf("decode embedding response failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding request returned status=%d error=%s", resp.StatusCode, responseBody.Error.Message)
	}
	if len(responseBody.Data) == 0 {
		return nil, fmt.Errorf("embedding response has no data")
	}
	embedding := responseBody.Data[0].Embedding
	if len(embedding) != e.dimension {
		return nil, fmt.Errorf("embedding dimension mismatch: actual=%d expected=%d", len(embedding), e.dimension)
	}
	return embedding, nil
}

type embeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}
