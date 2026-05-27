package player

import (
	"time"

	"codeberg.org/dergs/tonearm/internal/player/queue"
	"codeberg.org/dergs/tonearm/internal/settings"
	"codeberg.org/dergs/tonearm/internal/signals"
	"github.com/go-gst/go-gst/gst"
)

var (
	BaseQueue = queue.NewDurableQueue()
	UserQueue = queue.NewQueue()
)

func init() {
	ShuffleStateChanged.On(func(enabled bool) bool {
		currentTrackId := ""
		if TrackChanged.CurrentValue() != nil {
			currentTrackId = TrackChanged.CurrentValue().ID()
		}
		if enabled {
			BaseQueue.Shuffle(currentTrackId)
		} else {
			BaseQueue.Restore(currentTrackId)
		}
		return signals.Continue
	})
}

func playNextTrack() {
	if RepeatModeChanged.CurrentValue() == RepeatModeTrack {
		logger.Debug("single repeat mode is enabled, replaying track")
		playbin.SeekTime(0, gst.SeekFlagFlush|gst.SeekFlagKeyUnit)
		startUpdateRunner()
		resetLoadingState()
		return
	}

	nextTrack := getNextTrackFromQueue(false)
	if nextTrack != nil {
		logger.Info("playing next track", "track_id", nextTrack.ID())
		if currentlyEnqueuedTrack == nil || currentlyEnqueuedTrack.Data.ID != nextTrack.ID() {
			setLoadingState()
		}
		playTrack(nextTrack)

		history.Push(&HistoryEntry{
			TrackID: nextTrack.ID(),
		})

		return
	}

	if settings.Playback().AllowAutoplay() && TrackChanged.CurrentValue() != nil {
		// Since no other songs are left in the queue, retrieve mix to play from API
		logger.Info("starting track radio", "track_id", TrackChanged.CurrentValue().ID())
		PlayTrackRadio(TrackChanged.CurrentValue().ID(), true)
	} else {
		resetLoadingState()
	}
}

func playPreviousTrack() {
	ok, position := playbin.QueryPosition(gst.FormatTime)
	if ok && time.Duration(position) > 5*time.Second {
		logger.Debug("above the 5 second mark, replaying song", "action", "previous")
		SeekToPosition(0, false)
		return
	}

	if len(history.Entries.CurrentValue()) < 1 {
		logger.Debug("no history entries, replaying song", "action", "previous")
		SeekToPosition(0, false)
		return
	}

	setLoadingState()

	entry := history.Pop()
	if entry != nil {
		track, err := resolveTrack(TrackChanged.CurrentValue().ID())
		if err != nil {
			logger.Error("failed to resolve track", "trackID", entry.TrackID, "error", err)
			return
		}

		// Re-Queue current song to front of user-queue
		UserQueue.Prepend(track)

		// Switch to previous track without clearing base queue
		track, err = resolveTrack(entry.TrackID)
		if err != nil {
			logger.Error("failed to resolve track", "trackID", entry.TrackID, "error", err)
			return
		}
		logger.Debug("playing previous track", "track_id", track.ID(), "action", "previous")
		playTrack(track)
	}
}

func SkipThroughQueue(queue queue.Queue, to int) {
	go func() {
		setLoadingState()
		if skipped, err := queue.Skip(to); err == nil {
			for _, track := range skipped {
				history.Push(&HistoryEntry{track.ID()})
			}

			playTrack(queue.Pop())
		} else {
			resetLoadingState()
			logger.Error("failed to skip through queue", "error", err)
		}
	}()
}
