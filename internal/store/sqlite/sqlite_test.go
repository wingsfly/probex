package sqlite

import (
	"context"
	"path/filepath"
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
