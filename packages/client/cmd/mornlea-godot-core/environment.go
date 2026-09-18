//go:build cgo

package main

import (
	"encoding/binary"

	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/client/render"
)

// `encodeEnvironmentProjection` reuses the existing Go presentation kernels.
// A separate family preserves frame-v1 compatibility and prevents Rust/Python
// from duplicating seasonal warp or weather algorithms. Identity ties both
// pulls to one retained frame; consumers reject a mismatched pair atomically.
func encodeEnvironmentProjection(frame presentation.FrameSnapshot) []byte {
	if frame.Environment.Validate() != nil {
		return nil
	}
	record := make([]byte, EnvironmentBytes)
	binary.LittleEndian.PutUint32(record, MagicEnvironment)
	binary.LittleEndian.PutUint32(record[4:], EnvironmentVersion)
	binary.LittleEndian.PutUint64(record[16:], frame.Revision)
	binary.LittleEndian.PutUint64(record[24:], frame.Epoch)
	input := frame.Environment
	if !input.Ready {
		return record
	}
	binary.LittleEndian.PutUint32(record[8:], 1)
	// Reconstruct only the confirmed quantized season, never local-clock time.
	yearPhase := (float64(input.Season) + float64(input.SeasonProgress)/256) / 4
	day := render.DayNightAt(input.WorldTimeTicks, input.DayPhaseOffset, yearPhase)
	color := render.WeatherSkyColor(day.ClearColor, input.Weather)
	framePutFloat(record[32:], render.ApplyWeatherDaylight(day.Daylight, input.Weather, input.ServerTick))
	for i := range 3 {
		framePutFloat(record[36+i*4:], color[i])
	}
	return record
}
