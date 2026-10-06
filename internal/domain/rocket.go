package domain

import "time"

// Rocket is the state of one rocket after applying its messages in order.
// It holds only values, so copying a Rocket gives an independent copy.
// The zero value (plus a channel) is a rocket that hasn't launched yet.
type Rocket struct {
	Channel            string
	Type               string
	Mission            string
	Speed              int
	Exploded           bool
	ExplosionReason    string
	LaunchTime         time.Time
	LastUpdated        time.Time
	LastAppliedMessage int
}
