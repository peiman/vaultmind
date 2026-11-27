package memory

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseShardSelection parses the command's one-based i/n shard notation.
func ParseShardSelection(value string) (int, int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, 0, nil
	}
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("shard must be in i/n form")
	}
	index, indexErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	count, countErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if indexErr != nil || countErr != nil {
		return 0, 0, fmt.Errorf("shard must be in i/n form")
	}
	if err := ValidateShard(index, count); err != nil {
		return 0, 0, err
	}
	return index, count, nil
}

// ValidateShard validates the one-based i/n notation used by arc recite.
func ValidateShard(index, count int) error {
	if count <= 0 {
		return fmt.Errorf("shard count must be positive")
	}
	if index < 1 || index > count {
		return fmt.Errorf("shard index must be between 1 and %d", count)
	}
	return nil
}

// PlanArcShards partitions sorted arc indexes into contiguous, deterministic
// shards. Boundaries are chosen nearest each shard's byte target while keeping
// every shard non-empty when there are at least as many arcs as shards. When
// there are fewer arcs than shards, empty shards are part of the plan rather
// than an error: a caller can still render every requested part deterministically.
func PlanArcShards(weights []int, count int) ([][]int, error) {
	if count <= 0 {
		return nil, fmt.Errorf("shard count must be positive")
	}
	shards := make([][]int, count)
	if len(weights) == 0 {
		return shards, nil
	}
	total := 0
	for _, weight := range weights {
		if weight < 0 {
			return nil, fmt.Errorf("arc weight must not be negative")
		}
		total += weight
	}
	boundary := 0
	for shard := 1; shard < count; shard++ {
		remaining := count - shard
		minCut, maxCut := boundary, len(weights)-remaining
		if len(weights) >= count && minCut < boundary+1 {
			minCut = boundary + 1
		}
		target := float64(total) * float64(shard) / float64(count)
		best := minCut
		bestDistance := math.Inf(1)
		for cut := minCut; cut <= maxCut; cut++ {
			distance := math.Abs(float64(prefixWeight(weights, cut)) - target)
			if distance < bestDistance {
				best, bestDistance = cut, distance
			}
		}
		shards[shard-1] = indexes(boundary, best)
		boundary = best
	}
	shards[count-1] = indexes(boundary, len(weights))
	return shards, nil
}

func prefixWeight(weights []int, end int) int {
	total := 0
	for _, weight := range weights[:end] {
		total += weight
	}
	return total
}

func indexes(start, end int) []int {
	result := make([]int, end-start)
	for i := range result {
		result[i] = start + i
	}
	return result
}
