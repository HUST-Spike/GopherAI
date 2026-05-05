package rag

import (
	"os"
	"strconv"
)

const (
	defaultMilvusURI          = "http://127.0.0.1:19530"
	defaultMilvusCollection   = "gopherai_document_chunks_v1"
	defaultEmbeddingBaseURL   = "https://open.bigmodel.cn/api/paas/v4"
	defaultEmbeddingModel     = "embedding-3"
	defaultEmbeddingDimension = 1024
	defaultTopK               = 5
	defaultMaxContextChars    = 6000
	defaultRetrievalFailOpen  = false
	defaultMilvusVectorField  = "embedding"
	defaultMilvusSearchHNSWEF = 64
	defaultRerankProvider     = "zhipu"
	defaultRerankModel        = "rerank"
	defaultRerankBaseURL      = "https://open.bigmodel.cn/api/paas/v4"
	defaultRerankTimeoutSec   = 30
	defaultRerankScoreMode    = "rerank_only"
	defaultFusionStrategy     = "rrf"
	defaultBM25SparseField    = "content_sparse"
	defaultRRFK               = 60
	defaultRRFWeight          = 1.0
)

type Config struct {
	MilvusURI          string
	MilvusCollection   string
	MilvusVectorField  string
	EmbeddingAPIKey    string
	EmbeddingBaseURL   string
	EmbeddingModel     string
	EmbeddingDimension int
	TopK               int
	RetrievalTopK      int
	FinalTopK          int
	DenseTopK          int
	BM25TopK           int
	FusionTopK         int
	MaxContextChars    int
	RetrievalFailOpen  bool
	MilvusSearchHNSWEF int
	FusionEnabled      bool
	FusionStrategy     string
	BM25SparseField    string
	RRFK               float64
	RRFDenseWeight     float64
	RRFBM25Weight      float64
	RerankEnabled      bool
	RerankProvider     string
	RerankModel        string
	RerankBaseURL      string
	RerankAPIKey       string
	RerankTimeoutSec   int
	RerankMaxDocChars  int
	RerankScoreMode    string
	RerankFailOpen     bool
}

func LoadConfigFromEnv() Config {
	topK := getenvInt("RAG_TOP_K", defaultTopK)
	return Config{
		MilvusURI:          getenv("MILVUS_URI", defaultMilvusURI),
		MilvusCollection:   getenv("MILVUS_COLLECTION", defaultMilvusCollection),
		MilvusVectorField:  getenv("MILVUS_VECTOR_FIELD", defaultMilvusVectorField),
		EmbeddingAPIKey:    os.Getenv("EMBEDDING_API_KEY"),
		EmbeddingBaseURL:   getenv("EMBEDDING_BASE_URL", defaultEmbeddingBaseURL),
		EmbeddingModel:     getenv("EMBEDDING_MODEL", defaultEmbeddingModel),
		EmbeddingDimension: getenvInt("EMBEDDING_DIMENSION", defaultEmbeddingDimension),
		TopK:               topK,
		RetrievalTopK:      getenvInt("RAG_RETRIEVAL_TOP_K", topK),
		FinalTopK:          getenvInt("RAG_FINAL_TOP_K", topK),
		DenseTopK:          getenvInt("RAG_DENSE_TOP_K", getenvInt("RAG_RETRIEVAL_TOP_K", topK)),
		BM25TopK:           getenvInt("RAG_BM25_TOP_K", getenvInt("RAG_RETRIEVAL_TOP_K", topK)),
		FusionTopK:         getenvInt("RAG_FUSION_TOP_K", getenvInt("RAG_RETRIEVAL_TOP_K", topK)),
		MaxContextChars:    getenvInt("RAG_MAX_CONTEXT_CHARS", defaultMaxContextChars),
		RetrievalFailOpen:  getenvBool("RAG_RETRIEVAL_FAIL_OPEN", defaultRetrievalFailOpen),
		MilvusSearchHNSWEF: getenvInt("MILVUS_SEARCH_HNSW_EF", defaultMilvusSearchHNSWEF),
		FusionEnabled:      getenvBool("RAG_FUSION_ENABLED", false),
		FusionStrategy:     getenv("RAG_FUSION_STRATEGY", defaultFusionStrategy),
		BM25SparseField:    getenv("RAG_BM25_SPARSE_FIELD", defaultBM25SparseField),
		RRFK:               getenvFloat("RAG_RRF_K", defaultRRFK),
		RRFDenseWeight:     getenvFloat("RAG_RRF_DENSE_WEIGHT", defaultRRFWeight),
		RRFBM25Weight:      getenvFloat("RAG_RRF_BM25_WEIGHT", defaultRRFWeight),
		RerankEnabled:      getenvBool("RAG_RERANK_ENABLED", false),
		RerankProvider:     getenv("RAG_RERANK_PROVIDER", defaultRerankProvider),
		RerankModel:        getenv("RAG_RERANK_MODEL", defaultRerankModel),
		RerankBaseURL:      getenv("RAG_RERANK_BASE_URL", defaultRerankBaseURL),
		RerankAPIKey:       os.Getenv("RAG_RERANK_API_KEY"),
		RerankTimeoutSec:   getenvInt("RAG_RERANK_TIMEOUT_SECONDS", defaultRerankTimeoutSec),
		RerankMaxDocChars:  getenvNonNegativeInt("RAG_RERANK_MAX_DOCUMENT_CHARS", 0),
		RerankScoreMode:    getenv("RAG_RERANK_SCORE_MODE", defaultRerankScoreMode),
		RerankFailOpen:     getenvBool("RAG_RERANK_FAIL_OPEN", false),
	}
}

func getenv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvNonNegativeInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func getenvFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
