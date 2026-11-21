// Command radar is the API of the Radar hub app: a learning dashboard built
// on curated tracks and mentor feeds, plus SWE / security / AI news. It
// serves the React UI from ../web.
package main

import (
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	_ "time/tzdata" // the image has no zoneinfo; the weekly goal needs the learner's zone

	"hub.localhost/apps/radar/api/content"
	"hub.localhost/apps/radar/api/internal/engine"
	"hub.localhost/apps/radar/api/internal/fetch"
	"hub.localhost/apps/radar/api/internal/store"
	"hub.localhost/apps/radar/api/internal/tagger"
	"hub.localhost/libs/go-appkit"
)

// ui/dist is filled by the Docker build (or `pnpm nx build radar-api`).
//
//go:embed all:ui
var uiFS embed.FS

func main() {
	app := appkit.New()
	if err := run(app); err != nil {
		app.Log.Error("radar", "error", err)
		os.Exit(1)
	}
}

func run(app *appkit.App) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := os.MkdirAll(app.Config.DataDir, 0o755); err != nil {
		return err
	}
	st, err := store.Open(ctx, filepath.Join(app.Config.DataDir, "radar.db"))
	if err != nil {
		return err
	}
	defer st.Close()
	srcs, err := content.LoadSources()
	if err != nil {
		return err
	}
	cur, err := content.LoadCurriculum()
	if err != nil {
		return err
	}
	feeds, err := content.Feeds(srcs, cur)
	if err != nil {
		return err
	}
	if err := st.SyncSources(ctx, feeds); err != nil {
		return err
	}
	app.AddCheck("db", st.Ping)

	interval, err := time.ParseDuration(appkit.Env("RADAR_FETCH_INTERVAL", "30m"))
	if err != nil {
		return err
	}
	tg := tagger.New(cur)
	tag := func() {
		if n, err := tg.Pass(ctx, st); err != nil {
			app.Log.Error("tagging", "error", err)
		} else if n > 0 {
			app.Log.Info("tagged items", "count", n, "version", tg.Version)
		}
	}
	const window = 120 * 24 * time.Hour
	f := &fetch.Fetcher{Store: st, Log: app.Log, Interval: interval, Retention: window, OnNew: func(int) { tag() }}
	go func() {
		tag() // backfill, or retag after a skills.yaml change
		f.Run(ctx)
	}()

	(&api{store: st, fetcher: f, categories: srcs.Categories, curriculum: cur, window: window}).routes(app.Mux)
	(&learning{store: st, eng: engine.New(cur), now: time.Now}).routes(app.Mux)
	ui, _ := fs.Sub(uiFS, "ui/dist")
	app.Mux.Handle("/", appkit.SPA(ui))
	return app.Run()
}
