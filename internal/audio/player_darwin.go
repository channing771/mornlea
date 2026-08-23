//go:build darwin

package audio

/*
#cgo LDFLAGS: -framework AudioToolbox -framework CoreFoundation
#include <stdint.h>

typedef struct audio_player audio_player;

audio_player *audio_player_create(float volume, uint32_t max_samples);
int audio_player_play(audio_player *player, const int16_t *pcm, uint32_t samples);
void audio_player_close(audio_player *player);
*/
import "C"

import (
	"log/slog"
	"sync"
	"unsafe"
)

const (
	audioPlayReady = iota
	audioPlayBusy
	audioPlayFailure
)

// Player 持有一个预分配的 Darwin 本地提示音队列。
type Player struct {
	handle      *C.audio_player
	pcm         [cueCount][]int16
	disabled    bool
	failureOnce sync.Once
	closeOnce   sync.Once
}

// NewPlayer 创建一个以 volume 为总音量的本地播放器。设备不可用或音量为零时，
// 返回的播放器会保持无声，且不会影响客户端的其他生命周期。
func NewPlayer(volume float32) *Player {
	player := &Player{}
	if !(volume > 0) {
		return player
	}
	for cue, spec := range cueSpecs {
		player.pcm[cue] = synthesize(spec)
	}
	player.handle = C.audio_player_create(C.float(volume), C.uint32_t(maxCueSamples()))
	if player.handle == nil {
		player.disabled = true
	}
	return player
}

// Play 非阻塞地提交一个 cue；队列忙、无声路径或播放失败都会丢弃本次声音。
func (player *Player) Play(cue Cue) {
	if player == nil || !cue.valid() || !player.available() {
		return
	}
	pcm := player.pcm[cue]
	if len(pcm) == 0 {
		return
	}
	status := C.audio_player_play(
		player.handle,
		(*C.int16_t)(unsafe.Pointer(unsafe.SliceData(pcm))),
		C.uint32_t(len(pcm)),
	)
	switch status {
	case audioPlayReady, audioPlayBusy:
		return
	default:
		player.failureOnce.Do(func() {
			slog.Warn("本地音频播放失败，已静音")
		})
		player.disabled = true
	}
}

// Close 释放播放器拥有的系统队列；重复调用安全。
func (player *Player) Close() {
	if player == nil {
		return
	}
	player.closeOnce.Do(func() {
		if player.handle != nil {
			C.audio_player_close(player.handle)
			player.handle = nil
		}
		player.disabled = true
	})
}

func (player *Player) available() bool {
	return player != nil && player.handle != nil && !player.disabled
}

func maxCueSamples() int {
	maximum := 0
	for _, spec := range cueSpecs {
		maximum = max(maximum, spec.samples)
	}
	return maximum
}
