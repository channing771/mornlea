// Package presentation defines renderer-independent values published by the client runtime.
//
// Snapshots contain no mutable reference-bearing fields. Producers validate complete values
// before publication, and readiness flags keep zero values safe without inventing confirmed
// world or player state.
package presentation

import (
	"errors"
	"math"
	"unicode/utf8"

	"github.com/channing771/mornlea/packages/shared/core"
)

// `SessionPhase` is the renderer-independent lifecycle visible to presentation hosts.
// Its zero value is deliberately not ready and does not imply a live connection.
type SessionPhase uint8

const (
	// `SessionPhaseNotReady` means no session lifecycle has been published yet.
	SessionPhaseNotReady SessionPhase = iota
	// `SessionPhaseConnecting` covers asynchronous transport and handshake setup.
	SessionPhaseConnecting
	// `SessionPhaseLogin` means the transport handshake completed and login is in progress.
	SessionPhaseLogin
	// `SessionPhaseLoading` means login succeeded but the initial world criterion is incomplete.
	SessionPhaseLoading
	// `SessionPhasePlay` means the session may publish playable presentation state.
	SessionPhasePlay
	// `SessionPhaseDisconnected` is the terminal phase for a completed or failed session.
	SessionPhaseDisconnected
)

// `Valid` reports whether the phase is a defined lifecycle value.
func (phase SessionPhase) Valid() bool {
	return phase <= SessionPhaseDisconnected
}

// `CameraSnapshot` is a complete camera pose and projection in Mornlea coordinates.
// `Ready` gates every payload field so a copied zero value cannot masquerade as a camera.
type CameraSnapshot struct {
	Ready    bool
	Position [3]float32
	Yaw      float32
	Pitch    float32
	FOVY     float32
	Aspect   float32
	Near     float32
	Far      float32
}

// `Validate` rejects stale not-ready payloads and projection values that cannot describe a
// finite perspective camera. Yaw remains unbounded because the current client accumulates it.
func (snapshot CameraSnapshot) Validate() error {
	if !snapshot.Ready {
		if snapshot != (CameraSnapshot{}) {
			return errors.New("presentation: not-ready camera contains payload")
		}
		return nil
	}
	for _, component := range snapshot.Position {
		if !finiteFloat32(component) {
			return errors.New("presentation: camera position is not finite")
		}
	}
	if !finiteFloat32(snapshot.Yaw) || !finiteFloat32(snapshot.Pitch) {
		return errors.New("presentation: camera orientation is not finite")
	}
	if snapshot.Pitch < -cameraPitchLimit || snapshot.Pitch > cameraPitchLimit {
		return errors.New("presentation: camera pitch is outside the supported domain")
	}
	if !finiteFloat32(snapshot.FOVY) || snapshot.FOVY <= 0 || snapshot.FOVY >= float32(math.Pi) {
		return errors.New("presentation: camera field of view is outside the supported domain")
	}
	if !finiteFloat32(snapshot.Aspect) || snapshot.Aspect <= 0 {
		return errors.New("presentation: camera aspect is outside the supported domain")
	}
	if !finiteFloat32(snapshot.Near) || !finiteFloat32(snapshot.Far) ||
		snapshot.Near <= 0 || snapshot.Far <= snapshot.Near {
		return errors.New("presentation: camera clipping planes are outside the supported domain")
	}
	return nil
}

const cameraPitchLimit = float32(math.Pi/2) - 0.01

// `EnvironmentSnapshot` carries confirmed inputs for day, season, and weather presentation.
// It intentionally does not contain renderer colors, shaders, or host resource identifiers.
type EnvironmentSnapshot struct {
	Ready          bool
	ServerTick     uint64
	WorldTimeTicks uint64
	DayPhaseOffset uint16
	Weather        core.WeatherKind
	Season         core.Season
	SeasonProgress uint8
	Temperature    int8
}

// `Validate` applies the shared authoritative domains expected by environment presentation.
func (snapshot EnvironmentSnapshot) Validate() error {
	if !snapshot.Ready {
		if snapshot != (EnvironmentSnapshot{}) {
			return errors.New("presentation: not-ready environment contains payload")
		}
		return nil
	}
	if snapshot.DayPhaseOffset >= core.DayLengthTicks {
		return errors.New("presentation: environment day phase offset is outside the supported domain")
	}
	if snapshot.Weather > core.WeatherThunder {
		return errors.New("presentation: environment weather is outside the supported domain")
	}
	if snapshot.Season > core.SeasonWinter {
		return errors.New("presentation: environment season is outside the supported domain")
	}
	if float32(snapshot.Temperature) < core.TemperatureMin ||
		float32(snapshot.Temperature) > core.TemperatureMax {
		return errors.New("presentation: environment temperature is outside the supported domain")
	}
	return nil
}

