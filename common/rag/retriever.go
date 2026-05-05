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
	VectorScore      float64
	VectorRank       int
	RerankScore      float64
	RerankRank       int
}

type Retriever struct {
	cfg      Config
	embedder *Embedder
	reranker *Reranker
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
	if cfg.FinalTopK <= 0 {
		cfg.FinalTopK = cfg.TopK
	}
	if cfg.RetrievalTopK <= 0 {
		cfg.RetrievalTopK = cfg.FinalTopK
	}
	if cfg.RerankEnabled && cfg.RetrievalTopK < cfg.FinalTopK {
		cfg.RetrievalTopK = cfg.FinalTopK
	}
	if cfg.MilvusSearchHNSWEF <= 0 {
		cfg.MilvusSearchHNSWEF = defaultMilvusSearchHNSWEF
	}
	var reranker *Reranker
	if cfg.RerankEnabled {
		var err error
		reranker, err = NewReranker(cfg)
		if err != nil {
			return nil, err
		}
	}
	return &Retriever{cfg: cfg, embedder: embedder, reranker: reranker}, nil
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

	searchTopK := r.cfg.FinalTopK
	if r.cfg.RerankEnabled {
		searchTopK = r.cfg.RetrievalTopK
	}
	searchOption := milvusclient.NewSearchOption(
		r.cfg.MilvusCollection,
		searchTopK,
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
			VectorRank:       i + 1,
		}
		if chunk.ChunkID == "" {
			chunk.ChunkID = getString(resultSet.GetColumn("chunk_id"), i)
		}
		if i < len(resultSet.Scores) {
			chunk.Score = float64(resultSet.Scores[i])
			chunk.VectorScore = chunk.Score
		}
		chunks = append(chunks, chunk)
	}

	if r.cfg.RerankEnabled {
		reranked, err := r.reranker.Rerank(ctx, query, chunks)
		if err != nil {
			if !r.cfg.RerankFailOpen {
				return nil, err
			}
			log.Printf("rag_rerank_failed fail_open=true error=%v", err)
		} else {
			chunks = reranked
		}
	}
	if len(chunks) > r.cfg.FinalTopK {
		chunks = chunks[:r.cfg.FinalTopK]
	}

	firstScore := 0.0
	firstChunkID := ""
	firstVectorScore := 0.0
	firstRerankScore := 0.0
	if len(chunks) > 0 {
		firstScore = chunks[0].Score
		firstChunkID = chunks[0].ChunkID
		firstVectorScore = chunks[0].VectorScore
		firstRerankScore = chunks[0].RerankScore
	}
	log.Printf(
		"milvus_rag_retrieved user=%s session_id=%s collection=%s retrieval_top_k=%d final_top_k=%d rerank_enabled=%t chunk_count=%d first_chunk_id=%s first_score=%.4f first_vector_score=%.4f first_rerank_score=%.4f",
		username,
		sessionID,
		r.cfg.MilvusCollection,
		searchTopK,
		r.cfg.FinalTopK,
		r.cfg.RerankEnabled,
		len(chunks),
		firstChunkID,
		firstScore,
		firstVectorScore,
		firstRerankScore,
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
