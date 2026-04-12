package hub

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/store"
)

// Aggregator periodically rolls up raw probe results into time-bucketed summaries.
// Tiers: raw→1min→10min→1hr→8hr, each with configurable retention.
type Aggregator struct {
	store  store.TimeSeriesStore
	logger *slog.Logger
}

func NewAggregator(s store.TimeSeriesStore, logger *slog.Logger) *Aggregator {
	return &Aggregator{store: s, logger: logger}
}

// Run starts the aggregation loop, executing every 60 seconds.
func (a *Aggregator) Run(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runOnce(ctx)
		}
	}
}

func (a *Aggregator) runOnce(ctx context.Context) {
	now := time.Now()

	// Aggregate raw → 1m (process last 2 minutes to handle late arrivals)
	a.aggregateFromRaw(ctx, "agg_1m", 1*time.Minute, now.Add(-2*time.Minute), now)

	// Aggregate 1m → 10m
	a.aggregateFromAgg(ctx, "agg_1m", "agg_10m", 10*time.Minute, now.Add(-20*time.Minute), now)

	// Aggregate 10m → 1h
	a.aggregateFromAgg(ctx, "agg_10m", "agg_1h", 1*time.Hour, now.Add(-2*time.Hour), now)

	// Aggregate 1h → 8h
	a.aggregateFromAgg(ctx, "agg_1h", "agg_8h", 8*time.Hour, now.Add(-16*time.Hour), now)

	// Cleanup old data
	a.cleanup(ctx, now)
}

func (a *Aggregator) aggregateFromRaw(ctx context.Context, destTable string, bucketSize time.Duration, from, to time.Time) {
	// Query raw results in the time range
	results, _, err := a.store.QueryResults(ctx, model.ResultFilter{
		From:  from,
		To:    to,
		Limit: 100000,
	})
	if err != nil || len(results) == 0 {
		return
	}

	// Group by task_id + agent_id + time bucket
	type key struct {
		taskID, agentID string
		bucketStart     int64
	}
	groups := make(map[key][]*model.ProbeResult)
	for _, r := range results {
		bucketStart := r.Timestamp.Truncate(bucketSize)
		k := key{r.TaskID, r.AgentID, bucketStart.UnixMilli()}
		groups[k] = append(groups[k], r)
	}

	// Compute aggregates per group
	for k, group := range groups {
		bucket := computeBucket(k.taskID, k.agentID, time.UnixMilli(k.bucketStart), bucketSize, group)
		if err := a.store.InsertAggBucket(ctx, destTable, bucket); err != nil {
			a.logger.Error("insert agg bucket", "table", destTable, "error", err)
		}
	}
}

func (a *Aggregator) aggregateFromAgg(ctx context.Context, srcTable, destTable string, bucketSize time.Duration, from, to time.Time) {
	srcBuckets, err := a.store.QueryAggBuckets(ctx, srcTable, store.AggFilter{From: from, To: to, Limit: 100000})
	if err != nil || len(srcBuckets) == 0 {
		return
	}

	type key struct {
		taskID, agentID string
		bucketStart     int64
	}
	groups := make(map[key][]*store.AggBucket)
	for _, b := range srcBuckets {
		bucketStart := b.BucketStart.Truncate(bucketSize)
		k := key{b.TaskID, b.AgentID, bucketStart.UnixMilli()}
		groups[k] = append(groups[k], b)
	}

	for k, group := range groups {
		merged := mergeBuckets(k.taskID, k.agentID, time.UnixMilli(k.bucketStart), bucketSize, group)
		if err := a.store.InsertAggBucket(ctx, destTable, merged); err != nil {
			a.logger.Error("insert agg bucket", "table", destTable, "error", err)
		}
	}
}

