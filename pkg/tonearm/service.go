package tonearm

type Service interface {
	GetAlbum(id string) (Album, error)
	GetAlbumInfo(id string) (AlbumInfo, error)
	GetAlbumTracks(id string) (Paginator[Track], error)

	GetArtist(id string) (Artist, error)
	GetArtistInfo(id string) (ArtistInfo, error)

	GetPlaylist(id string) (Playlist, error)
	GetPlaylistInfo(id string) (PlaylistInfo, error)
	GetPlaylistTracks(id string) (Paginator[Track], error)

	GetTrack(id string) (Track, error)
	GetTrackRadio(id string) (PlaylistInfo, error)
}
