package discord

import (
	"bytes"
	"fmt"
	"log"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/geckyzz/contourgo/internal/config"
)

// knownEmbedServices maps short names to their replacement domain.
// Matched case-insensitively against the embed_service config value.
var knownEmbedServices = map[string]string{
	"fixupx":    "fixupx.com",
	"vxtwitter": "vxtwitter.com",
	"fxtwitter": "fxtwitter.com",
	"twittpr":   "twittpr.com",
	"fixvx":     "fixvx.com",
}

// twitterURLRegex matches twitter.com, x.com, and nitter instance tweet links.
var twitterURLRegex = regexp.MustCompile(
	`(?i)https?://(?:(?:www\.)?(?:twitter|x)\.com|nitter\.[a-z.]+)/([^/]+)/status/(\d+)`,
)

// RewriteTweetURL rewrites a tweet link to use the configured embed service domain.
// embedService may be a short name ("fixupx") or a bare domain ("fixupx.com").
// Returns the original link unchanged if embedService is empty or unrecognisable.
func RewriteTweetURL(link, embedService string) string {
	if embedService == "" {
		if matches := twitterURLRegex.FindStringSubmatch(link); len(matches) > 0 {
			return fmt.Sprintf("https://x.com/%s/status/%s", matches[1], matches[2])
		}
		return link
	}

	domain := strings.ToLower(strings.TrimSpace(embedService))
	if d, ok := knownEmbedServices[domain]; ok {
		domain = d
	}
	// Reject strings that don't look like a domain.
	if !strings.Contains(domain, ".") {
		return link
	}

	if matches := twitterURLRegex.FindStringSubmatch(link); len(matches) > 0 {
		return fmt.Sprintf("https://%s/%s/status/%s", domain, matches[1], matches[2])
	}
	return link
}

// ExtractTweetIDFromURL extracts the numeric tweet ID from a Nitter/Twitter URL.
// Returns empty string if not found.
func ExtractTweetIDFromURL(rawLink string) string {
	if matches := twitterURLRegex.FindStringSubmatch(rawLink); len(matches) > 2 {
		return matches[2]
	}
	return ""
}

// TwitterPostData holds the template-friendly data for a tweet announcement.
type TwitterPostData struct {
	Account      string // Twitter username without @
	DisplayName  string // Display name from the RSS channel title
	TweetID      string // Numeric tweet ID
	Link         string // Rewritten URL (embed_service applied if set), else canonical
	OriginalLink string // Canonical twitter.com URL
	Title        string // RSS item title (tweet text excerpt)
	PublishedAt  int64  // Unix timestamp
}

// renderCustomFormat executes the custom_format template string with the given data.
func renderCustomFormat(format string, data TwitterPostData) (string, error) {
	tmpl, err := template.New("twitter").Parse(format)
	if err != nil {
		return "", fmt.Errorf("invalid custom_format template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing custom_format template: %w", err)
	}
	return buf.String(), nil
}

// AnnounceTwitterPost sends a Discord message for a new tweet as plain content.
//
// Decision logic (no custom Discord embed is ever built):
//   - custom_format set  → render the template, send as content
//   - embed_service set  → post the rewritten link; Discord auto-embeds it
//   - neither            → post the canonical twitter.com link
//
// In all cases Discord's own link-preview engine handles the visual embed,
// which is exactly the purpose of embed services like fixupx / vxtwitter.
func (b *DiscordBot) AnnounceTwitterPost(
	channelID string,
	data TwitterPostData,
	monitorCfg config.MonitorConfig,
) error {
	if channelID == "" {
		channelID = b.AnnounceChannel
	}

	log.Printf(
		"[POST] Announcing Twitter post by @%s (tweet %s) to channel %s",
		data.Account, data.TweetID, channelID,
	)

	var content string

	switch {
	case monitorCfg.CustomFormat != "":
		// User-defined template — full control over the message text.
		rendered, err := renderCustomFormat(monitorCfg.CustomFormat, data)
		if err != nil {
			log.Printf("[TWITTER] %v — falling back to plain link", err)
			content = data.Link
		} else {
			content = rendered
		}

	default:
		// embed_service or no service: just post the (possibly rewritten) link.
		// Discord auto-previews it; embed services guarantee the embed works on X links.
		content = data.Link
	}

	// Prepend any matched @mention pings.
	mentions := b.GetMentionsForText(data.Title, b.Config.ResolveMentionsDisable(monitorCfg))
	if mentions != "" {
		content = mentions + "\n" + content
	}

	_, err := b.Session.ChannelMessageSend(channelID, content)
	return err
}

// ParseNitterRSSPubDate parses the pubDate string from a Nitter RSS feed.
// Nitter uses RFC1123/RFC822Z formats.
func ParseNitterRSSPubDate(s string) int64 {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.Unix()
		}
	}
	return time.Now().Unix()
}
