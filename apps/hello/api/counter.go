package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// counter persists a visit count in DATA_DIR/visits (a Docker volume).
type counter struct {
	mu   sync.Mutex
	path string
}

func newCounter(dir string) (*counter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &counter{path: filepath.Join(dir, "visits")}, nil
}

func (c *counter) next() (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	data, err := os.ReadFile(c.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return 0, err
	default:
		if n, err = strconv.Atoi(strings.TrimSpace(string(data))); err != nil {
			return 0, fmt.Errorf("corrupt %s: %w", c.path, err)
		}
	}
	n++
	// Write-then-rename so a crash never leaves a half-written file.
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(n)), 0o644); err != nil {
		return 0, err
	}
	return n, os.Rename(tmp, c.path)
}

// check fails health while the data dir isn't writable.
func (c *counter) check(context.Context) error {
	f, err := os.CreateTemp(filepath.Dir(c.path), ".health-*")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}
