package agent

import (
	"testing"
)

func TestAutonomyLevelString(t *testing.T) {
	tests := []struct {
		level AutonomyLevel
		want  string
	}{
		{AutonomySupervised, "supervised"},
		{AutonomyGuided, "guided"},
		{AutonomyCollaborative, "collaborative"},
		{AutonomyAutonomous, "autonomous"},
		{AutonomyFullAutopilot, "full_autopilot"},
	}

	for _, tt := range tests {
		got := tt.level.String()
		if got != tt.want {
			t.Errorf("AutonomyLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestParseAutonomyLevel(t *testing.T) {
	tests := []struct {
		input string
		want  AutonomyLevel
	}{
		{"supervised", AutonomySupervised},
		{"guided", AutonomyGuided},
		{"collaborative", AutonomyCollaborative},
		{"autonomous", AutonomyAutonomous},
		{"full_autopilot", AutonomyFullAutopilot},
		{"autopilot", AutonomyFullAutopilot},
		{"0", AutonomySupervised},
		{"4", AutonomyFullAutopilot},
		{"GUIDED", AutonomyGuided},       // case insensitive
		{"  guided  ", AutonomyGuided},    // whitespace trimmed
		{"unknown", AutonomyCollaborative}, // bad input defaults to collaborative
		{"", AutonomyCollaborative},        // empty defaults to collaborative
	}

	for _, tt := range tests {
		got := ParseAutonomyLevel(tt.input)
		if got != tt.want {
			t.Errorf("ParseAutonomyLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
