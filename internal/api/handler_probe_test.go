package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/probe"
	"github.com/hjma/probex/internal/store/sqlite"
)

type countingAlerts struct{ count atomic.Int64 }

func (a *countingAlerts) Evaluate(*model.ProbeResult) { a.count.Add(1) }

func testProbeRouter(s *sqlite.SQLiteStore, alerts AlertEvaluator) http.Handler {
	reg := probe.NewRegistry()
	reg.RegisterExternal(probe.ProbeMetadata{Name: "test", Kind: probe.ProbeKindExternal})
	reg.RegisterExternal(probe.ProbeMetadata{Name: "other", Kind: probe.ProbeKindExternal})
	r := chi.NewRouter()
	r.Post("/probes/{name}/push", NewProbeHandler(reg, s, alerts, "").PushResults)
	return r
}

func pushTest(r http.Handler, path string, payload any) *httptest.ResponseRecorder {
	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", path, bytes.NewReader(body)))
	return w
}

func resultRequest(ids ...string) map[string]any {
	var rows []map[string]any
	for _, id := range ids {
		rows = append(rows, map[string]any{"result_id": id, "timestamp": "2026-09-10T04:27:22.989Z", "success": true, "extra": map[string]any{"value": 1}})
	}
	return map[string]any{"agent_id": "browser", "node_id": "page", "results": rows}
}

func assertPushCounts(t *testing.T, w *httptest.ResponseRecorder, inserted, duplicates int) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	var response struct {
		Data map[string]int `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data["inserted"] != inserted || response.Data["duplicates"] != duplicates || response.Data["accepted"] != inserted+duplicates {
		t.Fatalf("counts=%v, want inserted=%d duplicates=%d", response.Data, inserted, duplicates)
	}
}

func TestExternalPushIdempotency(t *testing.T) {
	s, err := sqlite.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	alerts := &countingAlerts{}
	r := testProbeRouter(s, alerts)
	assertPushCounts(t, pushTest(r, "/probes/test/push", resultRequest("a", "a", "b")), 2, 1)
	assertPushCounts(t, pushTest(r, "/probes/test/push", resultRequest("a", "b", "c")), 1, 2)
	// Equal measurements in distinct samples must not be merged.
	assertPushCounts(t, pushTest(r, "/probes/test/push", resultRequest("d")), 1, 0)
	// Older clients remain non-idempotent unless they supply an ID.
	assertPushCounts(t, pushTest(r, "/probes/test/push", resultRequest("", "")), 2, 0)
	_, n, err := s.QueryResults(context.Background(), model.ResultFilter{})
	if err != nil || n != 6 || alerts.count.Load() != 6 {
		t.Fatalf("rows=%d alerts=%d err=%v", n, alerts.count.Load(), err)
	}
}

func TestExternalPushConcurrentReplay(t *testing.T) {
	s, err := sqlite.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	alerts := &countingAlerts{}
	r := testProbeRouter(s, alerts)
	var wg sync.WaitGroup
	var inserted atomic.Int64
	for range 16 {
		wg.Go(func() {
			w := pushTest(r, "/probes/test/push", resultRequest("same"))
			if w.Code != http.StatusOK {
				t.Errorf("status=%d body=%s", w.Code, w.Body)
				return
			}
			var response struct {
				Data map[string]int `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Error(err)
				return
			}
			inserted.Add(int64(response.Data["inserted"]))
		})
	}
	wg.Wait()
	if inserted.Load() != 1 || alerts.count.Load() != 1 {
		t.Fatalf("inserted=%d alerts=%d", inserted.Load(), alerts.count.Load())
	}
}

func TestExternalPushScopeAndFirstWriteWins(t *testing.T) {
	s, err := sqlite.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := testProbeRouter(s, nil)
	assertPushCounts(t, pushTest(r, "/probes/test/push", resultRequest("same")), 1, 0)
	for _, field := range []string{"agent_id", "node_id", "task_id"} {
		req := resultRequest("same")
		req[field] = "different"
		assertPushCounts(t, pushTest(r, "/probes/test/push", req), 1, 0)
	}
	// Even with the same explicit task, probe names have separate namespaces.
	for _, name := range []string{"test", "other"} {
		req := resultRequest("same")
		req["task_id"] = "shared"
		assertPushCounts(t, pushTest(r, "/probes/"+name+"/push", req), 1, 0)
	}
	req := resultRequest("same")
	req["results"].([]map[string]any)[0]["success"] = false
	assertPushCounts(t, pushTest(r, "/probes/test/push", req), 0, 1)
	rows, _, err := s.QueryResults(context.Background(), model.ResultFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if !row.Success {
			t.Fatal("replay overwrote an accepted result")
		}
	}
}

func TestExternalPushIdempotencySurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := sqlite.New(path)
	if err != nil {
		t.Fatal(err)
	}
	assertPushCounts(t, pushTest(testProbeRouter(s, nil), "/probes/test/push", resultRequest("same")), 1, 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = sqlite.New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	assertPushCounts(t, pushTest(testProbeRouter(s, nil), "/probes/test/push", resultRequest("same")), 0, 1)
}

func TestExternalPushValidatesEntireBatchBeforeWriting(t *testing.T) {
	s, err := sqlite.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := testProbeRouter(s, nil)
	for _, id := range []string{"  ", strings.Repeat("x", 129)} {
		w := pushTest(r, "/probes/test/push", resultRequest("valid", id))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status=%d", w.Code)
		}
	}
	_, n, err := s.QueryResults(context.Background(), model.ResultFilter{})
	if err != nil || n != 0 {
		t.Fatalf("partial write: rows=%d err=%v", n, err)
	}
}
