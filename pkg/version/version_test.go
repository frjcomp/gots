package version

import (
	"testing"
)

// TestVersionVariablesExist ensures version variables are exported and non-empty in builds
func TestVersionVariablesExist(t *testing.T) {
	// Version should be set (at least "dev" in development builds)
	if Version == "" {
		t.Fatal("Version should not be empty")
	}

	// Commit should be set
	if Commit == "" {
		t.Fatal("Commit should not be empty")
	}

	// Date should be set
	if Date == "" {
		t.Fatal("Date should not be empty")
	}
}

// TestVersionDefaults checks the default values set in development
func TestVersionDefaults(t *testing.T) {
	// In dev builds, these should be their defaults
	if Version != "dev" {
		t.Logf("Version: %s (expected 'dev' in dev build, may be set by build flags)", Version)
	}

	if Commit != "none" {
		t.Logf("Commit: %s (expected 'none' in dev build, may be set by build flags)", Commit)
	}

	if Date != "unknown" {
		t.Logf("Date: %s (expected 'unknown' in dev build, may be set by build flags)", Date)
	}
}
