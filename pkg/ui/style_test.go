package ui

import (
	"strings"
	"testing"
	"time"
)

// ── Constants Tests ──────────────────────────────────────────────────

func TestANSIConstants(t *testing.T) {
	codes := []struct {
		name  string
		value string
	}{
		{"Reset", Reset},
		{"Bold", Bold},
		{"Red", Red},
		{"Green", Green},
		{"Blue", Blue},
		{"Cyan", Cyan},
		{"BrightCyan", BrightCyan},
		{"ClearLine", ClearLine},
	}
	for _, c := range codes {
		if c.value == "" {
			t.Errorf("%s should not be empty", c.name)
		}
		if !strings.HasPrefix(c.value, "\033[") {
			t.Errorf("%s should start with ESC[, got %q", c.name, c.value)
		}
	}
}

func TestIconConstants(t *testing.T) {
	icons := []struct {
		name  string
		value string
	}{
		{"IconPrompt", IconPrompt},
		{"IconSuccess", IconSuccess},
		{"IconError", IconError},
		{"IconWarning", IconWarning},
		{"IconDelegate", IconDelegate},
		{"IconAgent", IconAgent},
	}
	for _, ic := range icons {
		if ic.value == "" {
			t.Errorf("%s should not be empty", ic.name)
		}
	}
}

func TestThemeColors(t *testing.T) {
	if ColorPrompt == "" {
		t.Error("ColorPrompt should not be empty")
	}
	if ColorError == "" {
		t.Error("ColorError should not be empty")
	}
	if ColorSuccess == "" {
		t.Error("ColorSuccess should not be empty")
	}
}

// ── truncateStr Tests ────────────────────────────────────────────────

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"hello world this is long", 10, "hello w..."},
		{"exact", 5, "exact"},
		{"", 5, ""},
	}
	for _, tc := range tests {
		got := truncateStr(tc.input, tc.max)
		if got != tc.want {
			t.Errorf("truncateStr(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
		}
	}
}

func TestTruncateStrLength(t *testing.T) {
	long := strings.Repeat("x", 100)
	result := truncateStr(long, 20)
	if len(result) > 20 {
		t.Errorf("result length %d exceeds max 20", len(result))
	}
}

// ── Spinner Tests ────────────────────────────────────────────────────

func TestSpinnerCreateAndStop(t *testing.T) {
	s := NewSpinner()
	if s == nil {
		t.Fatal("NewSpinner should not return nil")
	}
	if len(s.frames) == 0 {
		t.Error("spinner should have animation frames")
	}
}

func TestSpinnerStartStop(t *testing.T) {
	s := NewSpinner()
	s.Start("testing...")
	time.Sleep(200 * time.Millisecond)
	s.Stop() // should not panic

	// Double stop should not panic.
	s.Stop()
}

func TestSpinnerUpdate(t *testing.T) {
	s := NewSpinner()
	s.Start("initial message")
	time.Sleep(100 * time.Millisecond)
	s.Update("updated message")
	time.Sleep(100 * time.Millisecond)
	s.Stop()
}

func TestSpinnerStopWith(t *testing.T) {
	s := NewSpinner()
	s.Start("loading")
	time.Sleep(100 * time.Millisecond)
	s.StopWith(IconSuccess, ColorSuccess, "done!")
}

func TestSpinnerStartWhileActive(t *testing.T) {
	s := NewSpinner()
	s.Start("first")
	time.Sleep(100 * time.Millisecond)
	// Starting again while active should update message, not panic.
	s.Start("second")
	time.Sleep(100 * time.Millisecond)
	s.Stop()
}
