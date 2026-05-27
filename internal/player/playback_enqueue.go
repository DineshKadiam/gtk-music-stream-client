package player

import (
	"codeberg.org/dergs/tonearm/internal/settings"
	openapi "codeberg.org/dergs/tonearm/pkg/tidalapi/models/openapi"
)

var (
	currentlyEnqueuedTrack *openapi.TrackManifest
)

func enqueue(manifest *openapi.TrackManifest) error {
	playbin.SetArg("uri", manifest.Data.Attributes.URI)
	playbin.Set("volume", settings.Player().GetVolume())
	currentlyEnqueuedTrack = manifest
	return nil
}
