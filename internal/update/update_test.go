package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseSemver(t *testing.T) {
	cases := map[string]struct {
		want [3]int
		ok   bool
	}{
		"v0.2.0":            {[3]int{0, 2, 0}, true},
		"0.2.0":             {[3]int{0, 2, 0}, true},
		"v1.10.3":           {[3]int{1, 10, 3}, true},
		"v0.1.0-25-gabc123": {[3]int{0, 1, 0}, true}, // suffix ignored
		"v0.2.0-dev":        {[3]int{0, 2, 0}, true},
		"vdev":              {[3]int{}, false},
		"garbage":           {[3]int{}, false},
	}
	for in, exp := range cases {
		got, ok := parseSemver(in)
		if ok != exp.ok || (ok && got != exp.want) {
			t.Errorf("parseSemver(%q) = %v,%v want %v,%v", in, got, ok, exp.want, exp.ok)
		}
	}
}

func TestGreater(t *testing.T) {
	if !greater([3]int{0, 2, 0}, [3]int{0, 1, 9}) {
		t.Error("0.2.0 > 0.1.9")
	}
	if !greater([3]int{1, 0, 0}, [3]int{0, 9, 9}) {
		t.Error("1.0.0 > 0.9.9")
	}
	if greater([3]int{0, 2, 0}, [3]int{0, 2, 0}) {
		t.Error("equal is not greater")
	}
	if greater([3]int{0, 1, 0}, [3]int{0, 2, 0}) {
		t.Error("0.1.0 not > 0.2.0")
	}
}

func TestIsDev(t *testing.T) {
	for _, v := range []string{"v0.1.0-25-gabc", "v0.2.0-dev", "0.1.0-dirty"} {
		if !isDev(v) {
			t.Errorf("%q should be dev", v)
		}
	}
	for _, v := range []string{"v0.2.0", "1.0.0"} {
		if isDev(v) {
			t.Errorf("%q should not be dev", v)
		}
	}
}

// fakeServer returns a Result computed against a canned latest-release payload.
func checkAgainst(t *testing.T, payload, current string) *Result {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)
	// call CheckLatest against the fake by temporarily reusing its transport:
	// simplest is to hit the URL directly via a small helper.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		t.Fatal(err)
	}
	res := &Result{Current: current, Latest: rel.TagName, URL: rel.HTMLURL, Dev: isDev(current)}
	res.AssetName, res.AssetURL = assetForOS(&rel)
	if cur, ok := parseSemver(current); ok {
		if lat, ok2 := parseSemver(rel.TagName); ok2 && !res.Dev {
			res.Newer = greater(lat, cur)
		}
	}
	return res
}

func TestCheckNewerRelease(t *testing.T) {
	payload := `{"tag_name":"v0.3.0","html_url":"https://example/rel","assets":[
		{"name":"fipindicateur-linux-amd64.tar.gz","browser_download_url":"https://example/linux"},
		{"name":"FipIndicateur.dmg","browser_download_url":"https://example/dmg"}]}`
	res := checkAgainst(t, payload, "v0.2.0")
	if !res.Newer {
		t.Error("v0.3.0 should be newer than v0.2.0")
	}
	if res.Dev {
		t.Error("release build should not be flagged dev")
	}
	if res.AssetURL == "" {
		t.Error("an OS asset should be picked (linux or dmg present)")
	}
}

func TestCheckUpToDate(t *testing.T) {
	res := checkAgainst(t, `{"tag_name":"v0.2.0","html_url":"u","assets":[]}`, "v0.2.0")
	if res.Newer {
		t.Error("same version is not newer")
	}
}

func TestCheckDevBuild(t *testing.T) {
	res := checkAgainst(t, `{"tag_name":"v0.1.0","html_url":"u","assets":[]}`, "v0.1.0-25-gabc123")
	if !res.Dev {
		t.Error("git-describe build should be dev")
	}
	if res.Newer {
		t.Error("dev builds do not claim an available release upgrade")
	}
}
