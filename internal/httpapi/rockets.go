package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"lunar-rockets/internal/store"
)

// rocketJSON is the API's view of a rocket. Times are omitted while zero,
// e.g. before launch, rather than shown as year 1.
type rocketJSON struct {
	Channel            string    `json:"channel"`
	Type               string    `json:"type"`
	Mission            string    `json:"mission"`
	Speed              int       `json:"speed"`
	Exploded           bool      `json:"exploded"`
	ExplosionReason    string    `json:"explosionReason"`
	LaunchTime         time.Time `json:"launchTime,omitzero"`
	LastUpdated        time.Time `json:"lastUpdated,omitzero"`
	LastAppliedMessage int       `json:"lastAppliedMessage"`
	PendingMessages    int       `json:"pendingMessages"`
}

func toRocketJSON(snap store.Snapshot) rocketJSON {
	return rocketJSON{
		Channel:            snap.Channel,
		Type:               snap.Type,
		Mission:            snap.Mission,
		Speed:              snap.Speed,
		Exploded:           snap.Exploded,
		ExplosionReason:    snap.ExplosionReason,
		LaunchTime:         snap.LaunchTime,
		LastUpdated:        snap.LastUpdated,
		LastAppliedMessage: snap.LastAppliedMessage,
		PendingMessages:    snap.Pending,
	}
}

func (s *server) getRocket(w http.ResponseWriter, r *http.Request) {
	snap, ok := s.store.Get(r.PathValue("channel"))
	if !ok {
		s.writeError(w, http.StatusNotFound, errors.New("rocket not found"))
		return
	}
	s.writeJSON(w, http.StatusOK, toRocketJSON(snap))
}

// listRockets serves GET /rockets?sort=speed&order=desc. Both parameters are
// optional; the default is ascending by channel.
func (s *server) listRockets(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	by := store.SortByChannel
	if v := query.Get("sort"); v != "" {
		by = store.SortBy(v)
	}
	if !by.Valid() {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("unknown sort %q", by))
		return
	}

	var desc bool
	switch order := query.Get("order"); order {
	case "", "asc":
	case "desc":
		desc = true
	default:
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("unknown order %q, want asc or desc", order))
		return
	}

	snaps := s.store.List(by, desc)
	rockets := make([]rocketJSON, 0, len(snaps)) // non-nil, so no rockets encodes as [] rather than null
	for _, snap := range snaps {
		rockets = append(rockets, toRocketJSON(snap))
	}
	s.writeJSON(w, http.StatusOK, rockets)
}
