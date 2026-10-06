package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lunar-rockets/internal/store"
)

func newHandler() http.Handler {
	logger := slog.New(slog.DiscardHandler)
	return NewHandler(store.New(logger), logger)
}

func do(h http.Handler, method, path, reqBody string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(reqBody)))
	return rec
}

func TestPostMessage(t *testing.T) {
	h := newHandler()
	launch := body(1, "RocketLaunched", `{"type":"Falcon-9","launchSpeed":500,"mission":"ARTEMIS"}`)

	steps := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{"first launch is applied", launch, http.StatusOK, `{"status":"applied"}`},
		{"redelivered launch is a duplicate, still 200", launch, http.StatusOK, `{"status":"duplicate"}`},
		{"early message is buffered", body(3, "RocketSpeedIncreased", `{"by":100}`), http.StatusOK, `{"status":"buffered"}`},
	}
	for _, step := range steps {
		rec := do(h, http.MethodPost, "/messages", step.body)
		if rec.Code != step.wantStatus || strings.TrimSpace(rec.Body.String()) != step.wantBody {
			t.Errorf("%s: got %d %s, want %d %s", step.name, rec.Code, rec.Body, step.wantStatus, step.wantBody)
		}
	}
}

func TestPostMessageRejectsMalformedBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"not JSON", `not json`},
		{"unknown message type", body(1, "RocketLanded", `{}`)},
		{"body over 1 MB", body(1, "RocketExploded", `{"reason":"`+strings.Repeat("x", maxBodyBytes)+`"}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(newHandler(), http.MethodPost, "/messages", tt.body)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"error"`) {
				t.Errorf("got %d %s, want 400 with an error", rec.Code, rec.Body)
			}
		})
	}
}

func TestPostMessageWrongMethod(t *testing.T) {
	if rec := do(newHandler(), http.MethodGet, "/messages", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /messages = %d, want 405", rec.Code)
	}
}
