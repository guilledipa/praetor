package gitops

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestNewSyncer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	repoURL := "https://github.com/guilledipa/praetor-config.git"
	
	// Test default branch fallback
	syncer := NewSyncer(repoURL, "", "/tmp/test", 1*time.Minute, logger)
	if syncer.Branch != "main" {
		t.Errorf("Expected default branch 'main', got '%s'", syncer.Branch)
	}

	// Test custom branch
	syncerCustom := NewSyncer(repoURL, "staging", "/tmp/test2", 5*time.Minute, logger)
	if syncerCustom.Branch != "staging" {
		t.Errorf("Expected branch 'staging', got '%s'", syncerCustom.Branch)
	}

	if syncerCustom.Interval != 5*time.Minute {
		t.Errorf("Expected 5m interval, got %v", syncerCustom.Interval)
	}
}
