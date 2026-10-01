package config

import (
	"fmt"
	"sync"
	"testing"
)

func captureWarnings(t *testing.T) *[]string {
	t.Helper()
	var mu sync.Mutex
	var got []string
	prev := warnf
	warnf = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, fmt.Sprintf(format, args...))
	}
	t.Cleanup(func() { warnf = prev })
	return &got
}

func TestGetenvNewNameWins(t *testing.T) {
	got := captureWarnings(t)
	t.Setenv("DONEWHEN_X_WINS", "new")
	t.Setenv("RAENIL_X_WINS", "old")
	if v := Getenv("DONEWHEN_X_WINS"); v != "new" {
		t.Fatalf("got %q, want new", v)
	}
	if len(*got) != 0 {
		t.Fatalf("no warning expected when the new name is set, got %v", *got)
	}
}

func TestGetenvOldNameFallbackWarnsOnce(t *testing.T) {
	got := captureWarnings(t)
	warned.Delete("RAENIL_X_OLD")
	t.Setenv("RAENIL_X_OLD", "old")
	for i := 0; i < 3; i++ {
		if v := Getenv("DONEWHEN_X_OLD"); v != "old" {
			t.Fatalf("got %q, want old", v)
		}
	}
	if len(*got) != 1 {
		t.Fatalf("want exactly one warning, got %d: %v", len(*got), *got)
	}
}

func TestGetenvUnset(t *testing.T) {
	got := captureWarnings(t)
	if v := Getenv("DONEWHEN_X_UNSET"); v != "" {
		t.Fatalf("got %q, want empty", v)
	}
	if len(*got) != 0 {
		t.Fatalf("unexpected warning %v", *got)
	}
}
