package scraper

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NitterRSS is the top-level XML structure for a Nitter RSS feed.
type NitterRSS struct {
	XMLName xml.Name      `xml:"rss"`
	Channel NitterChannel `xml:"channel"`
}

type NitterChannel struct {
	Title string       `xml:"title"`
	Items []NitterItem `xml:"item"`
}

type NitterItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	PubDate string `xml:"pubDate"`
	GUID    string `xml:"guid"`
}

type twitterUserAgentTransport struct {
	Transport http.RoundTripper
}

func (t *twitterUserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())

	botID := "unknown"
	if BotIDSupplier != nil {
		if id := BotIDSupplier(); id != "" {
			botID = id
		}
	}

	// Browser agent layout but retains mandatory Contourgo UA info specifically for Nitter feeds.
	ua := fmt.Sprintf(
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Contourgo/%s+%s (+https://discord.com/users/%s; +https://%s)",
		Version,
		CommitSHA,
		botID,
		RepoURL,
	)
	req.Header.Set("User-Agent", ua)

	if t.Transport != nil {
		return t.Transport.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}

type TwitterScraper struct {
	client *http.Client
}

func NewTwitterScraper() *TwitterScraper {
	return &TwitterScraper{
		client: &http.Client{
			Timeout:   20 * time.Second,
			Transport: &twitterUserAgentTransport{},
		},
	}
}

// FetchRSS fetches and parses the Nitter RSS feed at the given URL.
// Returns the items and channel title (used as the account display name).
func (s *TwitterScraper) FetchRSS(rssURL string) ([]NitterItem, string, error) {
	req, err := http.NewRequest(http.MethodGet, rssURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml;q=0.9, */*;q=0.8")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, rssURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MiB limit
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %w", err)
	}

	var feed NitterRSS
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, "", fmt.Errorf("failed to parse RSS XML: %w", err)
	}

	// Nitter titles look like "Username / Nitter" — extract just the display name.
	displayName := feed.Channel.Title
	if idx := strings.Index(displayName, " / "); idx >= 0 {
		displayName = strings.TrimSpace(displayName[:idx])
	}

	return feed.Channel.Items, displayName, nil
}
