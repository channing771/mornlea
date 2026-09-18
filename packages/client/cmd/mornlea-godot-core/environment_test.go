//go:build cgo

package main

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
)

func TestEnvironmentProjectionReusesConfirmedRenderSemantics(t *testing.T) {
	for _, weather := range []core.WeatherKind{core.WeatherClear, core.WeatherRain, core.WeatherThunder} {
		for _, season := range []core.Season{core.SeasonSpring, core.SeasonWinter} {
			for _, ticks := range []uint64{0, 6000, 18000, 24001} {
				frame := presentation.FrameSnapshot{Revision: 7, Epoch: 3, Environment: presentation.EnvironmentSnapshot{
					Ready: true, ServerTick: 999, WorldTimeTicks: ticks, DayPhaseOffset: 13, Weather: weather, Season: season, SeasonProgress: 128,
				}}
				record := encodeEnvironmentProjection(frame)
				if len(record) != 48 || binary.LittleEndian.Uint32(record[8:]) != 1 || binary.LittleEndian.Uint64(record[16:]) != 7 || binary.LittleEndian.Uint64(record[24:]) != 3 {
					t.Fatalf("invalid projection identity: %x", record)
				}
				day := render.DayNightAt(ticks, 13, (float64(season)+0.5)/4)
				color := render.WeatherSkyColor(day.ClearColor, weather)
				want := [4]float32{render.ApplyWeatherDaylight(day.Daylight, weather, 999), color[0], color[1], color[2]}
				for i, value := range want {
					if got := math.Float32frombits(binary.LittleEndian.Uint32(record[32+i*4:])); got != value {
						t.Fatalf("projection %d = %v, want %v", i, got, value)
					}
				}
			}
		}
	}
	if got := encodeEnvironmentProjection(presentation.FrameSnapshot{}); len(got) != 48 || binary.LittleEndian.Uint32(got[8:]) != 0 || binary.LittleEndian.Uint64(got[32:]) != 0 {
		t.Fatalf("unconfirmed projection fabricated values: %x", got)
	}
}
