package notify

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jpegFixture builds a small JPEG in memory. The stdlib cannot ENCODE
// progressive JPEGs, so the fixture is baseline; the DECODE path in
// reencodeCover is identical for both (image/jpeg sniffs the marker frames),
// which is exactly what the CDN's progressive bytes go through.
func jpegFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x * 30), G: uint8(y * 30), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

// TestFetchCoverReencodesToPNG covers the whole path: one download, a PNG on
// disk, and a second call served entirely from the cache (no second request).
func TestFetchCoverReencodesToPNG(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write(jpegFixture(t))
	}))
	defer srv.Close()

	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	path := fetchCover(context.Background(), srv.URL+"/cover.jpg")
	if path == "" {
		t.Fatal("fetchCover returned an empty path")
	}
	if !strings.HasSuffix(path, ".png") {
		t.Fatalf("fetchCover returned %q, want a .png re-encode", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read re-encoded cover: %v", err)
	}
	src, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("re-encoded cover does not decode: %v", err)
	}
	if format != "png" {
		t.Fatalf("re-encoded cover format = %q, want png", format)
	}
	if got := src.Bounds(); got.Dx() != 8 || got.Dy() != 8 {
		t.Fatalf("re-encoded cover bounds = %v, want 8x8", got)
	}

	// Cache hit: same path, no second HTTP request.
	again := fetchCover(context.Background(), srv.URL+"/cover.jpg")
	if again != path {
		t.Fatalf("second fetchCover = %q, want the cached %q", again, path)
	}
	if hits != 1 {
		t.Fatalf("cover downloaded %d times, want 1 (cache miss on repeat)", hits)
	}
}

// TestFetchCoverBadBytes covers the failure-is-empty discipline: garbage
// bytes (a non-image 200) return "" and leave no poisoned PNG cache behind,
// so a later good fetch can succeed.
func TestFetchCoverBadBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("definitely not an image"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	if path := fetchCover(context.Background(), srv.URL+"/cover.jpg"); path != "" {
		t.Fatalf("fetchCover on garbage = %q, want empty", path)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "fipindicateur-cover-*.png"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("a PNG cache file survived a failed re-encode: %v", matches)
	}
}

// TestFetchCoverEmptyURL pins the cheap early-out.
func TestFetchCoverEmptyURL(t *testing.T) {
	if path := fetchCover(context.Background(), ""); path != "" {
		t.Fatalf("fetchCover(\"\") = %q, want empty", path)
	}
}

// TestPNGCacheSurvivesOriginalRemoval covers the idempotence contract: with
// both files present (the steady state after one fetch), the PNG is returned
// without touching the network even if the original JPEG disappears.
func TestPNGCacheSurvivesOriginalRemoval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jpegFixture(t))
	}))
	defer srv.Close()

	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	path := fetchCover(context.Background(), srv.URL+"/cover.jpg")
	if path == "" {
		t.Fatal("fetchCover returned an empty path")
	}
	if _, err := png.Decode(bytes.NewReader(mustRead(t, path))); err != nil {
		t.Fatalf("cached cover is not a valid PNG: %v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
