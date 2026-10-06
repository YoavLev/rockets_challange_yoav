package domain_test

import (
	"errors"
	"testing"
	"time"

	"lunar-rockets/internal/domain"
)

const channel = "193270a9-c9cf-404a-8f83-838e71d9ae67"

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// at returns the time of message n, so each message has a distinct time.
func at(n int) time.Time {
	return t0.Add(time.Duration(n) * time.Second)
}

func msg(n int, p domain.Payload) domain.Message {
	return domain.Message{Channel: channel, Number: n, Time: at(n), Payload: p}
}

func TestApply(t *testing.T) {
	launched := domain.Rocket{
		Channel: channel, Type: "Falcon-9", Speed: 500, Mission: "ARTEMIS",
		LaunchTime: at(1), LastUpdated: at(1), LastAppliedMessage: 1,
	}

	tests := []struct {
		name    string
		before  domain.Rocket
		msg     domain.Message
		want    domain.Rocket
		wantErr error
	}{
		{
			name:   "launch sets type, speed, mission and launch time",
			before: domain.Rocket{Channel: channel},
			msg:    msg(1, domain.RocketLaunched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"}),
			want:   launched,
		},
		{
			name:   "speed increased adds to speed",
			before: launched,
			msg:    msg(2, domain.RocketSpeedIncreased{Increase: 3000}),
			want: domain.Rocket{
				Channel: channel, Type: "Falcon-9", Speed: 3500, Mission: "ARTEMIS",
				LaunchTime: at(1), LastUpdated: at(2), LastAppliedMessage: 2,
			},
		},
		{
			name:   "speed decreased subtracts from speed",
			before: launched,
			msg:    msg(2, domain.RocketSpeedDecreased{Decrease: 200}),
			want: domain.Rocket{
				Channel: channel, Type: "Falcon-9", Speed: 300, Mission: "ARTEMIS",
				LaunchTime: at(1), LastUpdated: at(2), LastAppliedMessage: 2,
			},
		},
		{
			name:   "speed decrease below zero is allowed",
			before: launched,
			msg:    msg(2, domain.RocketSpeedDecreased{Decrease: 700}),
			want: domain.Rocket{
				Channel: channel, Type: "Falcon-9", Speed: -200, Mission: "ARTEMIS",
				LaunchTime: at(1), LastUpdated: at(2), LastAppliedMessage: 2,
			},
		},
		{
			name:   "exploded sets exploded and reason",
			before: launched,
			msg:    msg(2, domain.RocketExploded{Reason: "PRESSURE_VESSEL_FAILURE"}),
			want: domain.Rocket{
				Channel: channel, Type: "Falcon-9", Speed: 500, Mission: "ARTEMIS",
				Exploded: true, ExplosionReason: "PRESSURE_VESSEL_FAILURE",
				LaunchTime: at(1), LastUpdated: at(2), LastAppliedMessage: 2,
			},
		},
		{
			name:   "mission changed replaces mission",
			before: launched,
			msg:    msg(2, domain.RocketMissionChanged{NewMission: "SHUTTLE_MIR"}),
			want: domain.Rocket{
				Channel: channel, Type: "Falcon-9", Speed: 500, Mission: "SHUTTLE_MIR",
				LaunchTime: at(1), LastUpdated: at(2), LastAppliedMessage: 2,
			},
		},
		{
			name: "speed change after explosion is applied and rocket stays exploded",
			before: domain.Rocket{
				Channel: channel, Type: "Falcon-9", Speed: 500, Mission: "ARTEMIS",
				Exploded: true, ExplosionReason: "PRESSURE_VESSEL_FAILURE",
				LaunchTime: at(1), LastUpdated: at(2), LastAppliedMessage: 2,
			},
			msg: msg(3, domain.RocketSpeedIncreased{Increase: 100}),
			want: domain.Rocket{
				Channel: channel, Type: "Falcon-9", Speed: 600, Mission: "ARTEMIS",
				Exploded: true, ExplosionReason: "PRESSURE_VESSEL_FAILURE",
				LaunchTime: at(1), LastUpdated: at(3), LastAppliedMessage: 3,
			},
		},
		{
			name:    "second launch is invalid and leaves rocket unchanged",
			before:  launched,
			msg:     msg(2, domain.RocketLaunched{Type: "Atlas-V", LaunchSpeed: 900, Mission: "APOLLO"}),
			want:    launched,
			wantErr: domain.ErrInvalidMessage,
		},
		{
			name:    "nil payload is invalid and leaves rocket unchanged",
			before:  launched,
			msg:     msg(2, nil),
			want:    launched,
			wantErr: domain.ErrInvalidMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.Apply(tt.before, tt.msg)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("\n got  %+v\n want %+v", got, tt.want)
			}
		})
	}
}
