package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCLI calls run with the given arguments and captures its output.
func runCLI(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRunRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantInErr string
	}{
		{"unknown flag", []string{"-bogus"}, "flag provided but not defined"},
		{"zero parallelism", []string{"-parallel", "0"}, "parallelism must be at least 1"},
		{"too much parallelism", []string{"-parallel", "500"}, "cannot exceed 64"},
		{"negative delay", []string{"-delay", "-1s"}, "delay cannot be negative"},
		{"bad duration", []string{"-timeout", "soon"}, "invalid value"},
		{"empty output", []string{"-out", ""}, "-out cannot be empty"},
		{"stray argument", []string{"robots"}, "unexpected argument"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runCLI(tc.args...)
			if code != exitUsage {
				t.Errorf("exit code = %d, want %d", code, exitUsage)
			}
			if !strings.Contains(stderr, tc.wantInErr) {
				t.Errorf("stderr %q does not mention %q", stderr, tc.wantInErr)
			}
		})
	}
}

func TestRunHelpExitsCleanly(t *testing.T) {
	code, _, stderr := runCLI("-h")
	if code != exitOK || !strings.Contains(stderr, "Usage: wikiscraper") {
		t.Errorf("-h: code=%d stderr=%q", code, stderr)
	}
}

func TestRunRejectsBadURLFile(t *testing.T) {
	dir := t.TempDir()
	badList := filepath.Join(dir, "urls.txt")
	content := "https://en.wikipedia.org/wiki/Robot\nhttp://example.com/page\n"
	if err := os.WriteFile(badList, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runCLI("-urls", badList, "-out", filepath.Join(dir, "out.jl"))
	if code != exitUsage || !strings.Contains(stderr, "example.com") {
		t.Errorf("bad URL file: code=%d stderr=%q", code, stderr)
	}

	code, _, stderr = runCLI("-urls", filepath.Join(dir, "missing.txt"))
	if code != exitUsage || !strings.Contains(stderr, "opening URL file") {
		t.Errorf("missing URL file: code=%d stderr=%q", code, stderr)
	}
}
