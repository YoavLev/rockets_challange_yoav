package domain

import (
	"errors"
	"fmt"
)

type Outcome string

const (
	OutcomeBuffered  Outcome = "buffered"
	OutcomeApplied   Outcome = "applied"
	OutcomeDuplicate Outcome = "duplicate"
)

// Recorded reports whether the message is safely stored, so the producer
// doesn't need to send it again. An outcome added later (e.g. for a full
// buffer) returns false here, and the producer will redeliver.
func (o Outcome) Recorded() bool {
	switch o {
	case OutcomeApplied, OutcomeBuffered, OutcomeDuplicate:
		return true
	default:
		return false
	}
}

// Stream feeds one rocket's messages to Apply in order, each exactly once.
type Stream struct {
	rocket  Rocket
	handled int             // highest message number handled with no gaps before
	pending map[int]Message // early messages, keyed by number
}

func NewStream(channel string) *Stream {
	return &Stream{
		rocket:  Rocket{Channel: channel},
		pending: make(map[int]Message),
	}
}

func (s *Stream) Rocket() Rocket {
	return s.rocket
}

func (s *Stream) PendingLen() int {
	return len(s.pending)
}

func (s *Stream) Receive(m Message) (Outcome, error) {
	if m.Number <= s.handled {
		return OutcomeDuplicate, nil
	}
	if _, ok := s.pending[m.Number]; ok {
		return OutcomeDuplicate, nil
	}
	if m.Number > s.handled+1 {
		s.pending[m.Number] = m
		return OutcomeBuffered, nil
	}

	// m is the next message in order. Add it to pending so a single loop
	// applies it and then every pending message that follows without a gap.
	s.pending[m.Number] = m
	var errs []error
	for {
		next, ok := s.pending[s.handled+1]
		if !ok {
			break
		}
		delete(s.pending, next.Number)
		s.handled = next.Number

		r, err := Apply(s.rocket, next)
		if err != nil {
			errs = append(errs, fmt.Errorf("message %d: %w", next.Number, err))
			continue
		}
		s.rocket = r
	}

	return OutcomeApplied, errors.Join(errs...)
}
