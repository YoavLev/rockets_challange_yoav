package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"lunar-rockets/internal/domain"
	"lunar-rockets/internal/store"
)

const maxBodyBytes = 1 << 20 // 1 MB; real messages are a few hundred bytes

type server struct {
	store  *store.Store
	logger *slog.Logger
}

func NewHandler(st *store.Store, logger *slog.Logger) http.Handler {
	s := &server{store: st, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /messages", s.postMessage)
	mux.HandleFunc("GET /rockets/{channel}", s.getRocket)
	mux.HandleFunc("GET /rockets", s.listRockets)
	return mux
}

func (s *server) postMessage(w http.ResponseWriter, r *http.Request) {
	m, err := decodeMessage(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		s.logger.Warn("rejected malformed message", "error", err)
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	outcome := s.store.Receive(m)
	s.writeJSON(w, statusFor(outcome), map[string]domain.Outcome{"status": outcome})
}

// statusFor translates an outcome into the producer's protocol: anything
// other than 2xx makes it resend the message.
func statusFor(outcome domain.Outcome) int {
	if outcome.Recorded() {
		return http.StatusOK
	}
	return http.StatusServiceUnavailable
}

func (s *server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("write response", "error", err)
	}
}

func (s *server) writeError(w http.ResponseWriter, status int, err error) {
	s.writeJSON(w, status, map[string]string{"error": err.Error()})
}
