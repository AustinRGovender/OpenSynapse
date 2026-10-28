package engine

import (
	"math"
	"sort"
)

// histogram is a log-bucketed latency histogram used for whole-run and
// per-endpoint percentiles. Bucket width is ~0.5% of the value, so reported
// percentiles are within ±0.25% of the exact figure while memory stays
// bounded regardless of how many samples a run produces.
type histogram struct {
	buckets map[int32]int64
	count   int64
	sum     float64
	min     float64
	max     float64
}

const histScale = 200.0 // buckets per natural-log unit

func newHistogram() *histogram {
	return &histogram{buckets: make(map[int32]int64), min: math.Inf(1)}
}

func (h *histogram) add(v float64) {
	if v < 0 {
		v = 0
	}
	h.count++
	h.sum += v
	if v < h.min {
		h.min = v
	}
	if v > h.max {
		h.max = v
	}
	h.buckets[bucketOf(v)]++
}

func bucketOf(v float64) int32 {
	// Values below 1µs share the lowest bucket.
	if v < 0.001 {
		v = 0.001
	}
	return int32(math.Round(math.Log(v) * histScale))
}

func bucketValue(b int32) float64 {
	return math.Exp(float64(b) / histScale)
}

// percentile returns the value at quantile q (0..1). The exact max is
// returned for q >= 1 so the tail is never under-reported.
func (h *histogram) percentile(q float64) float64 {
	if h.count == 0 {
		return 0
	}
	if q >= 1 {
		return h.max
	}
	keys := make([]int32, 0, len(h.buckets))
	for k := range h.buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	rank := int64(math.Ceil(q * float64(h.count)))
	if rank < 1 {
		rank = 1
	}
	var seen int64
	for _, k := range keys {
		seen += h.buckets[k]
		if seen >= rank {
			v := bucketValue(k)
			// Clamp to the observed range so a single sample reports exactly.
			return math.Max(h.min, math.Min(v, h.max))
		}
	}
	return h.max
}

func (h *histogram) avg() float64 {
	if h.count == 0 {
		return 0
	}
	return h.sum / float64(h.count)
}

// exactPercentile computes a percentile from a small in-memory sample set
// (used for the per-second buckets, where exact values are cheap).
func exactPercentile(sorted []float64, q float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if q >= 1 {
		return sorted[n-1]
	}
	idx := int(math.Ceil(q*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}
