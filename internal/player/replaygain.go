package player

import (
	"fmt"
	"log/slog"

	"codeberg.org/dergs/tonearm/internal/g"
	"codeberg.org/dergs/tonearm/internal/settings"
	openapi "codeberg.org/dergs/tonearm/pkg/tidalapi/models/openapi"
	"codeberg.org/dergs/tonearm/pkg/tonearm"
	"github.com/go-gst/go-gst/gst"
)

var (
	rgvolume = g.Lazy(func() *gst.Element {
		rgvolume, err := gst.NewElementWithName("rgvolume", "rgvolume")
		if err != nil {
			panic(err)
		}
		rgvolume.Set("album-mode", calculateAlbumMode())
		rgvolume.Set("pre-amp", 0.0)
		rgvolume.Set("fallback-gain", 0.0)

		sinkPad := rgvolume.GetStaticPad("sink")
		sinkPad.AddProbe(gst.PadProbeTypeEventDownstream, func(pad *gst.Pad, info *gst.PadProbeInfo) (ret gst.PadProbeReturn) {
			ret = gst.PadProbeOK
			event := info.GetEvent()
			if event == nil {
				return
			}

			if event.Type() != gst.EventTypeSegment {
				return
			}

			if currentlyEnqueuedTrack == nil {
				return
			}

			if !settings.Playback().NormalizeVolume() {
				return
			}

			injectReplayGainTags(rgvolume, currentlyEnqueuedTrack)
			return
		})
		return rgvolume
	})
)

func buildEmptyBin() (*gst.Bin, error) {
	emptyBin := gst.NewBin("emptybin")
	identity, err := gst.NewElement("identity")
	if err != nil {
		return nil, err
	}
	emptyBin.Add(identity)

	ghostSink := gst.NewGhostPad("sink", identity.GetStaticPad("sink"))
	emptyBin.AddPad(ghostSink.Pad)

	ghostSrc := gst.NewGhostPad("src", identity.GetStaticPad("src"))
	emptyBin.AddPad(ghostSrc.Pad)
	return emptyBin, nil
}

func buildReplayGainFilterBin() (*gst.Bin, error) {
	bin := gst.NewBin("replaygain-bin")

	rglimiter, err := gst.NewElementWithName("rglimiter", "rglimiter")
	if err != nil {
		return nil, err
	}

	bin.Add(rgvolume())
	bin.Add(rglimiter)

	rgvolume().Link(rglimiter)

	sinkPad := rgvolume().GetStaticPad("sink")
	ghostSink := gst.NewGhostPad("sink", sinkPad)
	bin.AddPad(ghostSink.Pad)

	srcPad := rglimiter.GetStaticPad("src")
	ghostSrc := gst.NewGhostPad("src", srcPad)
	bin.AddPad(ghostSrc.Pad)

	return bin, nil
}

func injectReplayGainTags(rgvolume *gst.Element, manifest *openapi.TrackManifest) {
	tagList := gst.NewEmptyTagList()

	log := slog.With()
	if data := manifest.Data.Attributes.AlbumAudioNormalizationData; data != nil {
		tagList.AddValue(gst.TagMergeAppend, gst.TagAlbumPeak, float64(data.PeakAmplitude))
		log = log.With("albumPeak", fmt.Sprintf("%.6f", data.PeakAmplitude))

		tagList.AddValue(gst.TagMergeAppend, gst.TagAlbumGain, float64(data.ReplayGain))
		log = log.With("albumGain", fmt.Sprintf("%.2f dB", data.ReplayGain))
	}

	if data := manifest.Data.Attributes.TrackAudioNormalizationData; data != nil {
		tagList.AddValue(gst.TagMergeAppend, gst.TagTrackGain, float64(data.ReplayGain))
		log = log.With("trackGain", fmt.Sprintf("%.2f dB", data.ReplayGain))

		tagList.AddValue(gst.TagMergeAppend, gst.TagTrackPeak, float64(data.PeakAmplitude))
		log = log.With("trackPeak", fmt.Sprintf("%.6f", data.PeakAmplitude))
	}

	tagEvent := gst.NewTagEvent(tagList)
	albumMode := calculateAlbumMode()
	rgvolume.Set("album-mode", albumMode)
	if ok := rgvolume.GetStaticPad("sink").SendEvent(tagEvent); !ok {
		log.Warn("failed to send ReplayGain tag event")
	} else {
		log.Info("injected ReplayGain tags", "albumMode", albumMode)
	}
}

func calculateAlbumMode() bool {
	mode := settings.Playback().ReplayGainMode()

	source := SourceChanged.CurrentValue()
	if source == nil {
		return mode == settings.ReplayGainModeAlbum
	}

	if source.SourceType() == tonearm.SourceTypeAlbum {
		return mode != settings.ReplayGainModeTrack
	} else {
		return mode == settings.ReplayGainModeAlbum
	}
}