// `HUDSnapshot` contains only confirmed minimum-loop player values. A not-ready snapshot
// carries no values, preventing zero health, hunger, or oxygen from being shown as authority.
type HUDSnapshot struct {
	Ready  bool
	Health uint8
	Hunger uint8
	Oxygen uint16
}

// `Validate` applies the shared authoritative player-value domains.
func (snapshot HUDSnapshot) Validate() error {
	if !snapshot.Ready {
		if snapshot != (HUDSnapshot{}) {
			return errors.New("presentation: not-ready HUD contains payload")
		}
		return nil
	}
	if !core.ValidHealth(snapshot.Health) {
		return errors.New("presentation: HUD health is outside the supported domain")
	}
	if !core.ValidHunger(snapshot.Hunger) {
		return errors.New("presentation: HUD hunger is outside the supported domain")
	}
	if !core.ValidOxygen(snapshot.Oxygen) {
		return errors.New("presentation: HUD oxygen is outside the supported domain")
	}
	return nil
}

// `TargetSnapshot` is reversible local feedback derived from the validated world mirror.
// Hidden snapshots are empty so reset, UI, and disconnect paths cannot leak an old target.
type TargetSnapshot struct {
	Visible  bool
	Position core.BlockPos
	Name     string
}

// `Validate` accepts only visible blocks inside the world height with a non-empty UTF-8 name.
func (snapshot TargetSnapshot) Validate() error {
	if !snapshot.Visible {
		if snapshot != (TargetSnapshot{}) {
			return errors.New("presentation: hidden target contains payload")
		}
		return nil
	}
	if snapshot.Position.Y < core.MinY || snapshot.Position.Y >= core.MaxY {
		return errors.New("presentation: target height is outside the world")
	}
	if snapshot.Name == "" || !utf8.ValidString(snapshot.Name) {
		return errors.New("presentation: target name is invalid")
	}
	return nil
}

// `ErrorCode` identifies a stable session failure class without retaining a Go `error`.
type ErrorCode uint8

const (
	// `ErrorNone` is the safe zero value and means no terminal failure is present.
	ErrorNone ErrorCode = iota
	// `ErrorConnection` covers dial, transport, and unexpected remote-close failures.
	ErrorConnection
	// `ErrorLogin` covers explicit login rejection or invalid login completion.
	ErrorLogin
	// `ErrorProtocol` covers incompatible or malformed protocol traffic.
	ErrorProtocol
	// `ErrorOverflow` covers bounded queues that cannot accept state without data loss.
	ErrorOverflow
	// `ErrorInternal` covers failures that do not have a narrower stable classification.
	ErrorInternal
)

// `Valid` reports whether the error code belongs to the stable presentation domain.
func (code ErrorCode) Valid() bool {
	return code <= ErrorInternal
}

// `ErrorState` is the stable terminal error value exposed to presentation hosts.
// Normal client-requested shutdown uses the zero value rather than fabricating a failure.
type ErrorState struct {
	Code ErrorCode
}

// `Validate` rejects unknown classifications before publication.
func (state ErrorState) Validate() error {
	if !state.Code.Valid() {
		return errors.New("presentation: unknown error code")
	}
	return nil
}

// `Present` reports whether the state carries an error classification.
func (state ErrorState) Present() bool {
	return state.Code != ErrorNone
}

// `Text` returns fixed diagnostic text for a known code and no text for zero or unknown codes.
func (state ErrorState) Text() string {
	switch state.Code {
	case ErrorConnection:
		return "connection failed"
	case ErrorLogin:
		return "login failed"
	case ErrorProtocol:
		return "protocol error"
	case ErrorOverflow:
		return "client queue overflow"
	case ErrorInternal:
		return "internal client error"
	default:
		return ""
	}
}

func finiteFloat32(value float32) bool {
	return !float32IsNaN(value) && !float32IsInf(value)
}

func float32IsNaN(value float32) bool {
	return value != value
}

func float32IsInf(value float32) bool {
	return math.IsInf(float64(value), 0)
}
