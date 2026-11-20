package process

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"hub.localhost/platform/core/internal/runtime"
)

// logBuffer keeps the last N lines of an instance and feeds live followers.
type logBuffer struct {
	mu        sync.Mutex
	max       int
	lines     []runtime.LogLine
	followers map[chan runtime.LogLine]struct{}
}

func newLogBuffer(max int) *logBuffer {
	return &logBuffer{max: max, followers: map[chan runtime.LogLine]struct{}{}}
}

func (b *logBuffer) add(l runtime.LogLine) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, l)
	if len(b.lines) > b.max {
		b.lines = b.lines[len(b.lines)-b.max:]
	}
	for ch := range b.followers {
		select {
		case ch <- l:
		default: // slow follower: drop the line rather than block the app
		}
	}
}

// snapshot returns the last tail lines (all if tail <= 0) and, with follow,
// a channel of later lines.
func (b *logBuffer) snapshot(tail int, follow bool) ([]runtime.LogLine, <-chan runtime.LogLine, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	start := 0
	if tail > 0 && len(b.lines) > tail {
		start = len(b.lines) - tail
	}
	lines := append([]runtime.LogLine{}, b.lines[start:]...)
	if !follow {
		return lines, nil, func() {}
	}
	ch := make(chan runtime.LogLine, 256)
	b.followers[ch] = struct{}{}
	return lines, ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.followers[ch]; ok {
			delete(b.followers, ch)
			close(ch)
		}
	}
}

func (b *logBuffer) endFollowers() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.followers {
		delete(b.followers, ch)
		close(ch)
	}
}

// Directories never worth watching.
var skipDirs = map[string]bool{"node_modules": true, ".git": true, "dist": true, "tmp": true, "out-tsc": true, ".nx": true, ".vite": true}

// fingerprint hashes path, size and mtime of every file under dir whose base
// name matches one of patterns.
func fingerprint(dir string, patterns []string) string {
	h := sha256.New()
	_ = filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if e.IsDir() {
			if path != dir && (skipDirs[e.Name()] || strings.HasPrefix(e.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		for _, p := range patterns {
			if ok, _ := filepath.Match(p, e.Name()); ok {
				if fi, err := e.Info(); err == nil {
					fmt.Fprintf(h, "%s %d %d\n", path, fi.Size(), fi.ModTime().UnixNano())
				}
				break
			}
		}
		return nil
	})
	return string(h.Sum(nil))
}

// watch restarts p whenever a watched file changes.
func (d *Driver) watch(ctx context.Context, inst *instance, p *proc, logs *logBuffer) {
	last := fingerprint(p.dir, p.spec.Watch)
	t := time.NewTicker(d.opts.WatchInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		fp := fingerprint(p.dir, p.spec.Watch)
		if fp == last {
			continue
		}
		// Let a burst of saves settle (editors write several files).
		time.Sleep(d.opts.WatchInterval / 2)
		last = fingerprint(p.dir, p.spec.Watch)
		if ctx.Err() != nil {
			return
		}
		inst.opMu.Lock()
		if ctx.Err() != nil { // Stop won the race
			inst.opMu.Unlock()
			return
		}
		logs.add(hubLine(p.spec.Name, "file changed; restarting"))
		d.kill(p)
		err := d.spawn(inst, p, logs)
		if err == nil {
			writePids(d.opts.StateDir, inst.in.Project, inst)
		}
		inst.opMu.Unlock()
		if err != nil {
			logs.add(hubLine(p.spec.Name, "restart failed: "+err.Error()))
		}
	}
}

// Pid files let a new core kill dev processes a crashed core left behind.
// Each line: "<pgid> <start time>", the start time guarding against PID reuse.

func pidFile(stateDir, project string) string { return filepath.Join(stateDir, project+".pids") }

func writePids(stateDir, project string, inst *instance) {
	var b strings.Builder
	for _, p := range inst.procs {
		p.mu.Lock()
		if p.cmd != nil && p.running {
			pid := p.cmd.Process.Pid
			fmt.Fprintf(&b, "%d %s\n", pid, startTime(pid))
		}
		p.mu.Unlock()
	}
	_ = os.WriteFile(pidFile(stateDir, project), []byte(b.String()), 0o644)
}

func removePids(stateDir, project string) { _ = os.Remove(pidFile(stateDir, project)) }

// statFields returns the fields of /proc/<pid>/stat after the command name
// (which may contain spaces): [0]=state (field 3), [2]=pgrp (field 5),
// [19]=start time (field 22, clock ticks since boot).
func statFields(pid int) []string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return nil
	}
	s := string(data)
	return strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
}

func startTime(pid int) string {
	if f := statFields(pid); len(f) >= 20 {
		return f[19]
	}
	return ""
}

// groupAlive reports whether any process is still in process group pgid.
func groupAlive(pgid int) bool {
	entries, _ := os.ReadDir("/proc")
	want := strconv.Itoa(pgid)
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if f := statFields(pid); len(f) >= 3 && f[2] == want {
			return true
		}
	}
	return false
}

func killStale(stateDir string, log *slog.Logger) {
	files, _ := filepath.Glob(filepath.Join(stateDir, "*.pids"))
	for _, f := range files {
		data, _ := os.ReadFile(f)
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			pidStr, started, ok := strings.Cut(line, " ")
			pgid, err := strconv.Atoi(pidStr)
			if !ok || err != nil || started == "" {
				continue
			}
			// The leader (the shell) may be gone while its children live on.
			// Linux never hands out a PID still used as a process group ID, so
			// surviving members are ours, unless the leader's PID now belongs
			// to a different process (start time differs).
			if st := startTime(pgid); st != "" && st != started {
				continue
			}
			if !groupAlive(pgid) {
				continue
			}
			log.Info("stopping dev processes left by a previous core", "pgid", pgid, "file", filepath.Base(f))
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
		_ = os.Remove(f)
	}
}
