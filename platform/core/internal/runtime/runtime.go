// Package runtime defines the driver interface the lifecycle manager uses to
// run apps. The docker driver is the only one in Phase 1.
package runtime

import (
	"context"

	"hub.localhost/libs/manifest"
)

// Labels the core puts on the routed service container of every app.
const (
	LabelAppID   = "hub.app.id"
	LabelManaged = "hub.managed"
)

// Instance is one deployment of an app.
type Instance struct {
	App *manifest.Manifest
	// Project is the compose project name: "hub-<id>" for the real instance,
	// something else (e.g. "hub-validate-<id>") for throwaway runs.
	Project string
}

// ProjectName is the compose project of an app's real instance.
func ProjectName(appID string) string { return "hub-" + appID }

// Default returns the real instance of an app.
func Default(m *manifest.Manifest) Instance { return Instance{App: m, Project: ProjectName(m.ID)} }

type StartOptions struct {
	Build bool // rebuild images even if they exist
}

type StopOptions struct {
	RemoveVolumes bool // also delete named volumes (throwaway instances only)
}

// Status of an instance's routed service.
type Status struct {
	Running bool
	Addr    string // ip:port on the hub network, set when Running
	State   string // driver-specific state, e.g. "exited", for messages
}

// Running is a live instance discovered by Running.
type RunningInstance struct {
	AppID   string
	Project string
}

type EventKind string

const (
	EventStarted EventKind = "started"
	EventDied    EventKind = "died"
)

type Event struct {
	Kind     EventKind
	AppID    string
	Project  string
	Service  string
	ExitCode string
}

type LogLine struct {
	Service string `json:"service"`
	Stream  string `json:"stream"` // stdout | stderr
	Text    string `json:"text"`
}

// Driver runs app instances.
type Driver interface {
	// Prepare checks the runtime is reachable and creates shared resources.
	Prepare(ctx context.Context) error
	// Start brings the instance up and returns once its containers are started
	// (not necessarily healthy).
	Start(ctx context.Context, in Instance, opts StartOptions) error
	Stop(ctx context.Context, in Instance, opts StopOptions) error
	Status(ctx context.Context, in Instance) (Status, error)
	// Running lists instances whose routed service is running.
	Running(ctx context.Context) ([]RunningInstance, error)
	// Events streams container start/die events until ctx ends or the
	// connection drops (then the error channel receives).
	Events(ctx context.Context) (<-chan Event, <-chan error)
	// Logs streams log lines of every container of the instance. tail <= 0
	// means all. The channel closes when ctx ends or, without follow, at EOF.
	Logs(ctx context.Context, project string, tail int, follow bool) (<-chan LogLine, error)
}
