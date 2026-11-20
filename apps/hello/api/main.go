// Command hello is the hub's reference app: a Go API that embeds its React
// UI (../web) and serves both on one port, using libs/go-appkit.
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"

	"hub.localhost/libs/go-appkit"
)

// ui/dist is filled by the Docker build (or `pnpm nx build hello-api`).
//
//go:embed all:ui
var uiFS embed.FS

func main() {
	app := appkit.New()
	visits, err := newCounter(app.Config.DataDir)
	if err != nil {
		app.Log.Error("data dir", "error", err)
		os.Exit(1)
	}
	app.AddCheck("data", visits.check)

	app.Mux.HandleFunc("GET /api/greeting", greeting(app.Config, visits))
	ui, _ := fs.Sub(uiFS, "ui/dist")
	app.Mux.Handle("/", appkit.SPA(ui))

	if err := app.Run(); err != nil {
		app.Log.Error("server", "error", err)
		os.Exit(1)
	}
}

func greeting(cfg appkit.Config, visits *counter) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		n, err := visits.next()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		name := cfg.AppID
		if name == "" {
			name = "hello"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": fmt.Sprintf("Hello from %s!", name),
			"visits":  n,
			"appId":   cfg.AppID,
		})
	}
}
