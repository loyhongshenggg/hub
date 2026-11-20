package manifest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

const full = `
manifestVersion: 1
id: notes
name: Notes
description: Markdown notes
icon: ./icon.svg
runtime:
  driver: docker
  compose: ./compose.yaml
  service: web
  port: 8080
health:
  path: /healthz
  startTimeout: 90s
ui:
  path: /app
  keepAlive: false
autoStart: true
`

const minimal = `
manifestVersion: 1
id: hello
name: Hello
runtime: {driver: docker, service: web, port: 8080}
health: {path: /healthz}
`

func TestParseFull(t *testing.T) {
	m, err := Parse([]byte(full))
	if err != nil {
		t.Fatal(err)
	}
	want := &Manifest{
		ManifestVersion: 1, ID: "notes", Name: "Notes", Description: "Markdown notes", Icon: "./icon.svg",
		Runtime:   Runtime{Driver: "docker", Compose: "./compose.yaml", Service: "web", Port: 8080},
		Health:    Health{Path: "/healthz", StartTimeout: Duration{90 * time.Second}},
		UI:        UI{Path: "/app", KeepAlive: false},
		AutoStart: true,
	}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("got  %+v\nwant %+v", m, want)
	}
}

func TestParseDefaults(t *testing.T) {
	m, err := Parse([]byte(minimal))
	if err != nil {
		t.Fatal(err)
	}
	if m.Runtime.Compose != "./compose.yaml" || m.Health.StartTimeout.Duration != time.Minute ||
		m.UI.Path != "/" || !m.UI.KeepAlive || m.AutoStart {
		t.Errorf("defaults not applied: %+v", m)
	}
}

func TestParseInvalid(t *testing.T) {
	replace := func(old, new string) string {
		if !strings.Contains(minimal, old) {
			t.Fatalf("test setup: %q not in minimal manifest", old)
		}
		return strings.Replace(minimal, old, new, 1)
	}
	cases := []struct{ name, yaml, want string }{
		{"not yaml", "id: [", "invalid YAML"},
		{"not a mapping", "- a\n- b\n", "must be a YAML mapping"},
		{"no version", replace("manifestVersion: 1", ""), "manifestVersion is required"},
		{"version not int", replace("manifestVersion: 1", "manifestVersion: one"), "must be an integer"},
		{"future version", replace("manifestVersion: 1", "manifestVersion: 99"), "unsupported manifestVersion 99"},
		{"bad id", replace("id: hello", "id: Hello_World"), "id: "},
		{"id trailing dash", replace("id: hello", "id: hello-"), "id: "},
		{"missing name", replace("name: Hello", ""), "missing property 'name'"},
		{"unknown field", minimal + "colour: red\n", "colour"},
		{"bad driver", replace("driver: docker", "driver: podman"), "runtime.driver"},
		{"port too big", replace("port: 8080", "port: 70000"), "runtime.port"},
		{"port string", replace("port: 8080", "port: '8080'"), "runtime.port"},
		{"health path relative", replace("path: /healthz", "path: healthz"), "health.path"},
		{"bad timeout", replace("path: /healthz}", "path: /healthz, startTimeout: 1h}"), "health.startTimeout"},
		{"compose escapes", replace("service: web", "service: web, compose: ../x.yaml"), "inside the app folder"},
		{"compose absolute", replace("service: web", "service: web, compose: /etc/x.yaml"), "inside the app folder"},
		{"icon escapes", minimal + "icon: ../../secret.svg\n", "inside the app folder"},
		{"icon type", minimal + "icon: ./icon.gif\n", "icon: "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.yaml))
			if err == nil {
				t.Fatalf("want error containing %q, got nil", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got:\n%v", c.want, err)
			}
		})
	}
}

func TestParseReportsAllProblems(t *testing.T) {
	_, err := Parse([]byte("manifestVersion: 1\nid: X\nname: ''\n"))
	var merr *Error
	if !errors.As(err, &merr) || len(merr.Problems) < 3 {
		t.Fatalf("want several problems, got %v", err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(FileName, full)

	if _, err := Load(filepath.Join(dir, FileName)); err == nil || !strings.Contains(err.Error(), "runtime.compose") {
		t.Fatalf("want missing compose error, got %v", err)
	}
	write("compose.yaml", "services: {}\n")
	if _, err := Load(filepath.Join(dir, FileName)); err == nil || !strings.Contains(err.Error(), "icon") {
		t.Fatalf("want missing icon error, got %v", err)
	}
	write("icon.svg", "<svg/>")
	m, err := Load(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if m.Dir != dir || m.ComposePath() != filepath.Join(dir, "compose.yaml") || m.IconPath() != filepath.Join(dir, "icon.svg") {
		t.Errorf("paths: dir=%s compose=%s icon=%s", m.Dir, m.ComposePath(), m.IconPath())
	}
}

// TestStructMatchesSchema fails when the latest schema and the Go types drift.
func TestStructMatchesSchema(t *testing.T) {
	raw, err := Schema(LatestVersion)
	if err != nil {
		t.Fatal(err)
	}
	var s schemaNode
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	compare(t, "(root)", s, reflect.TypeFor[Manifest]())
}

type schemaNode struct {
	Properties map[string]schemaNode `json:"properties"`
}

func compare(t *testing.T, at string, s schemaNode, typ reflect.Type) {
	t.Helper()
	fields := map[string]reflect.Type{}
	for f := range typ.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			fields[name] = f.Type
		}
	}
	var schemaKeys, structKeys []string
	for k := range s.Properties {
		schemaKeys = append(schemaKeys, k)
	}
	for k := range fields {
		structKeys = append(structKeys, k)
	}
	slices.Sort(schemaKeys)
	slices.Sort(structKeys)
	if !slices.Equal(schemaKeys, structKeys) {
		t.Errorf("%s: schema properties %v != struct fields %v", at, schemaKeys, structKeys)
		return
	}
	for k, sub := range s.Properties {
		if len(sub.Properties) > 0 {
			compare(t, k, sub, fields[k])
		}
	}
}
