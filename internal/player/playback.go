package player

import (
	"context"
	"errors"

	"codeberg.org/dergs/tonearm/internal/gettext"
	"codeberg.org/dergs/tonearm/internal/notifications"
	"codeberg.org/dergs/tonearm/internal/settings"
	"codeberg.org/dergs/tonearm/pkg/tidalapi"
	openapi "codeberg.org/dergs/tonearm/pkg/tidalapi/models/openapi"
	v1 "codeberg.org/dergs/tonearm/pkg/tidalapi/models/v1"
	"codeberg.org/dergs/tonearm/pkg/tonearm"
	"github.com/go-gst/go-gst/gst"
	"github.com/infinytum/injector"
)

func playTrack(track tonearm.Track) error {
	tidal, err := injector.Inject[*tidalapi.TidalAPI]()
	if err != nil {
		return err
	}

	TrackChanged.Notify(func(oldState tonearm.Track) tonearm.Track {
		return track
	})

	PlaybackStateChanged.Notify(func(oldValue *PlaybackState) *PlaybackState {
		newState := *oldValue
		newState.Duration = track.Duration()
		return &newState
	})

	if didQueueGaplessPlayback {
		// In case the user switched track during a gapless transition, we want to make sure the player does not get stuck in a gapless playback loop
		didQueueGaplessPlayback = false
		logger.Debug("gapless playback detected, not enqueueing track again")
		resetLoadingState()
		return nil
	}

	if !track.IsStreamable() {
		notifications.OnToast.Notify(gettext.Get("Track not available for streaming, skipping to next track"))
		Next()
		return errors.New("track not available for streaming")
	}

	logger.Debug("fetching track manifest", "track_id", track.ID())
	trackManifest, err := tidal.OpenAPI.V2.TrackManifests.TrackManifest(
		context.Background(),
		track.ID(),
		openapi.ManifestTypeMPEGDASH,
		openapi.ManifestFormatsForQuality(settings.Player().GetAudioQuality()),
		openapi.ManifestURISchemeHTTPS,
		openapi.ManifestUsagePlayback,
		false,
	)
	if err != nil {
		logger.Error("unable to fetch track manifest", "error", err)
		return err
	}
	return play(trackManifest)
}

func play(manifest *openapi.TrackManifest) error {
	PlaybackQualityChanged.Notify(func(oldValue v1.AudioQuality) v1.AudioQuality {
		return openapi.AudioQualityFromFormats(manifest.Data.Attributes.Formats)
	})

	// Free up resources taken up by previous stream
	playbin.SetState(gst.StateNull)
	playbin.SetArg("uri", "")

	PlaybackStateChanged.Notify(func(oldValue *PlaybackState) *PlaybackState {
		newState := *oldValue
		newState.Loading = true
		return &newState
	})

	if err := enqueue(manifest); err != nil {
		return err
	}

	PlaybackStateChanged.Notify(func(oldValue *PlaybackState) *PlaybackState {
		newState := *oldValue
		newState.Status = PlaybackStatusPlaying
		newState.Loading = false
		return &newState
	})

	playbin.SetState(gst.StatePlaying)
	return nil
}
