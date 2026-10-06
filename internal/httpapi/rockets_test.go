package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestGetRocket(t *testing.T) {
	h := newHandler()
	do(h, http.MethodPost, "/messages", body(1, "RocketLaunched", `{"type":"Falcon-9","launchSpeed":500,"mission":"ARTEMIS"}`))
	do(h, http.MethodPost, "/messages", body(2, "RocketSpeedIncreased", `{"by":100}`))

	rec := do(h, http.MethodGet, "/rockets/"+channel, "")

	want := `{"channel":"` + channel + `","type":"Falcon-9","mission":"ARTEMIS","speed":600,` +
		`"exploded":false,"explosionReason":"",` +
		`"launchTime":"2022-02-02T19:39:05.86337+01:00","lastUpdated":"2022-02-02T19:39:05.86337+01:00",` +
		`"lastAppliedMessage":2,"pendingMessages":0}`
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("got %d\n %s\nwant 200\n %s", rec.Code, rec.Body, want)
	}
}

func TestGetRocketBeforeLaunchOmitsTimes(t *testing.T) {
	h := newHandler()
	do(h, http.MethodPost, "/messages", body(2, "RocketSpeedIncreased", `{"by":100}`))

	rec := do(h, http.MethodGet, "/rockets/"+channel, "")

	want := `{"channel":"` + channel + `","type":"","mission":"","speed":0,` +
		`"exploded":false,"explosionReason":"","lastAppliedMessage":0,"pendingMessages":1}`
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("got %d\n %s\nwant 200\n %s", rec.Code, rec.Body, want)
	}
}

func TestGetRocketUnknown(t *testing.T) {
	if rec := do(newHandler(), http.MethodGet, "/rockets/unknown", ""); rec.Code != http.StatusNotFound {
		t.Errorf("GET /rockets/unknown = %d, want 404", rec.Code)
	}
}

func TestListRockets(t *testing.T) {
	h := newHandler()
	for ch, speed := range map[string]int{"a": 700, "b": 500, "c": 900} {
		launch := body(1, "RocketLaunched", `{"type":"Falcon-9","launchSpeed":`+strconv.Itoa(speed)+`,"mission":"ARTEMIS"}`)
		do(h, http.MethodPost, "/messages", strings.Replace(launch, channel, ch, 1))
	}

	tests := []struct {
		query      string
		wantStatus int
		want       []string // channels, in order
	}{
		{"", http.StatusOK, []string{"a", "b", "c"}},
		{"?sort=speed", http.StatusOK, []string{"b", "a", "c"}},
		{"?sort=speed&order=desc", http.StatusOK, []string{"c", "a", "b"}},
		{"?sort=altitude", http.StatusBadRequest, nil},
		{"?order=sideways", http.StatusBadRequest, nil},
	}
	for _, tt := range tests {
		name := tt.query
		if name == "" {
			name = "no query"
		}
		t.Run(name, func(t *testing.T) {
			rec := do(h, http.MethodGet, "/rockets"+tt.query, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}

			var rockets []rocketJSON
			if err := json.Unmarshal(rec.Body.Bytes(), &rockets); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			var got []string
			for _, r := range rockets {
				got = append(got, r.Channel)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("order = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListRocketsEmpty(t *testing.T) {
	rec := do(newHandler(), http.MethodGet, "/rockets", "")
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != "[]" {
		t.Errorf("got %d %s, want 200 []", rec.Code, got)
	}
}
