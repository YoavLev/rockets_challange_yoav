package domain

import (
	"errors"
	"fmt"
)

var ErrInvalidMessage = errors.New("invalid message")

// Apply returns r with m applied. It assumes m is the next message in order.
func Apply(r Rocket, m Message) (Rocket, error) {
	switch p := m.Payload.(type) {
	case RocketLaunched:
		if !r.LaunchTime.IsZero() {
			return r, fmt.Errorf("%w: rocket already launched", ErrInvalidMessage)
		}
		r.Type = p.Type
		r.Speed = p.LaunchSpeed
		r.Mission = p.Mission
		r.LaunchTime = m.Time

	case RocketSpeedIncreased:
		r.Speed += p.Increase

	case RocketSpeedDecreased:
		r.Speed -= p.Decrease

	case RocketExploded:
		r.Exploded = true
		r.ExplosionReason = p.Reason

	case RocketMissionChanged:
		r.Mission = p.NewMission

	default:
		return r, fmt.Errorf("%w: unknown payload %T", ErrInvalidMessage, m.Payload)
	}

	r.LastUpdated = m.Time
	r.LastAppliedMessage = m.Number
	return r, nil
}
