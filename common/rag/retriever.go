package rag

import (
	"context"
	"fmt"
	"log"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

type RetrievedChunk struct {
	ChunkID          string
	DocumentID       string
	OriginalFilename string
	UserName         string
	SessionID        string
	ChunkIndex       int
	Content          string
	Score            float64
}

type Retriever struct {
	cfg      Config
	embedder *Embedder
}

func NewRetriever(cfg Config) (*Retriever, error) {
	embedder, err := NewEmbedder(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.MilvusURI == "" {
		return nil, fmt.Errorf("MILVUS_URI is required")
	}
	if cfg.MilvusCollection == "" {
		return nil, fmt.Errorf("MILVUS_COLLECTION is required")
	}
	if cfg.MilvusVectorField == "" {
		return nil, fmt.Errorf("MILVUS_VECTOR_FIELD is required")
	}
	if cfg.TopK <= 0 {
		cfg.TopK = defaultTopK
	}
	if cfg.MilvusSearchHNSWEF <= 0 {
		cfg.MilvusSearchHNSWEF = defaultMilvusSearchHNSWEF
	}
	return &Retriever{cfg: cfg, embedder: embedder}, nil
}

func (r *Retriever) Retrieve(ctx context.Context, username string, sessionID string, query string) ([]RetrievedChunk, error) {
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID is required")
	}

	queryVector, err := r.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	client, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
		Address: r.cfg.MilvusURI,
	})
	if err != nil {
		return nil, fmt.Errorf("connect milvus failed: %w", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			log.Printf("close milvus client failed: %v", err)
		}
	}()

	searchOption := milvusclient.NewSearchOption(
		r.cfg.MilvusCollection,
		r.cfg.TopK,
		[]entity.Vector{entity.FloatVector(queryVector)},
	).
		WithANNSField(r.cfg.MilvusVectorField).
		WithAnnParam(index.NewHNSWAnnParam(r.cfg.MilvusSearchHNSWEF)).
		WithFilter("user_name == {user_name} && session_id == {session_id}").
		WithTemplateParam("user_name", username).
		WithTemplateParam("session_id", sessionID).
		WithOutputFields(
			"chunk_id",
			"document_id",
			"original_filename",
			"user_name",
			"session_id",
			"chunk_index",
			"content",
		)

	resultSets, err := client.Search(ctx, searchOption)
	if err != nil {
		return nil, fmt.Errorf("milvus search failed: %w", err)
	}
	if len(resultSets) == 0 {
		return nil, fmt.Errorf("milvus search returned no result set")
	}
	resultSet := resultSets[0]
	if resultSet.Err != nil {
		return nil, resultSet.Err
	}
	if resultSet.Len() == 0 {
		return nil, fmt.Errorf("no chunks found for user=%s session=%s", username, sessionID)
	}

	chunks := make([]RetrievedChunk, 0, resultSet.Len())
	for i := 0; i < resultSet.Len(); i++ {
		chunk := RetrievedChunk{
			ChunkID:          getString(resultSet.IDs, i),
			DocumentID:       getString(resultSet.GetColumn("document_id"), i),
			OriginalFilename: getString(resultSet.GetColumn("original_filename"), i),
			UserName:         getString(resultSet.GetColumn("user_name"), i),
			SessionID:        getString(resultSet.GetColumn("session_id"), i),
			ChunkIndex:       getInt(resultSet.GetColumn("chunk_index"), i),
			Content:          getString(resultSet.GetColumn("content"), i),
		}
		if chunk.ChunkID == "" {
			chunk.ChunkID = getString(resultSet.GetColumn("chunk_id"), i)
		}
		if i < len(resultSet.Scores) {
			chunk.Score = float64(resultSet.Scores[i])
		}
		chunks = append(chunks, chunk)
	}
	firstScore := 0.0
	firstChunkID := ""
	if len(chunks) > 0 {
		firstScore = chunks[0].Score
		firstChunkID = chunks[0].ChunkID
	}
	log.Printf(
		"milvus_rag_retrieved user=%s session_id=%s collection=%s top_k=%d chunk_count=%d first_chunk_id=%s first_score=%.4f",
		username,
		sessionID,
		r.cfg.MilvusCollection,
		r.cfg.TopK,
		len(chunks),
		firstChunkID,
		firstScore,
	)
	return chunks, nil
}

func getString(col column.Column, idx int) string {
	if col == nil {
		return ""
	}
	value, err := col.GetAsString(idx)
	if err == nil {
		return value
	}
	raw, err := col.Get(idx)
	if err != nil || raw == nil {
		return ""
	}
	return fmt.Sprint(raw)
}

func getInt(col column.Column, idx int) int {
	if col == nil {
		return 0
	}
	value, err := col.GetAsInt64(idx)
	if err == nil {
		return int(value)
	}
	raw, err := col.Get(idx)
	if err != nil || raw == nil {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	default:
		return 0
	}
}
