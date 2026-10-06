package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"lunar-rockets/internal/domain"
)

type envelopeJSON struct {
	Metadata struct {
		Channel       string    `json:"channel"`
		MessageNumber int       `json:"messageNumber"`
		MessageTime   time.Time `json:"messageTime"`
		MessageType   string    `json:"messageType"`
	} `json:"metadata"`
	Message json.RawMessage `json:"message"`
}

// payloadJSON holds the fields of every message type. Their names don't
// clash, so one struct and one decode cover all five.
type payloadJSON struct {
	Type        string `json:"type"`
	LaunchSpeed int    `json:"launchSpeed"`
	Mission     string `json:"mission"`
	By          int    `json:"by"`
	Reason      string `json:"reason"`
	NewMission  string `json:"newMission"`
}

// decodeMessage turns the producer's JSON into a domain message. Any error
// means the body is malformed.
func decodeMessage(r io.Reader) (domain.Message, error) {
	var env envelopeJSON
	if err := json.NewDecoder(r).Decode(&env); err != nil {
		return domain.Message{}, fmt.Errorf("decode envelope: %w", err)
	}

	md := env.Metadata
	switch {
	case md.Channel == "":
		return domain.Message{}, errors.New("missing metadata.channel")
	case md.MessageNumber < 1:
		return domain.Message{}, fmt.Errorf("metadata.messageNumber must be at least 1, got %d", md.MessageNumber)
	case md.MessageTime.IsZero():
		return domain.Message{}, errors.New("missing metadata.messageTime")
	}

	var p payloadJSON
	if err := json.Unmarshal(env.Message, &p); err != nil {
		return domain.Message{}, fmt.Errorf("decode message: %w", err)
	}

	var payload domain.Payload
	switch md.MessageType {
	case "RocketLaunched":
		payload = domain.RocketLaunched{Type: p.Type, LaunchSpeed: p.LaunchSpeed, Mission: p.Mission}
	case "RocketSpeedIncreased":
		payload = domain.RocketSpeedIncreased{Increase: p.By}
	case "RocketSpeedDecreased":
		payload = domain.RocketSpeedDecreased{Decrease: p.By}
	case "RocketExploded":
		payload = domain.RocketExploded{Reason: p.Reason}
	case "RocketMissionChanged":
		payload = domain.RocketMissionChanged{NewMission: p.NewMission}
	default:
		return domain.Message{}, fmt.Errorf("unknown metadata.messageType %q", md.MessageType)
	}

	return domain.Message{
		Channel: md.Channel,
		Number:  md.MessageNumber,
		Time:    md.MessageTime,
		Payload: payload,
	}, nil
}
