package scraper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleNitterRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>mofusand_anime / Nitter</title>
    <item>
      <title>RT: Cute cat artwork</title>
      <link>https://nitter.net/mofusand_anime/status/1111111111111111111</link>
      <pubDate>Sat, 01 Jan 2000 00:00:00 +0000</pubDate>
      <guid>https://nitter.net/mofusand_anime/status/1111111111111111111</guid>
    </item>
    <item>
      <title>New drawing today 🐱</title>
      <link>https://nitter.net/mofusand_anime/status/2222222222222222222</link>
      <pubDate>Sun, 02 Jan 2000 12:00:00 +0000</pubDate>
      <guid>https://nitter.net/mofusand_anime/status/2222222222222222222</guid>
    </item>
  </channel>
</rss>`

func TestFetchRSS_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, sampleNitterRSS)
	}))
	defer srv.Close()

	scraper := NewTwitterScraper()
	items, displayName, err := scraper.FetchRSS(srv.URL + "/mofusand_anime/rss")
	if err != nil {
		t.Fatalf("FetchRSS() unexpected error: %v", err)
	}

	if displayName != "mofusand_anime" {
		t.Errorf("displayName = %q; want %q", displayName, "mofusand_anime")
	}

	if len(items) != 2 {
		t.Fatalf("got %d items; want 2", len(items))
	}

	if items[0].Title != "RT: Cute cat artwork" {
		t.Errorf("items[0].Title = %q; want %q", items[0].Title, "RT: Cute cat artwork")
	}
	if items[1].Title != "New drawing today 🐱" {
		t.Errorf("items[1].Title = %q; want %q", items[1].Title, "New drawing today 🐱")
	}
}

func TestFetchRSS_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	scraper := NewTwitterScraper()
	_, _, err := scraper.FetchRSS(srv.URL + "/nonexistent/rss")
	if err == nil {
		t.Error("expected error for HTTP 404, got nil")
	}
}
