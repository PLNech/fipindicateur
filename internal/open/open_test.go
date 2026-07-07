package open

import "testing"

func TestSearch(t *testing.T) {
	got := Search("Earl Hines You can depend on me")
	want := "https://duckduckgo.com/?q=Earl+Hines+You+can+depend+on+me"
	if got != want {
		t.Errorf("got %s want %s", got, want)
	}
}
