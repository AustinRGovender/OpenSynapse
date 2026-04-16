package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeStubK6 drops a minimal executable named `k6` (or `k6.cmd` on Windows)
// into dir so that exec.LookPath("k6") can resolve to it when dir is the only
// entry on PATH. Returns the full path written.
func writeStubK6(t *testing.T, dir string) string {
	t.Helper()
	var name string
	var content []byte
	if runtime.GOOS == "windows" {
		// On Windows, exec.LookPath tries extensions from PATHEXT; .cmd is in
		// the default set. An empty .exe would not be a valid PE image.
		name = "k6.cmd"
		content = []byte("@echo off\r\nexit /b 0\r\n")
	} else {
		name = "k6"
		content = []byte("#!/bin/sh\nexit 0\n")
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, content, 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return p
}

// TestNew_ErrorsWhenK6Missing asserts that New() returns an error when k6
// cannot be resolved on PATH. This is the failure mode that returned
// ENGINE_NOT_AVAILABLE from /runs inside the pre-ADR-0007 Docker image: the
// image installed no k6 binary, so LookPath failed and main.go wired the
// engine as nil.
func TestNew_ErrorsWhenK6Missing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	// Neutralise PATHEXT so a stray k6.* in CWD can't be picked up on Windows.
	if runtime.GOOS == "windows" {
		t.Setenv("PATHEXT", ".NOPE")
	}

	eng, err := New()
	if err == nil {
		t.Fatalf("expected error when k6 missing, got engine with path %q", eng.k6Path)
	}
	if eng != nil {
		t.Fatalf("expected nil engine on error, got %+v", eng)
	}
	if !strings.Contains(err.Error(), "k6") {
		t.Fatalf("expected error mentioning k6, got %q", err.Error())
	}
}

// TestNew_DiscoversK6OnPath asserts the happy path: a k6 binary on PATH is
// resolved and stored on the engine. This is the post-ADR-0007 Docker
// invariant (the image MUST ship a resolvable k6 on PATH).
func TestNew_DiscoversK6OnPath(t *testing.T) {
	dir := t.TempDir()
	stub := writeStubK6(t, dir)
	t.Setenv("PATH", dir)
	if runtime.GOOS == "windows" {
		// Ensure .cmd is in the resolvable extensions.
		t.Setenv("PATHEXT", ".CMD;.EXE;.BAT")
	}

	eng, err := New()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eng == nil {
		t.Fatal("expected engine, got nil")
	}
	if eng.k6Path == "" {
		t.Fatal("expected k6Path to be set")
	}
	// Windows LookPath may return a canonicalised case; compare loosely there.
	if runtime.GOOS == "windows" {
		if !strings.EqualFold(eng.k6Path, stub) {
			t.Fatalf("expected k6Path=%q, got %q", stub, eng.k6Path)
		}
	} else {
		if eng.k6Path != stub {
			t.Fatalf("expected k6Path=%q, got %q", stub, eng.k6Path)
		}
	}
	if eng.active == nil {
		t.Fatal("expected active map to be initialised")
	}
}
