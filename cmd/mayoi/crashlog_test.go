package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

)

func TestKeepCrashes(t *testing.T) {
	defer debug.SetCrashOutput(nil, debug.CrashOptions{})
	defer func(d string) { crashDir = d }(crashDir)
	home, _ := os.UserHomeDir()
	dir := t.TempDir()
	crashes := filepath.Join(dir, "crashes")
	os.MkdirAll(crashes, 0o755)
	for i := range maxCrashes {
		os.WriteFile(filepath.Join(crashes, fmt.Sprintf("2020-01-%02d-000000.log", i+1)), []byte("old"), 0o644)
	}
	path := filepath.Join(dir, "crash.log")
	os.WriteFile(path, []byte(crashHeader(time.Now())+"panic: boom\n\t"+home+"/x.go:1"), 0o644)
	when := time.Date(2026, 10, 14, 9, 30, 5, 0, time.Local)
	os.Chtimes(path, when, when)

	keepCrashes(dir)

	b, err := os.ReadFile(filepath.Join(crashes, "2026-10-14-093005.log"))
	if !strings.Contains(string(b), "panic: boom\n\t~/x.go:1") || strings.Contains(string(b), home) {
		t.Fatalf("moved crash = %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(crashes, "2020-01-01-000000.log")); !os.IsNotExist(err) {
		t.Errorf("oldest crash kept: %v", err)
	}
	if logs := listCrashes(crashes); len(logs) != maxCrashes {
		t.Errorf("%d crashes kept, want %d", len(logs), maxCrashes)
	}
	if b, err := os.ReadFile(path); err != nil || !strings.HasPrefix(string(b), "Mayoi ") || crashed(b) {
		t.Errorf("crash.log after launch: %q, %v", b, err)
	}

	// crash.log with only its header, from a launch that did not crash,
	// stays put.
	keepCrashes(dir)
	if logs := listCrashes(crashes); len(logs) != maxCrashes || logs[len(logs)-1].stem != "2026-10-14-093005" {
		t.Errorf("header-only crash.log moved: %+v", logs)
	}
}

func TestRecoverCrash(t *testing.T) {
	defer func(d string) { crashDir = d }(crashDir)
	crashDir = t.TempDir()
	panicked := func() {
		var wg sync.WaitGroup
		wg.Go(func() {
			defer recoverCrash()
			var m map[string]int
			//lint:ignore SA5000 the panic is what the test recovers
			m["x"] = 1
		})
		wg.Wait()
	}
	panicked()
	panicked() // the same panic again is kept once
	logs := listCrashes(filepath.Join(crashDir, "crashes"))
	if len(logs) != 1 || logs[0].kind != crashRecovered || logs[0].state != crashPending {
		t.Fatalf("logs = %+v", logs)
	}
	b, _ := os.ReadFile(logs[0].path)
	log := string(b)
	_, body, _ := strings.Cut(log, "\n\n")
	lines := strings.Split(body, "\n")
	if lines[0] != "panic: assignment to entry in nil map [recovered]" || !strings.HasPrefix(lines[2], "goroutine ") ||
		!strings.Contains(lines[3], ".TestRecoverCrash.func") {
		t.Errorf("log:\n%s", log)
	}
}

func TestRedactHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	for in, want := range map[string]string{
		home + "/Projects/x.go":       "~/Projects/x.go",
		"/Users/ada/Projects/x.go:1":  "~/Projects/x.go:1",
		"/home/ada/x":                 "~/x",
		`C:\Users\ada\AppData`:        `~\AppData`,
		`"path":"C:\\Users\\ada\\x"`:  `"path":"~\\x"`,
		"/Users/Shared/x":             "/Users/Shared/x",
		"/usr/local/go/src/runtime/x": "/usr/local/go/src/runtime/x",
	} {
		if got := redactHome(in); got != want {
			t.Errorf("redactHome(%q) = %q, want %q", in, got, want)
		}
	}
}
