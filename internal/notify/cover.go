package notify

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	// registered for image.Decode (progressive JPEGs included)
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// fetchCover downloads a cover image and returns its path, or "" on any
// error. The Linux notifier and the macOS notifier share this so the banner
// can carry the album art. Cached by URL hash next to the runtime dir.
//
// The cached file is RE-ENCODED to baseline PNG before being returned: Radio
// France serves progressive JPEGs, which the stdlib decodes fine but
// UNNotificationAttachment refuses (the attachment then fails silently and
// the banner degrades to title and body). Decoding once and caching the PNG
// makes every later banner a disk stat, and the same bytes also serve the
// Linux app_icon path.
func fetchCover(ctx context.Context, url string) string {
	if url == "" {
		return ""
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	sum := sha1.Sum([]byte(url))
	base := filepath.Join(dir, "fipindicateur-cover-"+hex.EncodeToString(sum[:8]))
	raw, png := base+".jpg", base+".png"
	if _, err := os.Stat(png); err == nil {
		return png // the re-encode is cached: no decode, no network
	}

	if _, err := os.Stat(raw); err != nil {
		if err := downloadCover(ctx, url, raw); err != nil {
			return ""
		}
	}
	path, ok := reencodeCover(raw, png)
	if !ok {
		// Undecodable or truncated bytes: drop the original so a later call
		// downloads fresh instead of returning "" forever off the bad cache.
		_ = os.Remove(raw)
		return ""
	}
	return path
}

// downloadCover fetches url into dst, capped at 8MB (a cover is a cover).
func downloadCover(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cover: HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 8<<20)); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

// reencodeCover decodes the fetched bytes (image/jpeg handles progressive
// JPEGs; UNNotificationAttachment does not) and writes them back as a PNG at
// pngPath. ok=false on any decode, encode or write failure.
func reencodeCover(rawPath, pngPath string) (path string, ok bool) {
	data, err := os.ReadFile(rawPath)
	if err != nil {
		return "", false
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", false
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		return "", false
	}
	if err := os.WriteFile(pngPath, buf.Bytes(), 0o644); err != nil {
		return "", false
	}
	return pngPath, true
}
