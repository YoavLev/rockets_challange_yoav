package domain_test

import (
	"errors"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"lunar-rockets/internal/domain"
)

func launch() domain.Message {
	return msg(1, domain.RocketLaunched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"})
}

func speedUp(n int) domain.Message {
	return msg(n, domain.RocketSpeedIncreased{Increase: 100})
}

// rocketExpected returns the  expected rocket after launch and 2..n speed increases applied.
func rocketExpected(n int) domain.Rocket {
	return domain.Rocket{
		Channel: channel, Type: "Falcon-9", Mission: "ARTEMIS",
		Speed:      500 + 100*(n-1),
		LaunchTime: at(1), LastUpdated: at(n), LastAppliedMessage: n,
	}
}

func TestReceive(t *testing.T) {
	const (
		A = domain.OutcomeApplied
		B = domain.OutcomeBuffered
		D = domain.OutcomeDuplicate
	)

	tests := []struct {
		name         string
		arrivals     []domain.Message
		wantOutcomes []domain.Outcome
		wantRocket   domain.Rocket
		wantPending  int
	}{
		{
			name:         "in order",
			arrivals:     []domain.Message{launch(), speedUp(2), speedUp(3)},
			wantOutcomes: []domain.Outcome{A, A, A},
			wantRocket:   rocketExpected(3),
			wantPending:  0,
		},
		{
			name:         "reversed",
			arrivals:     []domain.Message{speedUp(3), speedUp(2), launch()},
			wantOutcomes: []domain.Outcome{B, B, A},
			wantRocket:   rocketExpected(3),
			wantPending:  0,
		},
		{
			name:         "worked example from the plan",
			arrivals:     []domain.Message{launch(), speedUp(3), speedUp(3), speedUp(2), speedUp(5), speedUp(2), speedUp(4)},
			wantOutcomes: []domain.Outcome{A, B, D, A, B, D, A},
			wantRocket:   rocketExpected(5),
			wantPending:  0,
		},
		{
			name:         "duplicate of applied message",
			arrivals:     []domain.Message{launch(), speedUp(2), launch()},
			wantOutcomes: []domain.Outcome{A, A, D},
			wantRocket:   rocketExpected(2),
			wantPending:  0,
		},
		{
			name:         "duplicate of pending message",
			arrivals:     []domain.Message{launch(), speedUp(3), speedUp(3)},
			wantOutcomes: []domain.Outcome{A, B, D},
			wantRocket:   rocketExpected(1),
			wantPending:  1,
		},
		{
			name:         "permanent gap keeps later messages pending",
			arrivals:     []domain.Message{launch(), speedUp(3), speedUp(4)},
			wantOutcomes: []domain.Outcome{A, B, B},
			wantRocket:   rocketExpected(1),
			wantPending:  2,
		},
		{
			name:         "launch never arrives",
			arrivals:     []domain.Message{speedUp(2), speedUp(3)},
			wantOutcomes: []domain.Outcome{B, B},
			wantRocket:   domain.Rocket{Channel: channel},
			wantPending:  2,
		},
		{
			name:         "duplicate with different content: first one wins",
			arrivals:     []domain.Message{launch(), speedUp(2), msg(2, domain.RocketSpeedIncreased{Increase: 999})},
			wantOutcomes: []domain.Outcome{A, A, D},
			wantRocket:   rocketExpected(2),
			wantPending:  0,
		},
		{
			name:         "duplicate of message already drained from pending",
			arrivals:     []domain.Message{launch(), speedUp(3), speedUp(2), speedUp(3)},
			wantOutcomes: []domain.Outcome{A, B, A, D},
			wantRocket:   rocketExpected(3),
			wantPending:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := domain.NewStream(channel)
			var got []domain.Outcome
			for _, m := range tt.arrivals {
				outcome, err := s.Receive(m)
				if err != nil {
					t.Fatalf("Receive(%d): unexpected error: %v", m.Number, err)
				}
				got = append(got, outcome)
			}
			if !slices.Equal(got, tt.wantOutcomes) {
				t.Errorf("outcomes = %v, want %v", got, tt.wantOutcomes)
			}
			if s.Rocket() != tt.wantRocket {
				t.Errorf("\n got  %+v\n want %+v", s.Rocket(), tt.wantRocket)
			}
			if s.PendingLen() != tt.wantPending {
				t.Errorf("pending = %d, want %d", s.PendingLen(), tt.wantPending)
			}
		})

	}
}

