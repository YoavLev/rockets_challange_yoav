package httpapi

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"lunar-rockets/internal/domain"
)

const channel = "193270a9-c9cf-404a-8f83-838e71d9ae67"

// body builds a producer message with a fixed channel and time.
func body(number int, messageType, message string) string {
	return `{"metadata":{"channel":"` + channel + `","messageNumber":` + strconv.Itoa(number) + `,` +
		`"messageTime":"2022-02-02T19:39:05.86337+01:00","messageType":"` + messageType + `"},` +
		`"message":` + message + `}`
}

func TestDecodeMessage(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    domain.Payload
		wantErr bool
	}{
		{
			name: "launched",
			body: body(1, "RocketLaunched", `{"type":"Falcon-9","launchSpeed":500,"mission":"ARTEMIS"}`),
			want: domain.RocketLaunched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"},
		},
		{
			name: "speed increased",
			body: body(1, "RocketSpeedIncreased", `{"by":3000}`),
			want: domain.RocketSpeedIncreased{Increase: 3000},
		},
		{
			name: "speed decreased",
			body: body(1, "RocketSpeedDecreased", `{"by":2500}`),
			want: domain.RocketSpeedDecreased{Decrease: 2500},
		},
		{
			name: "exploded",
			body: body(1, "RocketExploded", `{"reason":"PRESSURE_VESSEL_FAILURE"}`),
			want: domain.RocketExploded{Reason: "PRESSURE_VESSEL_FAILURE"},
		},
		{
			name: "mission changed",
			body: body(1, "RocketMissionChanged", `{"newMission":"SHUTTLE_MIR"}`),
			want: domain.RocketMissionChanged{NewMission: "SHUTTLE_MIR"},
		},
		{
			name: "unknown fields are ignored",
			body: body(1, "RocketSpeedIncreased", `{"by":3000,"unit":"km/h"}`),
			want: domain.RocketSpeedIncreased{Increase: 3000},
		},
		{name: "not JSON", body: `not json`, wantErr: true},
		{name: "unknown message type", body: body(1, "RocketLanded", `{}`), wantErr: true},
		{name: "missing message", body: strings.Replace(body(1, "RocketExploded", `{}`), `,"message":{}`, "", 1), wantErr: true},
		{name: "missing channel", body: strings.Replace(body(1, "RocketExploded", `{}`), channel, "", 1), wantErr: true},
		{name: "message number zero", body: body(0, "RocketExploded", `{}`), wantErr: true},
		{name: "unparsable time", body: strings.Replace(body(1, "RocketExploded", `{}`), "2022-02-02T19:39:05.86337+01:00", "yesterday", 1), wantErr: true},
	}

	wantTime := time.Date(2022, 2, 2, 18, 39, 5, 863370000, time.UTC)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeMessage(strings.NewReader(tt.body))

			if tt.wantErr {
				if err == nil {
					t.Fatalf("got %+v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Channel != channel || got.Number != 1 || !got.Time.Equal(wantTime) {
				t.Errorf("metadata = %s, %d, %v; want %s, 1, %v", got.Channel, got.Number, got.Time, channel, wantTime)
			}
			if got.Payload != tt.want {
				t.Errorf("payload = %#v, want %#v", got.Payload, tt.want)
			}
		})
	}
}
