package store_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"lunar-rockets/internal/domain"
	"lunar-rockets/internal/store"
)

const channel = "193270a9-c9cf-404a-8f83-838e71d9ae67"

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func msg(ch string, n int, p domain.Payload) domain.Message {
	return domain.Message{Channel: ch, Number: n, Time: t0.Add(time.Duration(n) * time.Second), Payload: p}
}

func launch(ch string) domain.Message {
	return msg(ch, 1, domain.RocketLaunched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"})
}

func newStore() *store.Store {
	return store.New(slog.New(slog.DiscardHandler))
}

func TestReceiveAndGet(t *testing.T) {
	s := newStore()

	if got := s.Receive(launch(channel)); got != domain.OutcomeApplied {
		t.Fatalf("Receive(1) = %v, want %v", got, domain.OutcomeApplied)
	}
	if got := s.Receive(msg(channel, 3, domain.RocketSpeedIncreased{Increase: 100})); got != domain.OutcomeBuffered {
		t.Fatalf("Receive(3) = %v, want %v", got, domain.OutcomeBuffered)
	}

	snap, ok := s.Get(channel)
	if !ok {
		t.Fatalf("Get(%q) not found", channel)
	}
	if snap.Speed != 500 || snap.LastAppliedMessage != 1 || snap.Pending != 1 {
		t.Errorf("got speed %d, last applied %d, pending %d; want 500, 1, 1",
			snap.Speed, snap.LastAppliedMessage, snap.Pending)
	}
}

func TestGetUnknownChannel(t *testing.T) {
	if _, ok := newStore().Get("unknown"); ok {
		t.Error("Get(unknown) found a rocket, want not found")
	}
}

func TestReceiveWarnsOnceWhenSpeedDropsBelowZero(t *testing.T) {
	var logs bytes.Buffer
	s := store.New(slog.New(slog.NewTextHandler(&logs, nil)))

	s.Receive(launch(channel))
	s.Receive(msg(channel, 2, domain.RocketSpeedDecreased{Decrease: 600})) // 500 → -100
	s.Receive(msg(channel, 3, domain.RocketSpeedDecreased{Decrease: 50}))  // still negative

	if n := strings.Count(logs.String(), "speed dropped below zero"); n != 1 {
		t.Errorf("warnings = %d, want 1\nlogs:\n%s", n, logs.String())
	}
}

func TestList(t *testing.T) {
	s := newStore()
	for _, r := range []struct {
		channel, rocketType, mission string
		speed                        int
		launchedAt                   time.Duration
	}{
		{"a", "Falcon-9", "MARS", 500, 3 * time.Hour},
		{"b", "Atlas-V", "APOLLO", 900, 1 * time.Hour},
		{"c", "Falcon-9", "ARTEMIS", 500, 2 * time.Hour}, // ties with "a" on speed and type
	} {
		s.Receive(domain.Message{
			Channel: r.channel, Number: 1, Time: t0.Add(r.launchedAt),
			Payload: domain.RocketLaunched{Type: r.rocketType, LaunchSpeed: r.speed, Mission: r.mission},
		})
	}

	tests := []struct {
		by   store.SortBy
		desc bool
		want []string
	}{
		{store.SortByChannel, false, []string{"a", "b", "c"}},
		{store.SortByChannel, true, []string{"c", "b", "a"}},
		{store.SortBySpeed, false, []string{"a", "c", "b"}}, // tie on 500 broken by channel
		{store.SortBySpeed, true, []string{"b", "a", "c"}},  // tie still broken ascending
		{store.SortByMission, false, []string{"b", "c", "a"}},
		{store.SortByType, false, []string{"b", "a", "c"}}, // tie on Falcon-9
		{store.SortByLaunchTime, false, []string{"b", "c", "a"}},
		{store.SortByLaunchTime, true, []string{"a", "c", "b"}},
	}
	for _, tt := range tests {
		name := string(tt.by)
		if tt.desc {
			name += "_desc"
		}
		t.Run(name, func(t *testing.T) {
			var got []string
			for _, snap := range s.List(tt.by, tt.desc) {
				got = append(got, snap.Channel)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("order = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSortByValid(t *testing.T) {
	if !store.SortBySpeed.Valid() {
		t.Error("SortBySpeed.Valid() = false, want true")
	}
	if store.SortBy("altitude").Valid() {
		t.Error(`SortBy("altitude").Valid() = true, want false`)
	}
}

// Many writers and readers at once
// Run under -race: the race detector fails the test on any unguarded access.
func TestConcurrentReceive(t *testing.T) {
	const rockets, messages, writers = 20, 50, 8
	s := newStore()

	var deliveries []domain.Message
	for r := range rockets {
		ch := fmt.Sprintf("rocket-%02d", r)
		deliveries = append(deliveries, launch(ch), launch(ch)) // every message twice: at-least-once delivery
		for n := 2; n <= messages; n++ {
			m := msg(ch, n, domain.RocketSpeedIncreased{Increase: 100})
			deliveries = append(deliveries, m, m)
		}
	}
	rng := rand.New(rand.NewPCG(1, 2)) // fixed seed, so a failure is reproducible
	rng.Shuffle(len(deliveries), func(i, j int) {
		deliveries[i], deliveries[j] = deliveries[j], deliveries[i]
	})

	stop := make(chan struct{})
	var readers sync.WaitGroup
	readers.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				s.List(store.SortBySpeed, false)
				s.Get("rocket-00")
			}
		}
	})

	queue := make(chan domain.Message)
	var writersDone sync.WaitGroup
	for range writers {
		writersDone.Go(func() {
			for m := range queue {
				s.Receive(m)
			}
		})
	}
	for _, m := range deliveries {
		queue <- m
	}
	close(queue)
	writersDone.Wait()
	close(stop)
	readers.Wait()

	for r := range rockets {
		ch := fmt.Sprintf("rocket-%02d", r)
		snap, ok := s.Get(ch)
		if !ok {
			t.Fatalf("Get(%q) not found", ch)
		}
		if snap.Speed != 500+100*(messages-1) || snap.LastAppliedMessage != messages || snap.Pending != 0 {
			t.Errorf("%s: speed %d, last applied %d, pending %d; want %d, %d, 0",
				ch, snap.Speed, snap.LastAppliedMessage, snap.Pending, 500+100*(messages-1), messages)
		}
	}
}
