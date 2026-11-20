package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"hub.localhost/libs/go-appkit"
)

func TestGreetingCountsVisits(t *testing.T) {
	visits, err := newCounter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := greeting(appkit.Config{AppID: "hello"}, visits)
	for want := 1; want <= 3; want++ {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest("GET", "/api/greeting", nil))
		var body struct {
			Message string
			Visits  int
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Visits != want || body.Message != "Hello from hello!" {
			t.Fatalf("visit %d: %+v", want, body)
		}
	}
	if err := visits.check(t.Context()); err != nil {
		t.Errorf("check: %v", err)
	}
}
