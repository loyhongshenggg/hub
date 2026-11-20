// Package registry finds apps: every apps/<id>/app.manifest.yaml.
package registry

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"hub.localhost/libs/manifest"
)

// Problem is an app folder whose manifest could not be loaded.
type Problem struct {
	ID    string `json:"id"` // folder name
	Path  string `json:"path"`
	Error string `json:"error"`
}

// Result of one scan.
type Result struct {
	Apps     []*manifest.Manifest // sorted by id
	Problems []Problem
}

// Scan loads every manifest under appsDir. Folders without a manifest are
// ignored; invalid manifests become Problems.
func Scan(appsDir string) (Result, error) {
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		return Result{}, err
	}
	var res Result
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(appsDir, e.Name(), manifest.FileName)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		m, err := manifest.Load(path)
		if err == nil && m.ID != e.Name() {
			err = fmt.Errorf("id %q must match its folder name %q", m.ID, e.Name())
		}
		if err != nil {
			res.Problems = append(res.Problems, Problem{ID: e.Name(), Path: path, Error: err.Error()})
			continue
		}
		res.Apps = append(res.Apps, m)
	}
	slices.SortFunc(res.Apps, func(a, b *manifest.Manifest) int { return strings.Compare(a.ID, b.ID) })
	return res, nil
}

// fingerprint changes whenever a manifest or compose file is added, removed
// or edited.
func fingerprint(appsDir string) string {
	h := sha256.New()
	dirs, _ := os.ReadDir(appsDir)
	for _, d := range dirs {
		for _, name := range []string{manifest.FileName, "compose.yaml", "compose.yml"} {
			if fi, err := os.Stat(filepath.Join(appsDir, d.Name(), name)); err == nil {
				fmt.Fprintf(h, "%s/%s %d %d\n", d.Name(), name, fi.Size(), fi.ModTime().UnixNano())
			}
		}
	}
	return string(h.Sum(nil))
}

// Watch scans now and again whenever the apps folder changes (polled), calling
// onChange with each new result. It blocks until ctx ends.
func Watch(ctx context.Context, appsDir string, every time.Duration, onChange func(Result, error)) {
	last := ""
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if fp := fingerprint(appsDir); fp != last {
			last = fp
			onChange(Scan(appsDir))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
