package store

import (
	"context"
	"time"
)

// TSPoint represents a single time-series data point.
type TSPoint struct {
	TaskID  string             `json:"task_id"`
	AgentID string             `json:"agent_id"`
	Time    time.Time          `json:"time"`
	Metrics map[string]float64 `json:"metrics"`
}

// AggBucket represents an aggregated time bucket.
type AggBucket struct {
	TaskID      string    `json:"task_id"`
	AgentID     string    `json:"agent_id"`
	BucketStart time.Time `json:"bucket_start"`
	BucketEnd   time.Time `json:"bucket_end"`
	Count       int       `json:"count"`
	SuccessCount int      `json:"success_count"`
	Metrics     map[string]AggMetric `json:"metrics"`
}

// AggMetric holds aggregated values for a single metric.
type AggMetric struct {
	Avg float64 `json:"avg"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
	P95 float64 `json:"p95,omitempty"`
}

// AggFilter specifies query parameters for aggregated data.
type AggFilter struct {
	TaskID     string
	AgentID    string
	MetricName string
	From       time.Time
	To         time.Time
	Resolution string // "1m", "10m", "1h", "8h"
	Limit      int
}

// TimeSeriesStore extends Store with time-series aggregation capabilities.
// SQLite is the default implementation; can be swapped to InfluxDB/TimescaleDB/ClickHouse.
type TimeSeriesStore interface {
	Store

	// Aggregated data operations
	InsertAggBucket(ctx context.Context, table string, bucket *AggBucket) error
	QueryAggBuckets(ctx context.Context, table string, filter AggFilter) ([]*AggBucket, error)
	DeleteAggBefore(ctx context.Context, table string, before time.Time) (int64, error)
}
