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
	MaxContextChars    int
	RetrievalFailOpen  bool
	MilvusSearchHNSWEF int
}

func LoadConfigFromEnv() Config {
	return Config{
		MilvusURI:          getenv("MILVUS_URI", defaultMilvusURI),
		MilvusCollection:   getenv("MILVUS_COLLECTION", defaultMilvusCollection),
		MilvusVectorField:  getenv("MILVUS_VECTOR_FIELD", defaultMilvusVectorField),
		EmbeddingAPIKey:    os.Getenv("EMBEDDING_API_KEY"),
		EmbeddingBaseURL:   getenv("EMBEDDING_BASE_URL", defaultEmbeddingBaseURL),
		EmbeddingModel:     getenv("EMBEDDING_MODEL", defaultEmbeddingModel),
		EmbeddingDimension: getenvInt("EMBEDDING_DIMENSION", defaultEmbeddingDimension),
		TopK:               getenvInt("RAG_TOP_K", defaultTopK),
		MaxContextChars:    getenvInt("RAG_MAX_CONTEXT_CHARS", defaultMaxContextChars),
		RetrievalFailOpen:  getenvBool("RAG_RETRIEVAL_FAIL_OPEN", defaultRetrievalFailOpen),
		MilvusSearchHNSWEF: getenvInt("MILVUS_SEARCH_HNSW_EF", defaultMilvusSearchHNSWEF),
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
