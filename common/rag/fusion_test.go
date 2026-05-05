package rag

import "testing"

func TestRRFFuseCombinesDenseAndBM25Ranks(t *testing.T) {
	fused, err := rrfFuse(
		[]RetrievedChunk{
			{ChunkID: "a", VectorRank: 1, VectorScore: 0.9},
			{ChunkID: "b", VectorRank: 2, VectorScore: 0.8},
		},
		[]RetrievedChunk{
			{ChunkID: "b", BM25Rank: 1, BM25Score: 12},
			{ChunkID: "c", BM25Rank: 2, BM25Score: 8},
		},
		rrfOptions{K: 60, DenseWeight: 1, BM25Weight: 1, Limit: 10},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(fused) != 3 {
		t.Fatalf("fused len = %d, want 3", len(fused))
	}
	if fused[0].ChunkID != "b" {
		t.Fatalf("first chunk = %s, want b", fused[0].ChunkID)
	}
	if fused[0].VectorRank != 2 || fused[0].BM25Rank != 1 {
		t.Fatalf("expected both ranks on b, got %+v", fused[0])
	}
}

func TestRRFFuseAppliesLimit(t *testing.T) {
	fused, err := rrfFuse(
		[]RetrievedChunk{{ChunkID: "a", VectorRank: 1}, {ChunkID: "b", VectorRank: 2}},
		nil,
		rrfOptions{K: 60, DenseWeight: 1, BM25Weight: 1, Limit: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(fused) != 1 || fused[0].ChunkID != "a" {
		t.Fatalf("unexpected limited result: %+v", fused)
	}
}
