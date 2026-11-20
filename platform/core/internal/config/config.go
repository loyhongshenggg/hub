// Package config holds paths and addresses shared across the core.
package config

import (
	"errors"
	"net"
	"os"
	"path/filepath"
)

// Domain is the parent domain: shell at hub.localhost, apps at <id>.hub.localhost.
const Domain = "hub.localhost"

type Config struct {
	Root    string // repo root (holds apps/)
	DataDir string // SQLite db + generated compose overrides
	Addr    string // listen address, e.g. 127.0.0.1:7000
	// ShellDevURL, if set, proxies the shell to a dev server (Vite) instead
	// of serving the embedded build.
	ShellDevURL string
}

func (c Config) AppsDir() string     { return filepath.Join(c.Root, "apps") }
func (c Config) DBPath() string      { return filepath.Join(c.DataDir, "hub.db") }
func (c Config) OverrideDir() string { return filepath.Join(c.DataDir, "compose") }

func (c Config) port() string {
	_, port, err := net.SplitHostPort(c.Addr)
	if err != nil || port == "80" {
		return ""
	}
	return ":" + port
}

func (c Config) ShellHost() string        { return Domain + c.port() }
func (c Config) ShellOrigin() string      { return "http://" + c.ShellHost() }
func (c Config) AppHost(id string) string { return id + "." + Domain + c.port() }
func (c Config) AppURL(id string) string  { return "http://" + c.AppHost(id) }

// FindRoot walks up from dir to the first folder holding both nx.json and apps/.
func FindRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if isFile(filepath.Join(dir, "nx.json")) && isDir(filepath.Join(dir, "apps")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("hub repo not found (no nx.json + apps/ above the current folder); use --root or HUB_ROOT")
		}
		dir = parent
	}
}

// DefaultDataDir is $XDG_DATA_HOME/hub or ~/.local/share/hub.
func DefaultDataDir() (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "hub"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "hub"), nil
}

func isFile(p string) bool { fi, err := os.Stat(p); return err == nil && !fi.IsDir() }
func isDir(p string) bool  { fi, err := os.Stat(p); return err == nil && fi.IsDir() }
