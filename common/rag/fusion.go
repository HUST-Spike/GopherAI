package rag

import (
	"fmt"
	"sort"
)

type rrfOptions struct {
	K           float64
	DenseWeight float64
	BM25Weight  float64
	Limit       int
}

func rrfFuse(denseHits []RetrievedChunk, bm25Hits []RetrievedChunk, options rrfOptions) ([]RetrievedChunk, error) {
	if options.K <= 0 {
		return nil, fmt.Errorf("RAG_RRF_K must be positive")
	}
	if options.DenseWeight < 0 || options.BM25Weight < 0 {
		return nil, fmt.Errorf("RRF weights must be non-negative")
	}

	byChunkID := make(map[string]*RetrievedChunk)
	addHit := func(hit RetrievedChunk) *RetrievedChunk {
		if hit.ChunkID == "" {
			return nil
		}
		if existing, ok := byChunkID[hit.ChunkID]; ok {
			return existing
		}
		row := hit
		byChunkID[hit.ChunkID] = &row
		return &row
	}

	for _, hit := range denseHits {
		row := addHit(hit)
		if row == nil {
			continue
		}
		if row.VectorRank <= 0 || (hit.VectorRank > 0 && hit.VectorRank < row.VectorRank) {
			row.VectorRank = hit.VectorRank
			row.VectorScore = hit.VectorScore
		}
		if hit.VectorRank > 0 {
			row.RRFScore += options.DenseWeight / (options.K + float64(hit.VectorRank))
		}
	}
	for _, hit := range bm25Hits {
		row := addHit(hit)
		if row == nil {
			continue
		}
		if row.BM25Rank <= 0 || (hit.BM25Rank > 0 && hit.BM25Rank < row.BM25Rank) {
			row.BM25Rank = hit.BM25Rank
			row.BM25Score = hit.BM25Score
		}
		if hit.BM25Rank > 0 {
			row.RRFScore += options.BM25Weight / (options.K + float64(hit.BM25Rank))
		}
	}

	fused := make([]RetrievedChunk, 0, len(byChunkID))
	for _, hit := range byChunkID {
		hit.Score = hit.RRFScore
		fused = append(fused, *hit)
	}
	sort.SliceStable(fused, func(i, j int) bool {
		if fused[i].RRFScore == fused[j].RRFScore {
			return bestRank(fused[i]) < bestRank(fused[j])
		}
		return fused[i].RRFScore > fused[j].RRFScore
	})
	for i := range fused {
		fused[i].FusionRank = i + 1
	}
	if options.Limit > 0 && len(fused) > options.Limit {
		fused = fused[:options.Limit]
	}
	return fused, nil
}

func bestRank(hit RetrievedChunk) int {
	best := 1 << 30
	if hit.VectorRank > 0 && hit.VectorRank < best {
		best = hit.VectorRank
	}
	if hit.BM25Rank > 0 && hit.BM25Rank < best {
		best = hit.BM25Rank
	}
	return best
}
