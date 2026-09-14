package sqlite

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/hjma/probex/internal/model"
)

func TestInsertResultsIfAbsentRollsBackOnOtherErrors(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec(`CREATE TRIGGER reject_bad BEFORE INSERT ON probe_results WHEN NEW.id = 'bad' BEGIN SELECT RAISE(ABORT, 'test failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	rows := []*model.ProbeResult{{ID: "good", TaskID: "t", AgentID: "a", Timestamp: time.Now()}, {ID: "bad", TaskID: "t", AgentID: "a", Timestamp: time.Now()}}
	inserted, err := s.InsertResultsIfAbsent(context.Background(), rows)
	if err == nil || len(inserted) != 0 {
		t.Fatalf("inserted=%v err=%v", inserted, err)
	}
	_, n, err := s.QueryResults(context.Background(), model.ResultFilter{})
	if err != nil || n != 0 {
		t.Fatalf("partial transaction: rows=%d err=%v", n, err)
	}
}

func TestOrdinaryInsertStillRejectsDuplicateIDs(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := &model.ProbeResult{ID: "same", TaskID: "t", AgentID: "a", Timestamp: time.Now()}
	if err := s.InsertResults(context.Background(), []*model.ProbeResult{r, r}); err == nil {
		t.Fatal("ordinary writes should retain duplicate-ID errors")
	}
	_, n, err := s.QueryResults(context.Background(), model.ResultFilter{})
	if err != nil || n != 0 {
		t.Fatalf("partial transaction: rows=%d err=%v", n, err)
	}
}

func TestResultDimensionsFollowSelectedTimeRange(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now().UTC().Truncate(time.Second)
	rows := []*model.ProbeResult{
		{ID: "old", TaskID: "target", AgentID: "browser-a", NodeID: "old-page", Timestamp: now.Add(-2 * time.Hour)},
		{ID: "recent-a", TaskID: "target", AgentID: "browser-a", NodeID: "recent-page", Timestamp: now.Add(-30 * time.Minute)},
		{ID: "recent-b", TaskID: "target", AgentID: "browser-b", NodeID: "other-page", Timestamp: now.Add(-20 * time.Minute)},
		{ID: "future", TaskID: "target", AgentID: "browser-c", NodeID: "future-page", Timestamp: now.Add(time.Hour)},
		{ID: "other-task", TaskID: "other", AgentID: "browser-z", NodeID: "wrong-task", Timestamp: now.Add(-10 * time.Minute)},
	}
	if err := s.InsertResults(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	filter := model.ResultFilter{TaskID: "target", AgentID: "browser-a", From: now.Add(-time.Hour), To: now}
	agents, nodes, err := s.ResultDimensions(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := agents, []string{"browser-a", "browser-b"}; !slices.Equal(got, want) {
		t.Fatalf("agents=%v, want %v", got, want)
	}
	if got, want := nodes, []string{"recent-page"}; !slices.Equal(got, want) {
		t.Fatalf("nodes=%v, want %v", got, want)
	}
}
