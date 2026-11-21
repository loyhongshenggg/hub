// Package session gives the core a "desktop session": when it shuts down
// with --session (the desktop launcher's mode), running apps are stopped and
// remembered; the next start brings them back.
package session

import (
	"context"
	"log/slog"
	"time"

	"hub.localhost/platform/core/internal/lifecycle"
	"hub.localhost/platform/core/internal/store"
)

// Stop records the apps that are running (or starting), stops them all and
// waits until they are down or timeout passes. It returns how many apps it
// stopped.
func Stop(ctx context.Context, mg *lifecycle.Manager, st *store.Store, log *slog.Logger, timeout time.Duration) (int, error) {
	running := map[string]string{}
	for _, s := range mg.List() {
		if s.State == lifecycle.Running || s.State == lifecycle.Starting {
			running[s.App.ID] = string(s.Mode)
		}
	}
	if err := st.SaveSession(ctx, running); err != nil {
		return 0, err
	}
	for id := range running {
		log.Info("session: stopping", "app", id)
		if err := mg.Stop(id); err != nil {
			log.Warn("session: stop", "app", id, "error", err)
		}
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		busy := 0
		for id := range running {
			if s, ok := mg.Get(id); ok && s.State != lifecycle.Stopped && s.State != lifecycle.Error {
				busy++
			}
		}
		if busy == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return len(running), nil
}

// Restore starts the apps the last session left running (in the same
// mode) and forgets the record.
func Restore(ctx context.Context, mg *lifecycle.Manager, st *store.Store, log *slog.Logger) error {
	apps, err := st.TakeSession(ctx)
	if err != nil {
		return err
	}
	for id, mode := range apps {
		if _, ok := mg.Get(id); !ok {
			continue // removed since
		}
		m := lifecycle.ModeRun
		if mode == string(lifecycle.ModeDev) {
			m = lifecycle.ModeDev
		}
		log.Info("session: restoring", "app", id, "mode", m)
		if err := mg.StartMode(id, m); err != nil {
			log.Warn("session: restore", "app", id, "error", err)
		}
	}
	return nil
}
