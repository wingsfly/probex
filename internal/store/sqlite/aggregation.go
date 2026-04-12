package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hjma/probex/internal/store"
)

// InsertAggBucket inserts an aggregated bucket into the specified table.
func (s *SQLiteStore) InsertAggBucket(ctx context.Context, table string, bucket *store.AggBucket) error {
	metricsJSON, _ := json.Marshal(bucket.Metrics)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO `+table+` (task_id, agent_id, bucket_start, bucket_end, count, success_count, metrics)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		bucket.TaskID, bucket.AgentID,
		bucket.BucketStart.UnixMilli(), bucket.BucketEnd.UnixMilli(),
		bucket.Count, bucket.SuccessCount, string(metricsJSON),
	)
	return err
}

// QueryAggBuckets queries aggregated buckets from the specified table.
func (s *SQLiteStore) QueryAggBuckets(ctx context.Context, table string, filter store.AggFilter) ([]*store.AggBucket, error) {
	query := `SELECT task_id, agent_id, bucket_start, bucket_end, count, success_count, metrics FROM ` + table
	var conds []string
	var args []any
	if filter.TaskID != "" {
		conds = append(conds, "task_id = ?")
		args = append(args, filter.TaskID)
	}
	if filter.AgentID != "" {
		conds = append(conds, "agent_id = ?")
		args = append(args, filter.AgentID)
	}
	if !filter.From.IsZero() {
		conds = append(conds, "bucket_start >= ?")
		args = append(args, filter.From.UnixMilli())
	}
	if !filter.To.IsZero() {
		conds = append(conds, "bucket_end <= ?")
		args = append(args, filter.To.UnixMilli())
	}
	if len(conds) > 0 {
		query += " WHERE "
		for i, c := range conds {
			if i > 0 {
				query += " AND "
			}
			query += c
		}
	}
	query += " ORDER BY bucket_start DESC"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buckets []*store.AggBucket
	for rows.Next() {
		var b store.AggBucket
		var start, end int64
		var metricsStr string
		if err := rows.Scan(&b.TaskID, &b.AgentID, &start, &end, &b.Count, &b.SuccessCount, &metricsStr); err != nil {
			return nil, err
		}
		b.BucketStart = time.UnixMilli(start)
		b.BucketEnd = time.UnixMilli(end)
		json.Unmarshal([]byte(metricsStr), &b.Metrics)
		buckets = append(buckets, &b)
	}
	return buckets, rows.Err()
}

// DeleteAggBefore removes aggregated buckets older than the given time.
func (s *SQLiteStore) DeleteAggBefore(ctx context.Context, table string, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM `+table+` WHERE bucket_end < ?`, before.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
