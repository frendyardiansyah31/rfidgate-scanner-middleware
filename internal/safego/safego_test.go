package safego

import (
	"strings"
	"testing"

	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

type spyLog struct{ errors []string }

func (s *spyLog) Debug(string, ...logger.Field) {}
func (s *spyLog) Info(string, ...logger.Field)  {}
func (s *spyLog) Warn(string, ...logger.Field)  {}
func (s *spyLog) Error(msg string, f ...logger.Field) {
	s.errors = append(s.errors, msg)
}
func (s *spyLog) Fatal(string, ...logger.Field) {}

func TestRecover_SwallowsPanic_AndReportsAsError(t *testing.T) {
	log := &spyLog{}
	run := func() (err error) {
		defer Recover(log, "unit", &err)
		panic("boom")
	}
	err := run()
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "unit") {
		t.Fatalf("panic must surface as an error naming where/what, got %v", err)
	}
	if len(log.errors) != 1 {
		t.Fatalf("panic must be logged once, got %d", len(log.errors))
	}
}

func TestRecover_NoPanic_LeavesErrorUntouched(t *testing.T) {
	log := &spyLog{}
	run := func() (err error) {
		defer Recover(log, "unit", &err)
		return nil
	}
	if err := run(); err != nil || len(log.errors) != 0 {
		t.Fatalf("no panic must mean no error and no log, got err=%v logs=%d", err, len(log.errors))
	}
}

func TestRecover_NilErrPointerAndNilLogger_AreSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Recover must not re-panic: %v", r)
		}
	}()
	func() {
		defer Recover(nil, "unit", nil)
		panic("boom")
	}()
}
