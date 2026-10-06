package domain

import "time"

type Message struct {
	Channel string
	Number  int
	Time    time.Time
	Payload Payload
}

type Payload interface{ isPayload() } // Marker interface for all payload types.

type (
	RocketLaunched struct {
		Type        string
		LaunchSpeed int
		Mission     string
	}

	RocketSpeedIncreased struct {
		Increase int
	}

	RocketSpeedDecreased struct {
		Decrease int
	}

	RocketExploded struct {
		Reason string
	}

	RocketMissionChanged struct {
		NewMission string
	}
)

func (RocketLaunched) isPayload()       {}
func (RocketSpeedIncreased) isPayload() {}
func (RocketSpeedDecreased) isPayload() {}
func (RocketExploded) isPayload()       {}
func (RocketMissionChanged) isPayload() {}
