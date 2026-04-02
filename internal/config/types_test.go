package config

import (
	"encoding/json"
	"testing"
	"time"
)

// --- ParseDurationOrDefault ---

func TestParseDurationOrDefault(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		fallback time.Duration
		want     time.Duration
	}{
		{"valid seconds", "15s", 0, 15 * time.Second},
		{"valid minutes", "5m", 0, 5 * time.Minute},
		{"valid hours", "2h", 0, 2 * time.Hour},
		{"valid milliseconds", "500ms", 0, 500 * time.Millisecond},
		{"valid composite", "1m30s", 0, 90 * time.Second},
		{"empty string returns fallback", "", 42 * time.Second, 42 * time.Second},
		{"invalid string returns fallback", "not-a-duration", 7 * time.Second, 7 * time.Second},
		{"negative duration parses", "-5s", 10 * time.Second, -5 * time.Second},
		{"zero duration parses", "0s", 10 * time.Second, 0},
		{"bare number returns fallback", "15", 3 * time.Second, 3 * time.Second},
		{"whitespace returns fallback", "  ", 1 * time.Second, 1 * time.Second},
		{"zero fallback with empty", "", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ParseDurationOrDefault(tt.input, tt.fallback)
			if got != tt.want {
				t.Errorf("ParseDurationOrDefault(%q, %v) = %v, want %v",
					tt.input, tt.fallback, got, tt.want)
			}
		})
	}
}

// --- Default*Config functions ---

func TestDefaultWebTimeoutsConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultWebTimeoutsConfig()

	if cfg == nil {
		t.Fatal("DefaultWebTimeoutsConfig() returned nil")
	}

	tests := []struct {
		name     string
		field    string
		fallback time.Duration
		want     time.Duration
	}{
		{"CmdTimeout", cfg.CmdTimeout, 0, 15 * time.Second},
		{"GhCmdTimeout", cfg.GhCmdTimeout, 0, 10 * time.Second},
		{"TmuxCmdTimeout", cfg.TmuxCmdTimeout, 0, 2 * time.Second},
		{"FetchTimeout", cfg.FetchTimeout, 0, 8 * time.Second},
		{"DefaultRunTimeout", cfg.DefaultRunTimeout, 0, 30 * time.Second},
		{"MaxRunTimeout", cfg.MaxRunTimeout, 0, 60 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDurationOrDefault(tt.field, tt.fallback)
			if got != tt.want {
				t.Errorf("default %s: ParseDurationOrDefault(%q, %v) = %v, want %v",
					tt.name, tt.field, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestDefaultWorkerStatusConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultWorkerStatusConfig()

	if cfg == nil {
		t.Fatal("DefaultWorkerStatusConfig() returned nil")
	}

	stale := ParseDurationOrDefault(cfg.StaleThreshold, 0)
	if stale != 5*time.Minute {
		t.Errorf("StaleThreshold = %v, want 5m", stale)
	}
	stuck := ParseDurationOrDefault(cfg.StuckThreshold, 0)
	if stuck != 30*time.Minute {
		t.Errorf("StuckThreshold = %v, want 30m", stuck)
	}
	// stale < stuck invariant
	if stale >= stuck {
		t.Errorf("StaleThreshold (%v) must be < StuckThreshold (%v)", stale, stuck)
	}
	hbFresh := ParseDurationOrDefault(cfg.HeartbeatFreshThreshold, 0)
	if hbFresh != 5*time.Minute {
		t.Errorf("HeartbeatFreshThreshold = %v, want 5m", hbFresh)
	}
	mayorActive := ParseDurationOrDefault(cfg.MayorActiveThreshold, 0)
	if mayorActive != 5*time.Minute {
		t.Errorf("MayorActiveThreshold = %v, want 5m", mayorActive)
	}
}

func TestDefaultFeedCuratorConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultFeedCuratorConfig()

	if cfg == nil {
		t.Fatal("DefaultFeedCuratorConfig() returned nil")
	}

	dedupe := ParseDurationOrDefault(cfg.DoneDedupeWindow, 0)
	if dedupe != 10*time.Second {
		t.Errorf("DoneDedupeWindow = %v, want 10s", dedupe)
	}
	agg := ParseDurationOrDefault(cfg.SlingAggregateWindow, 0)
	if agg != 30*time.Second {
		t.Errorf("SlingAggregateWindow = %v, want 30s", agg)
	}
	if cfg.MinAggregateCount != 3 {
		t.Errorf("MinAggregateCount = %d, want 3", cfg.MinAggregateCount)
	}
}

// --- JSON serialization round-trips ---

func TestWebTimeoutsConfig_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := &WebTimeoutsConfig{
		CmdTimeout:        "20s",
		GhCmdTimeout:      "15s",
		TmuxCmdTimeout:    "3s",
		FetchTimeout:      "12s",
		DefaultRunTimeout: "45s",
		MaxRunTimeout:      "90s",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var loaded WebTimeoutsConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if loaded != *original {
		t.Errorf("round-trip mismatch:\ngot  %+v\nwant %+v", loaded, *original)
	}
}

func TestWorkerStatusConfig_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := &WorkerStatusConfig{
		StaleThreshold:          "10m",
		StuckThreshold:          "1h",
		HeartbeatFreshThreshold: "3m",
		MayorActiveThreshold:    "8m",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var loaded WorkerStatusConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if loaded != *original {
		t.Errorf("round-trip mismatch:\ngot  %+v\nwant %+v", loaded, *original)
	}
}

func TestFeedCuratorConfig_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := &FeedCuratorConfig{
		DoneDedupeWindow:     "20s",
		SlingAggregateWindow: "1m",
		MinAggregateCount:    5,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var loaded FeedCuratorConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if loaded != *original {
		t.Errorf("round-trip mismatch:\ngot  %+v\nwant %+v", loaded, *original)
	}
}

// --- TownSettings with/without new config fields ---

// --- Gemini provider defaults ---

func TestGeminiProviderDefaults(t *testing.T) {
	t.Parallel()

	t.Run("defaultRuntimeCommand", func(t *testing.T) {
		cmd := defaultRuntimeCommand("gemini")
		if cmd != "gemini" {
			t.Errorf("defaultRuntimeCommand(gemini) = %q, want %q", cmd, "gemini")
		}
	})

	t.Run("defaultSessionIDEnv", func(t *testing.T) {
		env := defaultSessionIDEnv("gemini")
		if env != "GEMINI_SESSION_ID" {
			t.Errorf("defaultSessionIDEnv(gemini) = %q, want %q", env, "GEMINI_SESSION_ID")
		}
	})

	t.Run("defaultHooksProvider", func(t *testing.T) {
		provider := defaultHooksProvider("gemini")
		if provider != "gemini" {
			t.Errorf("defaultHooksProvider(gemini) = %q, want %q", provider, "gemini")
		}
	})

	t.Run("defaultHooksDir", func(t *testing.T) {
		dir := defaultHooksDir("gemini")
		if dir != ".gemini" {
			t.Errorf("defaultHooksDir(gemini) = %q, want %q", dir, ".gemini")
		}
	})

	t.Run("defaultHooksFile", func(t *testing.T) {
		file := defaultHooksFile("gemini")
		if file != "settings.json" {
			t.Errorf("defaultHooksFile(gemini) = %q, want %q", file, "settings.json")
		}
	})

	t.Run("defaultProcessNames", func(t *testing.T) {
		names := defaultProcessNames("gemini", "gemini")
		if len(names) != 1 || names[0] != "gemini" {
			t.Errorf("defaultProcessNames(gemini) = %v, want [gemini]", names)
		}
	})

	t.Run("defaultReadyDelayMs", func(t *testing.T) {
		delay := defaultReadyDelayMs("gemini")
		if delay != 5000 {
			t.Errorf("defaultReadyDelayMs(gemini) = %d, want 5000", delay)
		}
	})

	t.Run("defaultInstructionsFile", func(t *testing.T) {
		file := defaultInstructionsFile("gemini")
		if file != "AGENTS.md" {
			t.Errorf("defaultInstructionsFile(gemini) = %q, want %q", file, "AGENTS.md")
		}
	})
}

func TestParseDurationOrDefault_AllWebTimeoutDefaults(t *testing.T) {
	t.Parallel()
	// Verify that an empty WebTimeoutsConfig (all fields "") falls back to
	// the same values as DefaultWebTimeoutsConfig when parsed.
	empty := &WebTimeoutsConfig{}
	defaults := DefaultWebTimeoutsConfig()

	pairs := []struct {
		name     string
		empty    string
		dflt     string
		fallback time.Duration
	}{
		{"CmdTimeout", empty.CmdTimeout, defaults.CmdTimeout, 15 * time.Second},
		{"GhCmdTimeout", empty.GhCmdTimeout, defaults.GhCmdTimeout, 10 * time.Second},
		{"TmuxCmdTimeout", empty.TmuxCmdTimeout, defaults.TmuxCmdTimeout, 2 * time.Second},
		{"FetchTimeout", empty.FetchTimeout, defaults.FetchTimeout, 8 * time.Second},
		{"DefaultRunTimeout", empty.DefaultRunTimeout, defaults.DefaultRunTimeout, 30 * time.Second},
		{"MaxRunTimeout", empty.MaxRunTimeout, defaults.MaxRunTimeout, 60 * time.Second},
	}

	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			// Empty field should produce fallback
			got := ParseDurationOrDefault(p.empty, p.fallback)
			if got != p.fallback {
				t.Errorf("empty %s: got %v, want %v", p.name, got, p.fallback)
			}
			// Default field should produce same value as fallback
			got = ParseDurationOrDefault(p.dflt, 0)
			if got != p.fallback {
				t.Errorf("default %s: got %v, want %v", p.name, got, p.fallback)
			}
		})
	}
}


