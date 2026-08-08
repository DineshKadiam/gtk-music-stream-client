package player

import (
	"math"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
)

type ControllableState struct {
	HasTrack    bool
	PlayerReady bool
}

func (cs *ControllableState) CanControl() bool {
	return cs.HasTrack && cs.PlayerReady
}

func CycleRepeatMode() {
	switch RepeatModeChanged.CurrentValue() {
	case RepeatModeNone:
		SetRepeatMode(RepeatModeQueue)
	case RepeatModeQueue:
		SetRepeatMode(RepeatModeTrack)
	case RepeatModeTrack:
		SetRepeatMode(RepeatModeNone)
	}
}

func SetShuffle(enabled bool) {
	ShuffleStateChanged.Notify(func(oldValue bool) bool {
		return enabled
	})
}

func Next() {
	logger.Debug("player controls requested to play next track")
	disableGaplessPlayback = true
	playNextTrack()
	disableGaplessPlayback = false
}

func Pause() {
	logger.Debug("player controls requested to pause")
	playbin.SetState(gst.StatePaused)
	PlaybackStateChanged.Notify(func(oldValue *PlaybackState) *PlaybackState {
		newState := *oldValue
		newState.Status = PlaybackStatusPaused
		return &newState
	})
}

func Play() {
	logger.Debug("player controls requested to start playing")
	playbin.SetState(gst.StatePlaying)
}

func PlayPause() {
	logger.Debug("player controls requested to start playing or pause")
	switch PlaybackStateChanged.CurrentValue().Status {
	case PlaybackStatusPlaying:
		Pause()
	case PlaybackStatusPaused:
		Play()
	case PlaybackStatusStopped:
		seekToPosition(0)
		Play()
	}
}

func Previous() {
	logger.Debug("player controls requested to play previous track")
	disableGaplessPlayback = true
	playPreviousTrack()
	disableGaplessPlayback = false
}

func SeekToPercent(percent float64) {
	if percent < 0 || percent > 100 {
		percent = math.Max(0, math.Min(100, percent))
		logger.Warn("percent must be between 0 and 100, clamping to nearest value", "percent", percent)
	}

	position := float64(PlaybackStateChanged.CurrentValue().Duration) * (percent / 100.0)
	SeekToPosition(time.Duration(int64(position)), false)
}

var (
	seekMutex    sync.Mutex
	seekDebounce *time.Timer
)

func seekToPosition(position time.Duration) {
	playbin.SeekTime(position, gst.SeekFlagFlush|gst.SeekFlagKeyUnit)
}

func seekToPositionPrecise(position time.Duration) {
	playbin.SeekTime(position, gst.SeekFlagFlush|gst.SeekFlagAccurate)
}

func SeekToPosition(position time.Duration, precise bool) {
	logger.Debug("player controls requested to seek to position", "position", position)
	go func() {
		seekMutex.Lock()
		defer seekMutex.Unlock()

		if seekDebounce != nil {
			seekDebounce.Stop()
		} else {
			playbin.SetState(gst.StatePaused)
			if updateRunnerSourceHandle != 0 {
				stopUpdateRunner()
			}
		}

		seekDebounce = time.AfterFunc(500*time.Millisecond, func() {
			seekMutex.Lock()
			defer seekMutex.Unlock()

			PlaybackStateChanged.Notify(func(oldValue *PlaybackState) *PlaybackState {
				newState := *oldValue
				newState.Position = position
				newState.IsSeeking = true
				return &newState
			})

			if precise {
				seekToPositionPrecise(position)
			} else {
				seekToPosition(position)
			}
			if PlaybackStateChanged.CurrentValue().Status == PlaybackStatusPlaying {
				playbin.SetState(gst.StatePlaying)
				startUpdateRunner()
			}

			seekDebounce = nil
		})
	}()
}

func SeekToPositionRelative(delta time.Duration) {
	SeekToPosition(PlaybackStateChanged.CurrentValue().Position+delta, false)
}

func SetRepeatMode(m RepeatMode) {
	if m == RepeatModeChanged.CurrentValue() {
		return
	}

	RepeatModeChanged.Notify(func(oldValue RepeatMode) RepeatMode {
		return m
	})
}

func SetVolume(volume float64) {
	if volume < 0 {
		logger.Info("Volume is lower than 0, overriding back to 0.")
		volume = 0
	} else if volume > 1 {
		logger.Warn("Volume is higher than 1. This will cause overdrive to the speakers.")
	}
	if playbin != nil {
		playbin.SetProperty("volume", volume)
	}
}

func Stop() {
	logger.Debug("player controls requested to stop")
	playbin.SetState(gst.StateNull)
}

func ToggleShuffle() {
	ShuffleStateChanged.Notify(func(oldValue bool) bool {
		return !oldValue
	})
}
