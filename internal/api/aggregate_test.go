package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/store/sqlite"
)

func TestGuideXSessionClosurePredicate(t *testing.T) {
	for _, tc := range []struct {
		name, task, extra, err string
		success, exclude       bool
	}{
		{"normal", "ext_guidex-runtime-v4", `{"completion_reason":"session_ended"}`, "", true, true},
		{"custom task", "custom", `{"client_adapter":"guidex-runtime-v4","completion_reason":"session_ended"}`, "", true, true},
		{"old failure", "ext_guidex-runtime-v4", `{"completion_reason":"session_ended"}`, "", false, false},
		{"error", "ext_guidex-runtime-v4", `{"completion_reason":"session_ended"}`, "upstream error", true, false},
		{"completed", "ext_guidex-runtime-v4", `{"completion_reason":"completed"}`, "", true, false},
		{"interrupted", "ext_guidex-runtime-v4", `{"completion_reason":"interrupted"}`, "", true, false},
		{"timeout", "ext_guidex-runtime-v4", `{"completion_reason":"timeout"}`, "", false, false},
		{"legacy", "ext_guidex-interaction", `{"completion_reason":"session_ended"}`, "", true, false},
		{"missing", "ext_guidex-runtime-v4", `{}`, "", true, false},
		{"malformed", "ext_guidex-runtime-v4", `{`, "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &model.ProbeResult{TaskID: tc.task, Success: tc.success, Extra: json.RawMessage(tc.extra), Error: tc.err}
			if got := isGuideXSessionClosure(r); got != tc.exclude {
				t.Fatalf("exclude=%v, want %v", got, tc.exclude)
			}
		})
	}
}

func TestNormalClosureExcludedBeforeAggregationButPreservedInDetailsAndExports(t *testing.T) {
	s, err := sqlite.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := NewResultHandler(s)
	ctx := context.Background()
	task := "ext_guidex-runtime-v4"
	now := time.Now().UTC().Truncate(time.Second)
	for i, id := range []string{"leave-long", "normal-a", "leave-short", "normal-b", "legacy"} {
		r := &model.ProbeResult{ID: id, TaskID: task, AgentID: "browser", Success: true, Timestamp: now.Add(time.Duration(i) * time.Second)}
		switch id {
		case "leave-long", "leave-short":
			r.NodeID = "leavers"
			r.Extra = json.RawMessage(`{"client_adapter":"guidex-runtime-v4","completion_reason":"session_ended","success":true,"upload":0,"observed":17295,"reply":99999}`)
		case "normal-a":
			v := 100.0
			r.LatencyMs = &v
			r.Extra = json.RawMessage(`{"client_adapter":"guidex-runtime-v4","completion_reason":"completed","reply":100}`)
		case "normal-b":
			v := 300.0
			r.LatencyMs = &v
			r.Extra = json.RawMessage(`{"client_adapter":"guidex-runtime-v4","completion_reason":"completed","reply":300}`)
		case "legacy":
			r.TaskID = "ext_guidex-interaction"
			r.Extra = json.RawMessage(`{"completion_reason":"session_ended","reply":555}`)
		}
		if err := s.InsertResult(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	request := func(handler http.HandlerFunc, query string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest("GET", "/results?"+query, nil))
		if w.Code != 200 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body)
		}
		return w
	}
	chart := func(query string) []*model.ProbeResult {
		t.Helper()
		var response struct{ Data []*model.ProbeResult }
		if err := json.Unmarshal(request(h.Aggregate, query).Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response.Data
	}
	for _, points := range []string{"1", "100"} {
		rows := chart("task_id=" + task + "&points=" + points)
		if points == "1" {
			if len(rows) != 1 || rows[0].LatencyMs == nil || *rows[0].LatencyMs != 200 {
				t.Fatalf("unexpected mixed bucket: %+v", rows)
			}
			var extra map[string]any
			if err := json.Unmarshal(rows[0].Extra, &extra); err != nil || extra["reply"] != float64(200) {
				t.Fatalf("closure polluted average: %s (%v)", rows[0].Extra, err)
			}
		} else if len(rows) != 2 {
			t.Fatalf("raw chart rows=%d, want 2", len(rows))
		}
	}
	if rows := chart("task_id=" + task + "&node_id=leavers"); len(rows) != 0 {
		t.Fatalf("closure-only chart not empty: %+v", rows)
	}
	if rows := chart("points=100"); len(rows) != 3 {
		t.Fatalf("All Tasks rows=%d, want 3 (including legacy)", len(rows))
	}
	var detail struct {
		Data []*model.ProbeResult
		Meta Meta
	}
	if err := json.Unmarshal(request(h.Query, "task_id="+task+"&node_id=leavers&slim=1").Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Data) != 2 || detail.Meta.Total != 2 || !detail.Data[0].Success || detail.Data[0].LatencyMs != nil {
		t.Fatalf("details changed: %+v", detail)
	}
	var exported []*model.ProbeResult
	if err := json.Unmarshal(request(h.ExportJSON, "task_id="+task).Body.Bytes(), &exported); err != nil || len(exported) != 4 {
		t.Fatalf("export rows=%d err=%v", len(exported), err)
	}
	csv := request(h.ExportCSV, "task_id="+task).Body.String()
	for _, id := range []string{"leave-long", "leave-short"} {
		if !strings.Contains(csv, id) {
			t.Fatalf("missing closure in CSV: %s", id)
		}
	}
	summary, err := s.GetResultSummary(ctx, model.ResultFilter{TaskID: task})
	if err != nil || summary.Count != 4 || summary.SuccessRate != 100 || summary.AvgLatencyMs != 200 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	raw, total, err := s.QueryResults(ctx, model.ResultFilter{TaskID: task})
	if err != nil || total != 4 || len(raw[0].Extra) == 0 {
		t.Fatalf("raw data changed: total=%d err=%v", total, err)
	}
}

func TestAllTasksAggregationDoesNotStripOriginalDetails(t *testing.T) {
	extra := json.RawMessage(`{"completion_reason":"completed","reply":100}`)
	raw := &model.ProbeResult{TaskID: "ext_guidex-runtime-v4", Extra: extra}
	rows := aggregateByTask([]*model.ProbeResult{raw}, 500)
	if len(rows) != 1 || rows[0].Extra != nil || string(raw.Extra) != string(extra) {
		t.Fatalf("aggregate mutated raw details: rows=%+v raw=%+v", rows, raw)
	}
}
