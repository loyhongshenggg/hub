// Command finance is the API of the Finance hub app: goals, spending
// insights and statement import for Singapore (docs/finance/design.md). It
// serves the React UI from ../web.
package main

import (
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	_ "time/tzdata" // months and goal periods follow Asia/Singapore

	"hub.localhost/apps/finance/api/internal/claude"
	"hub.localhost/apps/finance/api/internal/importer"
	"hub.localhost/apps/finance/api/internal/merchant"
	"hub.localhost/apps/finance/api/internal/ocr"
	"hub.localhost/apps/finance/api/internal/store"
	"hub.localhost/libs/go-appkit"
)

// ui/dist is filled by the Docker build (or `pnpm nx build finance-api`).
//
//go:embed all:ui
var uiFS embed.FS

func main() {
	app := appkit.New()
	if err := run(app); err != nil {
		app.Log.Error("finance", "error", err)
		os.Exit(1)
	}
}

func run(app *appkit.App) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Personal finances are lived in local time; default to Singapore.
	loc, err := time.LoadLocation(appkit.Env("FINANCE_TZ", "Asia/Singapore"))
	if err != nil {
		return err
	}
	now := func() time.Time { return time.Now().In(loc) }

	if err := os.MkdirAll(app.Config.DataDir, 0o700); err != nil {
		return err
	}
	st, err := store.Open(ctx, filepath.Join(app.Config.DataDir, "finance.db"))
	if err != nil {
		return err
	}
	defer st.Close()
	app.AddCheck("db", st.Ping)

	brands, err := merchant.LoadBrands()
	if err != nil {
		return err
	}
	resolver := &merchant.Resolver{Brands: brands}
	resolver.Rule = func(key string) (merchant.Match, bool) {
		r, ok, err := st.GetRule(context.Background(), key)
		if err != nil || !ok {
			return merchant.Match{}, false
		}
		return merchant.Match{Name: r.Merchant, Category: r.Category, Kind: r.Kind}, true
	}
	eng := &ocr.Tesseract{}
	if err := eng.Available(); err != nil {
		app.Log.Warn("statement import disabled until installed", "error", err)
	}
	cl := claude.New(st)
	imp := importer.New(st, eng, resolver, cl, app.Config.DataDir, app.Log)
	if err := imp.LoadDirectory(ctx); err != nil {
		return err
	}
	// First start: fetch the OpenStreetMap directory (small, ~20k places).
	if srcs, err := st.DirectorySources(ctx); err == nil && len(srcs) == 0 && appkit.Env("FINANCE_AUTO_DIRECTORY", "1") == "1" {
		_ = imp.RefreshDirectory("osm")
	}
	go imp.Run(ctx)

	(&api{store: st, imp: imp, claude: cl, ocr: eng, now: now}).routes(app.Mux)
	ui, _ := fs.Sub(uiFS, "ui/dist")
	app.Mux.Handle("/", appkit.SPA(ui))
	return app.Run()
}
