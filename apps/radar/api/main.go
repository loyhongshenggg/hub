// Command radar is the API of the Radar hub app: it follows news feeds
// (SWE, security, AI) and serves the React UI from ../web.
package main

import (
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"hub.localhost/apps/radar/api/content"
	"hub.localhost/apps/radar/api/internal/fetch"
	"hub.localhost/apps/radar/api/internal/store"
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
	if err := st.SyncSources(ctx, srcs.Sources); err != nil {
		return err
	}
	app.AddCheck("db", st.Ping)

	interval, err := time.ParseDuration(appkit.Env("RADAR_FETCH_INTERVAL", "30m"))
	if err != nil {
		return err
	}
	f := &fetch.Fetcher{Store: st, Log: app.Log, Interval: interval}
	go f.Run(ctx)

	(&api{store: st, fetcher: f, categories: srcs.Categories}).routes(app.Mux)
	ui, _ := fs.Sub(uiFS, "ui/dist")
	app.Mux.Handle("/", appkit.SPA(ui))
	return app.Run()
}
