package main

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sync"
	"time"

	"hub.localhost/apps/radar/api/content"
	"hub.localhost/apps/radar/api/internal/store"
)

// avatars serves mentor and source pictures from a local cache, so the
// browser never calls third parties for them and they work offline.
type avatars struct {
	store  *store.Store
	cur    *content.Curriculum
	client *http.Client // reader.SafeClient outside tests
	mu     sync.Mutex
}

const (
	avatarTTL = 30 * 24 * time.Hour
	avatarMax = 512 << 10
)

var avatarTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

func (a *avatars) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+content.FacePath+"{id}", a.serve)
}

func (a *avatars) serve(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	src := a.cur.AvatarURL(id)
	if src == "" {
		writeError(w, 404, "no picture for "+id)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cached, err := a.store.Avatar(r.Context(), id)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if cached == nil || cached.URL != src || time.Since(cached.FetchedAt) > avatarTTL {
		fresh, ferr := a.fetch(r.Context(), src)
		switch {
		case ferr == nil:
			if err := a.store.PutAvatar(r.Context(), id, *fresh); err != nil {
				writeError(w, 500, err.Error())
				return
			}
			cached = fresh
		case cached == nil:
			writeError(w, 502, ferr.Error())
			return
		} // else: keep serving the stale copy
	}
	w.Header().Set("Content-Type", cached.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(cached.Data)
}

func (a *avatars) fetch(ctx context.Context, src string) (*store.Avatar, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; RadarReader/1.0; personal reading app)")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("avatar: HTTP %d", resp.StatusCode)
	}
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if !avatarTypes[ct] {
		return nil, fmt.Errorf("avatar: not a picture (%s)", ct)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, avatarMax+1))
	if err != nil {
		return nil, err
	}
	if len(data) > avatarMax {
		return nil, fmt.Errorf("avatar: larger than %d KB", avatarMax>>10)
	}
	return &store.Avatar{URL: src, ContentType: ct, Data: data, FetchedAt: time.Now()}, nil
}
