// Package open launches URLs in the user's default browser, portably.
package open

import (
	"net/url"
)

// URL opens the given URL in the default browser. Errors are ignored:
// opening a link is best-effort and must never crash the app.
func URL(u string) {
	if u == "" {
		return
	}
	cmd := openCmd(u)
	if cmd == nil {
		return
	}
	_ = cmd.Start()
}

// Search builds a DuckDuckGo search URL for the given query terms.
func Search(query string) string {
	return "https://duckduckgo.com/?q=" + url.QueryEscape(query)
}

// WikipediaFr builds a French Wikipedia search URL. FIP is a French station,
// so fr.wp is the right default; its coverage of the world/jazz catalogue is
// solid too.
func WikipediaFr(query string) string {
	return "https://fr.wikipedia.org/w/index.php?search=" + url.QueryEscape(query)
}
