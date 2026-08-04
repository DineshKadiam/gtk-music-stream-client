package tonearm

import "time"

type TrackQuality string

const (
	TrackQualityMax    TrackQuality = "max"
	TrackQualityHigh   TrackQuality = "high"
	TrackQualityMedium TrackQuality = "medium"
	TrackQualityLow    TrackQuality = "low"
)

type TrackFlag int

func (f TrackFlag) IsAI() bool {
	return f&TrackFlagAI != 0
}

func (f TrackFlag) IsExplicit() bool {
	return f&TrackFlagExplicit != 0
}

const TrackFlagNone TrackFlag = 0

const (
	TrackFlagExplicit TrackFlag = 1 << iota
	TrackFlagAI
)

type TrackInfo interface {
	Shareable

	// Duration returns the total duration of the track
	Duration() time.Duration

	// Flags returns the flags associated with the track
	Flags() TrackFlag

	// ID returns the unique identifier for the track
	ID() string

	// IsStreamable returns whether the track is streamable
	IsStreamable() bool

	// Title returns the title of the track without any additional information
	Title() string

	// Version returns the version of the track (e.g. "Acoustic")
	Version() string
}

type Track interface {
	TrackInfo
	PlaybackSource

	// Artists returns the artists associated with the track
	Artists() ArtistInfos

	// Album returns the album associated with the track
	Album() AlbumInfo
}

func FormatTitle(track TrackInfo) string {
	suffix := ""
	if track.Version() != "" {
		suffix = " (" + track.Version() + ")"
	}
	if track.Flags().IsAI() {
		suffix = " \U0001F170"
	}

	if track.Flags().IsExplicit() {
		suffix += " \U0001F174"
	}
	return track.Title() + suffix
}
