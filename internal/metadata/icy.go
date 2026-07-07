package metadata

import (
	"context"
	"strings"

	"github.com/PLNech/fipindicateur/internal/stations"
)

// ICYProvider parses icecast stream titles (as observed by the player's
// media-title property) into NowPlaying. It is the fallback for stations with
// no livemeta id and whenever livemeta errors persist.
type ICYProvider struct {
	titles chan string
}

// NewICY returns an ICY provider.
func NewICY() *ICYProvider {
	return &ICYProvider{titles: make(chan string, 4)}
}

// PushTitle feeds a raw icecast title (e.g. "ARTIST - TITLE"). Non-blocking:
// if the buffer is full the title is dropped (the next one supersedes it).
func (p *ICYProvider) PushTitle(title string) {
	select {
	case p.titles <- title:
	default:
	}
}

// looksLikeStreamName reports whether a title is really the stream's filename
// (mpv sets media-title to the URL basename before real ICY tags arrive).
func looksLikeStreamName(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "id=radiofrance") ||
		strings.Contains(l, ".mp3") ||
		strings.Contains(l, ".aac") ||
		strings.Contains(l, ".m3u8") ||
		strings.Contains(l, "icecast")
}

// parseICY splits an icecast title on the first " - " into artist and title.
func parseICY(raw string) NowPlaying {
	raw = strings.TrimSpace(raw)
	if raw == "" || looksLikeStreamName(raw) {
		return NowPlaying{}
	}
	if i := strings.Index(raw, " - "); i >= 0 {
		artist := strings.TrimSpace(raw[:i])
		return NowPlaying{
			Artist:        artist,
			PrimaryArtist: CleanArtist(artist),
			Title:         strings.TrimSpace(raw[i+3:]),
		}
	}
	return NowPlaying{Title: raw}
}

// Watch implements Provider, emitting parsed titles as they arrive.
func (p *ICYProvider) Watch(ctx context.Context, _ stations.Station) <-chan NowPlaying {
	out := make(chan NowPlaying, 1)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-p.titles:
				np := parseICY(t)
				if np.Empty() {
					continue
				}
				select {
				case out <- np:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}
