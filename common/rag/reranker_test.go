package rag

import "testing"

func TestApplyRerankScoresSortsByRerankScore(t *testing.T) {
	chunks := []RetrievedChunk{
		{ChunkID: "a", VectorRank: 1, VectorScore: 0.9, Score: 0.9},
		{ChunkID: "b", VectorRank: 2, VectorScore: 0.8, Score: 0.8},
		{ChunkID: "c", VectorRank: 3, VectorScore: 0.7, Score: 0.7},
	}
	ranked := applyRerankScores(chunks, map[int]float64{
		0: 0.2,
		1: 0.95,
		2: 0.5,
	})

	if got := ranked[0].ChunkID; got != "b" {
		t.Fatalf("first chunk = %s, want b", got)
	}
	if ranked[0].RerankRank != 1 || ranked[0].RerankScore != 0.95 || ranked[0].VectorScore != 0.8 {
		t.Fatalf("unexpected first chunk metadata: %+v", ranked[0])
	}
	if got := ranked[0].Score; got != ranked[0].RerankScore {
		t.Fatalf("final score = %v, want rerank score %v", got, ranked[0].RerankScore)
	}
}

func TestApplyRerankScoresUsesVectorRankAsTieBreaker(t *testing.T) {
	chunks := []RetrievedChunk{
		{ChunkID: "a", VectorRank: 1},
		{ChunkID: "b", VectorRank: 2},
	}
	ranked := applyRerankScores(chunks, map[int]float64{
		0: 0.8,
		1: 0.8,
	})

	if ranked[0].ChunkID != "a" || ranked[1].ChunkID != "b" {
		t.Fatalf("tie should preserve vector rank, got %+v", ranked)
	}
}

func TestApplyRerankScoresUsesBM25RankAsTieBreaker(t *testing.T) {
	chunks := []RetrievedChunk{
		{ChunkID: "bm25-only", BM25Rank: 1},
		{ChunkID: "dense", VectorRank: 2},
	}
	ranked := applyRerankScores(chunks, map[int]float64{
		0: 0.9,
		1: 0.9,
	})

	if ranked[0].ChunkID != "bm25-only" {
		t.Fatalf("expected BM25-only hit to use BM25 rank as tie breaker, got %+v", ranked)
	}
}
