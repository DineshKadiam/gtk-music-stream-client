package player

import (
	"context"
	"time"

	"codeberg.org/dergs/tonearm/internal/settings"
	"codeberg.org/dergs/tonearm/internal/signals"
	"codeberg.org/dergs/tonearm/pkg/tidalapi"
	openapi "codeberg.org/dergs/tonearm/pkg/tidalapi/models/openapi"
	v1 "codeberg.org/dergs/tonearm/pkg/tidalapi/models/v1"
	"codeberg.org/dergs/tonearm/pkg/tonearm"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"github.com/go-gst/go-gst/gst"
	"github.com/infinytum/injector"
)

const UpdateInterval = 250 * time.Millisecond

var updateRunnerSourceHandle uint32
var didQueueGaplessPlayback bool
var disableGaplessPlayback bool

func onAboutToFinish(_ *gst.Element) {
	logger := logger.With("event", "aboutToFinish").WithGroup("aboutToFinish")
	count := 0
	for {
		if !didQueueGaplessPlayback {
			break
		}

		if count > 5 {
			logger.Warn("Previous gapless transition hasn't completed in 10 seconds. Enqueueing new track anyway.")
			break
		}

		count++
		logger.Debug("Previous gapless transition has not completed yet, sleeping...", "count", count)
		time.Sleep(1 * time.Second)
	}

	if RepeatModeChanged.CurrentValue() == RepeatModeTrack {
		return
	}

	if disableGaplessPlayback {
		return
	}

	nextTrack := getNextTrackFromQueue(true)
	if nextTrack != nil && nextTrack.IsStreamable() {
		trackManifest, err := injector.MustInject[*tidalapi.TidalAPI]().OpenAPI.V2.TrackManifests.TrackManifest(
			context.Background(),
			nextTrack.ID(),
			openapi.ManifestTypeMPEGDASH,
			openapi.ManifestFormatsForQuality(settings.Player().GetAudioQuality()),
			openapi.ManifestURISchemeHTTPS,
			openapi.ManifestUsagePlayback,
			false,
		)
		if err != nil {
			logger.Error("failed to get track manifest", "error", err)
			return
		}
		if err := enqueue(trackManifest); err != nil {
			logger.Error("enqueueing for gapless playback", "error", err)
			return
		}
		logger.Info("enqueued next song for gapless playback", "track_id", nextTrack.ID())
		didQueueGaplessPlayback = true

		// One-Shot Handler to update the track quality
		TrackChanged.OnLazy(func(t tonearm.Track) bool {
			logger.Debug("triggered one-shot handler to propagate gapless playback quality")
			PlaybackQualityChanged.Notify(func(oldValue v1.AudioQuality) v1.AudioQuality {
				return openapi.AudioQualityFromFormats(trackManifest.Data.Attributes.Formats)
			})
			return signals.Unsubscribe
		})
	}
}

func onBusMessage(msg *gst.Message) bool {
	switch msg.Type() {
	case gst.MessageError:
		err := msg.ParseError()
		logger.Error("playback failed", "code", err.Code(), "message", err.Message(), "error", err.Error(), "debug", err.DebugString())
	case gst.MessageTag:
		// The logs in this branch are all debug because of the nature of MessageTag.
		// If they weren't Debug, the console would get spammed with error messages,
		// since it is expected to have missing data. Between streams
		tagList := msg.ParseTags()
		if tagList == nil {
			logger.Debug("Error while getting codec")
			return true
		}

		codecIdentifier, ok := tagList.GetString(gst.TagAudioCodec)
		if !ok {
			logger.Debug("Error while getting codec")
			return true
		}
		codec := Codec(codecIdentifier)

		var bitRate uint32
		if codec == CodecAAC {
			bitRate, ok = tagList.GetUint32(gst.TagMaximumBitrate)

			if !ok {
				logger.Debug("Error while getting bitrate")
				return true
			}
		}

		AudioStreamQuality.Notify(func(oldValue *StreamQuality) *StreamQuality {
			var res StreamQuality
			if oldValue == nil {
				res = StreamQuality{}
			} else {
				res = *oldValue
			}

			res.Codec = codec
			res.BitRate = bitRate

			return &res
		})
	case gst.MessageStreamStart:
		startUpdateRunner()
		playbin.Set("volume", settings.Player().GetVolume())
		// A hack to trigger the correct track updates with gapless playback
		if didQueueGaplessPlayback {
			stateBeforeLoading = gst.StatePlaying
			go playNextTrack()
		}
	case gst.MessageEOS:
		playbin.SetState(gst.StatePaused)
		stopUpdateRunner()
		PlaybackStateChanged.Notify(func(oldValue *PlaybackState) *PlaybackState {
			newState := *oldValue
			newState.Status = PlaybackStatusStopped
			return &newState
		})
		go playNextTrack()
	case gst.MessageBuffering:
		percent := msg.ParseBuffering()
		if percent == 100 {
			playbin.SetState(gst.StatePlaying)
		} else {
			playbin.SetState(gst.StatePaused)
		}
	case gst.MessageStateChanged:
		_, newState := msg.ParseStateChanged()
		switch newState {
		case gst.StatePlaying:
			startUpdateRunner()
			PlaybackStateChanged.Notify(func(oldValue *PlaybackState) *PlaybackState {
				newState := *oldValue
				newState.Status = PlaybackStatusPlaying
				return &newState
			})
		}
	}
	return true
}

