package store

import (
	"cmp"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"lunar-rockets/internal/domain"
)

type Store struct {
	logger   *slog.Logger
	mu       sync.RWMutex
	streams  map[string]*domain.Stream // keyed by channel
	received int                       // messages received, duplicates included
}

func New(logger *slog.Logger) *Store {
	return &Store{
		logger:  logger,
		streams: make(map[string]*domain.Stream),
	}
}

// Snapshot is a copy of one rocket's state at the moment it was read.
type Snapshot struct {
	domain.Rocket
	Pending int
}

type SortBy string

const (
	SortByChannel    SortBy = "channel"
	SortBySpeed      SortBy = "speed"
	SortByMission    SortBy = "mission"
	SortByType       SortBy = "type"
	SortByLaunchTime SortBy = "launchTime"
)

func (by SortBy) Valid() bool {
	switch by {
	case SortByChannel, SortBySpeed, SortByMission, SortByType, SortByLaunchTime:
		return true
	}
	return false
}

// Receive records m and reports what happened to it. Skipped messages and
// speed dropping below zero are logged as warnings.
func (s *Store) Receive(m domain.Message) domain.Outcome {
	s.mu.Lock()
	s.received++
	stream, ok := s.streams[m.Channel]
	if !ok {
		stream = domain.NewStream(m.Channel)
		s.streams[m.Channel] = stream
	}
	speedBefore := stream.Rocket().Speed
	outcome, err := stream.Receive(m)
	speedAfter := stream.Rocket().Speed
	s.mu.Unlock()

	if err != nil {
		s.logger.Warn("skipped invalid message", "channel", m.Channel, "error", err)
	}
	if speedBefore >= 0 && speedAfter < 0 {
		s.logger.Warn("speed dropped below zero", "channel", m.Channel, "speed", speedAfter)
	}
	return outcome
}

func (s *Store) Get(channel string) (Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stream, ok := s.streams[channel]
	if !ok {
		return Snapshot{}, false
	}
	return snapshot(stream), true
}

func snapshot(stream *domain.Stream) Snapshot {
	return Snapshot{Rocket: stream.Rocket(), Pending: stream.PendingLen()}
}

// List returns every rocket sorted by the given key. Ties are broken by
// channel, ascending, so the order is the same between calls.
func (s *Store) List(by SortBy, desc bool) []Snapshot {
	s.mu.RLock()
	snaps := make([]Snapshot, 0, len(s.streams))
	for _, stream := range s.streams {
		snaps = append(snaps, snapshot(stream))
	}
	s.mu.RUnlock()

	slices.SortFunc(snaps, func(a, b Snapshot) int {
		c := compare(a, b, by)
		if desc {
			c = -c
		}
		if c != 0 {
			return c
		}
		return strings.Compare(a.Channel, b.Channel)
	})
	return snaps
}

func compare(a, b Snapshot, by SortBy) int {
	switch by {
	case SortBySpeed:
		return cmp.Compare(a.Speed, b.Speed)
	case SortByMission:
		return strings.Compare(a.Mission, b.Mission)
	case SortByType:
		return strings.Compare(a.Type, b.Type)
	case SortByLaunchTime:
		return a.LaunchTime.Compare(b.LaunchTime)
	default:
		return strings.Compare(a.Channel, b.Channel)
	}
}

type Stats struct {
	Messages int // received, duplicates included
	Rockets  int
}

func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Stats{Messages: s.received, Rockets: len(s.streams)}
}
