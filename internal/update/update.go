// Package update checks GitHub Releases for a newer version. It only reads the
// public releases API and never downloads or replaces anything: the check is
// on-demand (or opt-in at startup), notifies the result, and opens the release
// page so the user decides. This keeps the local-first, no-surprise posture.
package update

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// LatestURL is the GitHub Releases API endpoint for the project's latest
// published release.
const LatestURL = "https://api.github.com/repos/PLNech/fipindicateur/releases/latest"

// Asset is one downloadable artifact attached to a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Release is the subset of the GitHub release payload we use.
type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Result summarises a check for the UI.
type Result struct {
	Current   string // running version, e.g. "v0.2.0" or "v0.1.0-25-g<hash>"
	Latest    string // latest release tag, e.g. "v0.2.0"
	URL       string // release page
	Newer     bool   // a strictly newer release exists (for non-dev builds)
	Dev       bool   // running a dev build (git-describe suffix): update via source
	AssetName string // best asset for this OS, if any
	AssetURL  string
}

// CheckLatest fetches the latest release from GitHub with a bounded timeout.
func CheckLatest(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, LatestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "fipindicateur-update-check")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r Release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Check compares the running version against the latest release. current is the
// value from internal/version (via version.String()).
func Check(ctx context.Context, current string) (*Result, error) {
	r, err := CheckLatest(ctx)
	if err != nil {
		return nil, err
	}
	res := &Result{Current: current, Latest: r.TagName, URL: r.HTMLURL, Dev: isDev(current)}
	res.AssetName, res.AssetURL = assetForOS(r)

	cur, curOK := parseSemver(current)
	lat, latOK := parseSemver(r.TagName)
	if curOK && latOK && !res.Dev {
		res.Newer = greater(lat, cur)
	}
	return res, nil
}

// isDev reports whether the version is a development build. Release tags are
// clean (vX.Y.Z); git-describe dev builds carry a commit or dirty suffix, and
// the un-stamped default is "v0.2.0-dev": all contain a hyphen after X.Y.Z.
func isDev(v string) bool {
	base := strings.TrimPrefix(v, "v")
	return strings.Contains(base, "-")
}

// parseSemver extracts [major, minor, patch] from a leading vX.Y.Z, ignoring
// any suffix. Returns ok=false if the three numeric fields are not present.
func parseSemver(v string) ([3]int, bool) {
	s := strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// greater reports whether a is a strictly higher version than b.
func greater(a, b [3]int) bool {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

// assetForOS picks the release asset matching the running OS: the Linux tarball
// or the macOS disk image. Returns empty strings if none matches.
func assetForOS(r *Release) (name, url string) {
	want := osToken()
	for _, a := range r.Assets {
		if strings.Contains(strings.ToLower(a.Name), want) {
			return a.Name, a.URL
		}
	}
	return "", ""
}

func osToken() string {
	switch runtime.GOOS {
	case "darwin":
		return "dmg"
	default:
		return "linux"
	}
}

// SinceThrottle reports whether at least d has elapsed since last (or last is
// zero). Used to avoid re-checking too often on repeated startups.
func SinceThrottle(last time.Time, d time.Duration) bool {
	return last.IsZero() || time.Since(last) >= d
}
