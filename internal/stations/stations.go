// Package stations lists the FIP webradios and builds their stream URLs.
package stations

import "fmt"

// Quality selects the icecast stream bitrate/codec.
type Quality struct {
	Name string // "midfi" or "hifi"
	Ext  string // "mp3" or "aac"
}

var (
	// Midfi is the default 128k MP3 stream.
	Midfi = Quality{Name: "midfi", Ext: "mp3"}
	// Hifi is the opt-in 192k AAC stream.
	Hifi = Quality{Name: "hifi", Ext: "aac"}
)

// Station is a single FIP webradio.
type Station struct {
	Key     string // stable internal key, persisted in config
	Slug    string // icecast slug, e.g. "fiprock"
	Display string // French display name with accents
	MetaID  int    // livemeta pull id, 0 if unknown (ICY fallback only)
}

// StreamURL returns the icecast stream URL for the given quality.
func (s Station) StreamURL(q Quality) string {
	return fmt.Sprintf("https://icecast.radiofrance.fr/%s-%s.%s?id=radiofrance", s.Slug, q.Name, q.Ext)
}

// All is the ordered list of 13 FIP webradios.
// MetaID sources: fip=7, rock=64, jazz=65, groove=66, world=69,
// nouveautes=70, reggae=71, electro=74, metal=77. The four with id 0
// (hiphop, sacrefrancais, pop, cultes) have no known livemeta id and
// rely on the ICY metadata fallback.
var All = []Station{
	{Key: "fip", Slug: "fip", Display: "FIP", MetaID: 7},
	{Key: "rock", Slug: "fiprock", Display: "Rock", MetaID: 64},
	{Key: "jazz", Slug: "fipjazz", Display: "Jazz", MetaID: 65},
	{Key: "groove", Slug: "fipgroove", Display: "Groove", MetaID: 66},
	{Key: "world", Slug: "fipworld", Display: "Monde", MetaID: 69},
	{Key: "nouveautes", Slug: "fipnouveautes", Display: "Nouveautés", MetaID: 70},
	{Key: "reggae", Slug: "fipreggae", Display: "Reggae", MetaID: 71},
	{Key: "electro", Slug: "fipelectro", Display: "Electro", MetaID: 74},
	{Key: "hiphop", Slug: "fiphiphop", Display: "Hip-Hop", MetaID: 0},
	{Key: "metal", Slug: "fipmetal", Display: "Metal", MetaID: 77},
	{Key: "sacrefrancais", Slug: "fipsacrefrancais", Display: "Sacré français !", MetaID: 0},
	{Key: "pop", Slug: "fippop", Display: "Pop", MetaID: 0},
	{Key: "cultes", Slug: "fipcultes", Display: "Cultes", MetaID: 0},
}

// ByKey returns the station with the given key, or the default (fip) if not found.
func ByKey(key string) Station {
	for _, s := range All {
		if s.Key == key {
			return s
		}
	}
	return All[0]
}