func onDeepElementAdded(bin, obj any) {
	if element, ok := obj.(*gst.Bin); ok {
		factory := element.GetFactory()
		if factory == nil {
			return
		}

		if factory.GetName() != "decodebin3" {
			return
		}

		element.Connect("pad-added", func(decodebin *gst.Element, pad *gst.Pad) {
			if pad.GetDirection() != gst.PadDirectionSource {
				return
			}

			pad.AddProbe(gst.PadProbeTypeEventDownstream, func(p *gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
				event := info.GetEvent()
				if event.Type() != gst.EventTypeCaps {
					return gst.PadProbeOK
				}

				caps := p.GetCurrentCaps()
				if caps == nil {
					return gst.PadProbeOK
				}

				s := caps.GetStructureAt(0)
				name := s.Name() // e.g. "audio/x-raw"
				if name != "audio/x-raw" {
					return gst.PadProbeOK
				}

				var bitDepth int
				switch format, _ := s.GetValue("format"); format {
				case "S16LE":
					bitDepth = 16
				case "S24LE", "S24_32LE":
					bitDepth = 24
				case "S32LE":
					bitDepth = 32
				}

				rate, _ := s.GetValue("rate")

				AudioStreamQuality.Notify(func(oldValue *StreamQuality) *StreamQuality {
					var res StreamQuality
					if oldValue == nil {
						res = StreamQuality{}
					} else {
						res = *oldValue
					}

					res.BitDepth = bitDepth
					if rate, ok := rate.(int); ok {
						res.SampleRate = rate
					}

					return &res
				})

				return gst.PadProbeOK
			})
		})
	}
}

func onVolumeChange() {
	if volume, err := playbin.GetProperty("volume"); err != nil {
		return
	} else if volume.(float64) == VolumeChanged.CurrentValue() {
		return
	} else {
		settings.Player().SetVolume(volume.(float64))
		VolumeChanged.Notify(func(oldValue float64) float64 {
			return volume.(float64)
		})
	}
}

var onUpdateTick = glib.SourceFunc(func(uintptr) bool {
	PlaybackStateChanged.Notify(func(oldValue *PlaybackState) *PlaybackState {
		newState := *oldValue
		if ok, duration := playbin.QueryDuration(gst.FormatTime); ok && !didQueueGaplessPlayback {
			newState.Duration = time.Duration(duration)
		}

		if ok, position := playbin.QueryPosition(gst.FormatTime); ok {
			newState.Position = time.Duration(position)
			newState.IsSeeking = false
		}

		return &newState
	})
	return glib.SOURCE_CONTINUE
})

func startUpdateRunner() {
	if updateRunnerSourceHandle != 0 {
		return
	}
	updateRunnerSourceHandle = glib.TimeoutAdd(uint32(UpdateInterval.Milliseconds()), &onUpdateTick, 0)
	logger.Debug("started playbin update runner", "source_handle", updateRunnerSourceHandle)
}

func stopUpdateRunner() {
	if updateRunnerSourceHandle == 0 {
		return
	}

	glib.SourceRemove(updateRunnerSourceHandle)
	logger.Debug("stopped playbin update runner", "source_handle", updateRunnerSourceHandle)
	updateRunnerSourceHandle = 0
}