func (a *Aggregator) cleanup(ctx context.Context, now time.Time) {
	// Retention: 1m=24h, 10m=7d, 1h=30d, 8h=1y
	tiers := []struct {
		table     string
		retention time.Duration
	}{
		{"agg_1m", 24 * time.Hour},
		{"agg_10m", 7 * 24 * time.Hour},
		{"agg_1h", 30 * 24 * time.Hour},
		{"agg_8h", 365 * 24 * time.Hour},
	}
	for _, t := range tiers {
		deleted, err := a.store.DeleteAggBefore(ctx, t.table, now.Add(-t.retention))
		if err != nil {
			a.logger.Error("agg cleanup", "table", t.table, "error", err)
		} else if deleted > 0 {
			a.logger.Info("agg cleanup", "table", t.table, "deleted", deleted)
		}
	}
}

func computeBucket(taskID, agentID string, bucketStart time.Time, bucketSize time.Duration, results []*model.ProbeResult) *store.AggBucket {
	bucket := &store.AggBucket{
		TaskID:      taskID,
		AgentID:     agentID,
		BucketStart: bucketStart,
		BucketEnd:   bucketStart.Add(bucketSize),
		Count:       len(results),
		Metrics:     make(map[string]store.AggMetric),
	}

	// Collect all numeric metrics
	metricValues := make(map[string][]float64)
	for _, r := range results {
		if r.Success {
			bucket.SuccessCount++
		}
		if r.LatencyMs != nil {
			metricValues["latency_ms"] = append(metricValues["latency_ms"], *r.LatencyMs)
		}
		if r.JitterMs != nil {
			metricValues["jitter_ms"] = append(metricValues["jitter_ms"], *r.JitterMs)
		}
		if r.PacketLossPct != nil {
			metricValues["packet_loss_pct"] = append(metricValues["packet_loss_pct"], *r.PacketLossPct)
		}
		if r.DownloadBps != nil {
			metricValues["download_bps"] = append(metricValues["download_bps"], *r.DownloadBps)
		}
		if r.UploadBps != nil {
			metricValues["upload_bps"] = append(metricValues["upload_bps"], *r.UploadBps)
		}
	}

	for name, vals := range metricValues {
		bucket.Metrics[name] = calcAggMetric(vals)
	}

	return bucket
}

func mergeBuckets(taskID, agentID string, bucketStart time.Time, bucketSize time.Duration, buckets []*store.AggBucket) *store.AggBucket {
	merged := &store.AggBucket{
		TaskID:      taskID,
		AgentID:     agentID,
		BucketStart: bucketStart,
		BucketEnd:   bucketStart.Add(bucketSize),
		Metrics:     make(map[string]store.AggMetric),
	}

	allMetrics := make(map[string][]float64)
	for _, b := range buckets {
		merged.Count += b.Count
		merged.SuccessCount += b.SuccessCount
		for name, m := range b.Metrics {
			// Approximate: use avg * count as a weighted value
			for i := 0; i < b.Count; i++ {
				allMetrics[name] = append(allMetrics[name], m.Avg)
			}
			// Also track min/max across sub-buckets
			if existing, ok := merged.Metrics[name]; ok {
				existing.Min = math.Min(existing.Min, m.Min)
				existing.Max = math.Max(existing.Max, m.Max)
				merged.Metrics[name] = existing
			}
		}
	}

	for name, vals := range allMetrics {
		agg := calcAggMetric(vals)
		if existing, ok := merged.Metrics[name]; ok {
			agg.Min = math.Min(agg.Min, existing.Min)
			agg.Max = math.Max(agg.Max, existing.Max)
		}
		merged.Metrics[name] = agg
	}

	return merged
}

func calcAggMetric(vals []float64) store.AggMetric {
	if len(vals) == 0 {
		return store.AggMetric{}
	}
	sort.Float64s(vals)
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	p95Idx := int(float64(len(vals)) * 0.95)
	if p95Idx >= len(vals) {
		p95Idx = len(vals) - 1
	}
	return store.AggMetric{
		Avg: sum / float64(len(vals)),
		Min: vals[0],
		Max: vals[len(vals)-1],
		P95: vals[p95Idx],
	}
}
