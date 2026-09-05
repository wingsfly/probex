package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/hjma/probex/internal/model"
)

// maxAggFields are spike/anomaly metrics aggregated with MAX (not AVG) so that
// bursts of loss/jitter/reorder survive time-bucket downsampling. Matches the
// frontend's previous client-side aggregation.
var maxAggFields = map[string]bool{
	"jitter_ms": true, "packet_loss_pct": true,
	"out_of_order_pct": true, "lost_percent": true,
	"effective_loss_pct": true, "retransmits": true,
}

// Aggregate returns time-bucketed, downsampled results for charts, so the client
// never has to pull and process thousands of raw rows. Same ProbeResult shape as
// /results, so the chart code needs no structural change.
func (h *ResultHandler) Aggregate(w http.ResponseWriter, r *http.Request) {
	filter := h.parseFilter(r)
	filter.Limit = 200000 // pull the full range; aggregation happens in-process
	filter.Offset = 0
	results, _, err := h.store.QueryResults(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	points := 500
	if p, e := strconv.Atoi(r.URL.Query().Get("points")); e == nil && p > 0 {
		points = p
	}
	if filter.TaskID != "" {
		writeData(w, aggregateResults(results, points))
		return
	}
	// All Tasks: aggregate each task independently so multi-task latency lines
	// stay separate and the payload stays small over slow links.
	writeData(w, aggregateByTask(results, points))
}

func aggregateByTask(results []*model.ProbeResult, points int) []*model.ProbeResult {
	byTask := map[string][]*model.ProbeResult{}
	order := []string{}
	for _, r := range results {
		if _, ok := byTask[r.TaskID]; !ok {
			order = append(order, r.TaskID)
		}
		byTask[r.TaskID] = append(byTask[r.TaskID], r)
	}
	if len(byTask) == 0 {
		return results
	}
	per := points / len(byTask)
	if per < 50 {
		per = 50
	}
	out := make([]*model.ProbeResult, 0, points)
	for _, tid := range order {
		out = append(out, aggregateResults(byTask[tid], per)...)
	}
	return out
}

// aggregateResults buckets results (input is timestamp-DESC) into at most
// maxPoints representative points, preserving order.
func aggregateResults(results []*model.ProbeResult, maxPoints int) []*model.ProbeResult {
	if maxPoints <= 0 || len(results) <= maxPoints {
		return results
	}
	bucketSize := (len(results) + maxPoints - 1) / maxPoints
	out := make([]*model.ProbeResult, 0, maxPoints)
	for i := 0; i < len(results); i += bucketSize {
		end := i + bucketSize
		if end > len(results) {
			end = len(results)
		}
		out = append(out, aggregateBucket(results[i:end]))
	}
	return out
}

func aggregateBucket(bucket []*model.ProbeResult) *model.ProbeResult {
	mid := bucket[len(bucket)/2]
	agg := &model.ProbeResult{
		ID: mid.ID, TaskID: mid.TaskID, AgentID: mid.AgentID, NodeID: mid.NodeID,
		Timestamp: mid.Timestamp, Success: true,
	}
	agg.LatencyMs = aggFloat(bucket, func(r *model.ProbeResult) *float64 { return r.LatencyMs }, false)
	agg.JitterMs = aggFloat(bucket, func(r *model.ProbeResult) *float64 { return r.JitterMs }, true)
	agg.PacketLossPct = aggFloat(bucket, func(r *model.ProbeResult) *float64 { return r.PacketLossPct }, true)
	agg.DNSResolveMs = aggFloat(bucket, func(r *model.ProbeResult) *float64 { return r.DNSResolveMs }, false)
	agg.TLSHandshakeMs = aggFloat(bucket, func(r *model.ProbeResult) *float64 { return r.TLSHandshakeMs }, false)
	agg.DownloadBps = aggFloat(bucket, func(r *model.ProbeResult) *float64 { return r.DownloadBps }, false)
	agg.UploadBps = aggFloat(bucket, func(r *model.ProbeResult) *float64 { return r.UploadBps }, false)
	agg.Extra = aggregateExtra(bucket)
	return agg
}

func aggFloat(bucket []*model.ProbeResult, get func(*model.ProbeResult) *float64, useMax bool) *float64 {
	var sum, mx float64
	cnt := 0
	for _, r := range bucket {
		v := get(r)
		if v == nil {
			continue
		}
		sum += *v
		if cnt == 0 || *v > mx {
			mx = *v
		}
		cnt++
	}
	if cnt == 0 {
		return nil
	}
	res := sum / float64(cnt)
	if useMax {
		res = mx
	}
	return &res
}

// aggregateExtra averages numeric extra fields across the bucket (MAX for
// anomaly fields), and keeps the first string value for non-numeric fields.
func aggregateExtra(bucket []*model.ProbeResult) json.RawMessage {
	sums := map[string]float64{}
	maxs := map[string]float64{}
	counts := map[string]int{}
	strs := map[string]string{}
	for _, r := range bucket {
		if len(r.Extra) == 0 {
			continue
		}
		var m map[string]any
		if json.Unmarshal(r.Extra, &m) != nil {
			continue
		}
		for k, v := range m {
			switch n := v.(type) {
			case float64:
				sums[k] += n
				if counts[k] == 0 || n > maxs[k] {
					maxs[k] = n
				}
				counts[k]++
			case string:
				if _, ok := strs[k]; !ok {
					strs[k] = n
				}
			}
		}
	}
	if len(counts) == 0 && len(strs) == 0 {
		return nil
	}
	out := map[string]any{}
	for k, c := range counts {
		if maxAggFields[k] {
			out[k] = maxs[k]
		} else {
			out[k] = sums[k] / float64(c)
		}
	}
	for k, s := range strs {
		out[k] = s
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return b
}
