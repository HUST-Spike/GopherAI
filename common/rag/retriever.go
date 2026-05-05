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
	BM25Score        float64
	BM25Rank         int
	RRFScore         float64
	FusionRank       int
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
	if cfg.FusionEnabled {
		if cfg.FusionStrategy == "" {
			cfg.FusionStrategy = defaultFusionStrategy
		}
		if cfg.FusionStrategy != defaultFusionStrategy {
			return nil, fmt.Errorf("unsupported fusion strategy: %s", cfg.FusionStrategy)
		}
		if cfg.DenseTopK <= 0 {
			cfg.DenseTopK = cfg.RetrievalTopK
		}
		if cfg.BM25TopK <= 0 {
			cfg.BM25TopK = cfg.RetrievalTopK
		}
		if cfg.FusionTopK <= 0 {
			cfg.FusionTopK = cfg.RetrievalTopK
		}
		if cfg.BM25SparseField == "" {
			cfg.BM25SparseField = defaultBM25SparseField
		}
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

	var chunks []RetrievedChunk
	var denseTopK int
	var bm25TopK int
	if r.cfg.FusionEnabled {
		denseTopK = r.cfg.DenseTopK
		bm25TopK = r.cfg.BM25TopK
		denseHits, err := r.searchDense(ctx, client, username, sessionID, queryVector, denseTopK)
		if err != nil {
			return nil, err
		}
		bm25Hits, err := r.searchBM25(ctx, client, username, sessionID, query, bm25TopK)
		if err != nil {
			return nil, err
		}
		chunks, err = rrfFuse(denseHits, bm25Hits, rrfOptions{
			K:           r.cfg.RRFK,
			DenseWeight: r.cfg.RRFDenseWeight,
			BM25Weight:  r.cfg.RRFBM25Weight,
			Limit:       r.cfg.FusionTopK,
		})
		if err != nil {
			return nil, err
		}
	} else {
		denseTopK = r.cfg.FinalTopK
		if r.cfg.RerankEnabled {
			denseTopK = r.cfg.RetrievalTopK
		}
		chunks, err = r.searchDense(ctx, client, username, sessionID, queryVector, denseTopK)
		if err != nil {
			return nil, err
		}
	}

	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks found for user=%s session=%s", username, sessionID)
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
	firstBM25Score := 0.0
	firstRRFScore := 0.0
	firstRerankScore := 0.0
	if len(chunks) > 0 {
		firstScore = chunks[0].Score
		firstChunkID = chunks[0].ChunkID
		firstVectorScore = chunks[0].VectorScore
		firstBM25Score = chunks[0].BM25Score
		firstRRFScore = chunks[0].RRFScore
		firstRerankScore = chunks[0].RerankScore
	}
	log.Printf(
		"milvus_rag_retrieved user=%s session_id=%s collection=%s fusion_enabled=%t dense_top_k=%d bm25_top_k=%d final_top_k=%d rerank_enabled=%t chunk_count=%d first_chunk_id=%s first_score=%.4f first_vector_score=%.4f first_bm25_score=%.4f first_rrf_score=%.4f first_rerank_score=%.4f",
		username,
		sessionID,
		r.cfg.MilvusCollection,
		r.cfg.FusionEnabled,
		denseTopK,
		bm25TopK,
		r.cfg.FinalTopK,
		r.cfg.RerankEnabled,
		len(chunks),
		firstChunkID,
		firstScore,
		firstVectorScore,
		firstBM25Score,
		firstRRFScore,
		firstRerankScore,
	)
	return chunks, nil
}

func (r *Retriever) searchDense(ctx context.Context, client *milvusclient.Client, username string, sessionID string, queryVector []float32, topK int) ([]RetrievedChunk, error) {
	searchOption := milvusclient.NewSearchOption(
		r.cfg.MilvusCollection,
		topK,
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
	return r.runMilvusSearch(ctx, client, searchOption, "dense")
}

func (r *Retriever) searchBM25(ctx context.Context, client *milvusclient.Client, username string, sessionID string, query string, topK int) ([]RetrievedChunk, error) {
	searchOption := milvusclient.NewSearchOption(
		r.cfg.MilvusCollection,
		topK,
		[]entity.Vector{entity.Text(query)},
	).
		WithANNSField(r.cfg.BM25SparseField).
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
	return r.runMilvusSearch(ctx, client, searchOption, "bm25")
}

func (r *Retriever) runMilvusSearch(ctx context.Context, client *milvusclient.Client, searchOption milvusclient.SearchOption, source string) ([]RetrievedChunk, error) {
	resultSets, err := client.Search(ctx, searchOption)
	if err != nil {
		return nil, fmt.Errorf("milvus %s search failed: %w", source, err)
	}
	if len(resultSets) == 0 {
		return nil, fmt.Errorf("milvus %s search returned no result set", source)
	}
	resultSet := resultSets[0]
	if resultSet.Err != nil {
		return nil, resultSet.Err
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
		switch source {
		case "dense":
			chunk.VectorRank = i + 1
			chunk.VectorScore = chunk.Score
		case "bm25":
			chunk.BM25Rank = i + 1
			chunk.BM25Score = chunk.Score
		}
		chunks = append(chunks, chunk)
	}
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