func TestReceiveSkipsInvalidMessage(t *testing.T) {
	s := domain.NewStream(channel)
	secondLaunch := msg(2, domain.RocketLaunched{Type: "Atlas-V", LaunchSpeed: 900, Mission: "APOLLO"})

	mustReceive(t, s, launch(), domain.OutcomeApplied)
	mustReceive(t, s, speedUp(3), domain.OutcomeBuffered)

	// Message 2 is rejected by Apply, but the stream moves past it and drains 3.
	outcome, err := s.Receive(secondLaunch)
	if !errors.Is(err, domain.ErrInvalidMessage) {
		t.Fatalf("Receive(2) error = %v, want %v", err, domain.ErrInvalidMessage)
	}
	if outcome != domain.OutcomeApplied {
		t.Errorf("Receive(2) outcome = %v, want %v", outcome, domain.OutcomeApplied)
	}

	// The stream isn't stuck: later messages still apply.
	mustReceive(t, s, speedUp(4), domain.OutcomeApplied)

	want := domain.Rocket{
		Channel: channel, Type: "Falcon-9", Mission: "ARTEMIS", Speed: 700,
		LaunchTime: at(1), LastUpdated: at(4), LastAppliedMessage: 4,
	}
	if s.Rocket() != want {
		t.Errorf("\n got  %+v\n want %+v", s.Rocket(), want)
	}
	if s.PendingLen() != 0 {
		t.Errorf("pending = %d, want 0", s.PendingLen())
	}
}

// mustReceive feeds m to s and fails the test on an error or an unexpected outcome.
func mustReceive(t *testing.T, s *domain.Stream, m domain.Message, want domain.Outcome) {
	t.Helper()
	got, err := s.Receive(m)
	if err != nil {
		t.Fatalf("Receive(%d): unexpected error: %v", m.Number, err)
	}
	if got != want {
		t.Fatalf("Receive(%d) outcome = %v, want %v", m.Number, got, want)
	}
}

// Any delivery order with any duplicates must give the same rocket as
// applying the stream once, in order.
func TestReceiveAnyDeliveryOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Mission changes make the test order-sensitive; increases alone commute.
		length := rapid.IntRange(1, 30).Draw(t, "length")
		stream := []domain.Message{launch()}
		for n := 2; n <= length; n++ {
			var p domain.Payload = domain.RocketSpeedIncreased{Increase: rapid.IntRange(1, 1000).Draw(t, "increase")}
			if rapid.Bool().Draw(t, "missionChange") {
				p = domain.RocketMissionChanged{NewMission: rapid.SampledFrom([]string{"A", "B", "C"}).Draw(t, "mission")}
			}
			stream = append(stream, msg(n, p))
		}

		want := domain.Rocket{Channel: channel}
		for _, m := range stream {
			var err error
			if want, err = domain.Apply(want, m); err != nil {
				t.Fatalf("Apply(%d): %v", m.Number, err)
			}
		}

		duplicates := rapid.SliceOf(rapid.SampledFrom(stream)).Draw(t, "duplicates")
		delivery := rapid.Permutation(append(slices.Clone(stream), duplicates...)).Draw(t, "delivery")

		s := domain.NewStream(channel)
		gotDuplicates := 0
		for _, m := range delivery {
			outcome, err := s.Receive(m)
			if err != nil {
				t.Fatalf("Receive(%d): %v", m.Number, err)
			}
			if outcome == domain.OutcomeDuplicate {
				gotDuplicates++
			}
		}

		if s.Rocket() != want {
			t.Fatalf("\n got  %+v\n want %+v", s.Rocket(), want)
		}
		if s.PendingLen() != 0 {
			t.Fatalf("pending = %d, want 0", s.PendingLen())
		}
		if gotDuplicates != len(duplicates) {
			t.Fatalf("duplicates = %d, want %d", gotDuplicates, len(duplicates))
		}
	})
}

func TestOutcomeRecorded(t *testing.T) {
	for _, o := range []domain.Outcome{domain.OutcomeApplied, domain.OutcomeBuffered, domain.OutcomeDuplicate} {
		if !o.Recorded() {
			t.Errorf("%s.Recorded() = false, want true", o)
		}
	}
	if domain.Outcome("retry-later").Recorded() {
		t.Error("an unknown outcome must not count as recorded, or the producer would never resend it")
	}
}
