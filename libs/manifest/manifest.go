// Package manifest parses and validates hub app manifests (app.manifest.yaml).
//
// The JSON Schemas in schema/ are the source of truth. Every supported
// manifestVersion keeps its own schema; Parse validates against the schema the
// file declares, then upgrades it into Manifest, the current in-memory shape.
package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v4"
)

// FileName is the manifest file every app ships at its folder root.
const FileName = "app.manifest.yaml"

// LatestVersion is the newest manifestVersion this package understands.
const LatestVersion = 1

// Manifest is an app manifest after validation, defaults and upgrades.
type Manifest struct {
	// ManifestVersion is the version the file declared.
	ManifestVersion int     `json:"manifestVersion"`
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description,omitempty"`
	Icon            string  `json:"icon,omitempty"`
	Runtime         Runtime `json:"runtime"`
	Health          Health  `json:"health"`
	UI              UI      `json:"ui"`
	AutoStart       bool    `json:"autoStart"`

	// Dir is the absolute folder holding the manifest. Set by Load.
	Dir string `json:"-"`
}

// Runtime says how the core runs the app.
type Runtime struct {
	Driver  string `json:"driver"`
	Compose string `json:"compose"`
	Service string `json:"service"`
	Port    int    `json:"port"`
}

// Health is the readiness check.
type Health struct {
	Path         string   `json:"path"`
	StartTimeout Duration `json:"startTimeout"`
}

// UI says how the shell shows the app.
type UI struct {
	Path      string `json:"path"`
	KeepAlive bool   `json:"keepAlive"`
}

// Duration is a time.Duration written as a string such as "60s".
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// Load reads and parses the manifest at path, and checks that the files it
// references exist.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if m.Dir, err = filepath.Abs(filepath.Dir(path)); err != nil {
		return nil, err
	}
	if _, err := os.Stat(m.ComposePath()); err != nil {
		return nil, fmt.Errorf("%s: runtime.compose: %w", path, err)
	}
	if m.Icon != "" {
		if _, err := os.Stat(m.IconPath()); err != nil {
			return nil, fmt.Errorf("%s: icon: %w", path, err)
		}
	}
	return m, nil
}

// Parse decodes and validates manifest YAML, applies defaults and upgrades
// older versions to Manifest. It does not touch the filesystem.
func Parse(data []byte) (*Manifest, error) {
	var doc any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if _, ok := doc.(map[string]any); !ok {
		return nil, errors.New("manifest must be a YAML mapping")
	}
	// Round-trip through JSON so validator and decoder both see plain JSON values.
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("manifest is not JSON-compatible: %w", err)
	}

	var head struct {
		ManifestVersion *json.RawMessage `json:"manifestVersion"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, err
	}
	if head.ManifestVersion == nil {
		return nil, fmt.Errorf("manifestVersion is required (latest is %d)", LatestVersion)
	}
	var version int
	if err := json.Unmarshal(*head.ManifestVersion, &version); err != nil {
		return nil, fmt.Errorf("manifestVersion must be an integer, got %s", *head.ManifestVersion)
	}

	if err := validate(version, raw); err != nil {
		return nil, err
	}

	var m *Manifest
	switch version {
	case 1:
		m, err = decodeV1(raw)
	default:
		// validate already rejected unknown versions.
		panic(fmt.Sprintf("manifest: no decoder for version %d", version))
	}
	if err != nil {
		return nil, err
	}
	return m, m.checkPaths()
}

func decodeV1(raw []byte) (*Manifest, error) {
	m := &Manifest{
		Runtime: Runtime{Compose: "./compose.yaml"},
		Health:  Health{StartTimeout: Duration{60 * time.Second}},
		UI:      UI{Path: "/", KeepAlive: true},
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manifest) checkPaths() error {
	if !filepath.IsLocal(m.Runtime.Compose) {
		return fmt.Errorf("runtime.compose: %q must be a relative path inside the app folder", m.Runtime.Compose)
	}
	if m.Icon != "" && !filepath.IsLocal(m.Icon) {
		return fmt.Errorf("icon: %q must be a relative path inside the app folder", m.Icon)
	}
	return nil
}

// ComposePath is the absolute path of the compose file. Requires Dir.
func (m *Manifest) ComposePath() string { return filepath.Join(m.Dir, m.Runtime.Compose) }

// IconPath is the absolute path of the icon, or "" if none. Requires Dir.
func (m *Manifest) IconPath() string {
	if m.Icon == "" {
		return ""
	}
	return filepath.Join(m.Dir, m.Icon)
}
