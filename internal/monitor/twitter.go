package monitor

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/discord"
	"github.com/geckyzz/contourgo/internal/scraper"
)

// checkTwitter is the entry point called from CheckAll.
// It follows the exact same structure as checkNyaa, checkNekoBT, etc.
func (m *Monitor) checkTwitter(force bool, targetKey string) {
	cfg := m.Config()
	monitorMap, exists := cfg.Monitors["twitter"]
	if !exists || len(monitorMap) == 0 {
		return
	}

	scr := scraper.NewTwitterScraper()

	for key, monitorCfg := range monitorMap {
		if targetKey != "" && key != targetKey {
			continue
		}
		if !m.isDue("twitter", key, monitorCfg, force) {
			continue
		}
		m.updateLastCheck("twitter", key)

		monitorPrefix := fmt.Sprintf("[TWITTER][%s]", key)
		m.checkTwitterAccount(monitorPrefix, key, monitorCfg, scr)
	}
}

// checkTwitterAccount fetches and processes the Nitter RSS for one monitored account.
func (m *Monitor) checkTwitterAccount(
	prefix, key string,
	monitorCfg config.MonitorConfig,
	scr *scraper.TwitterScraper,
) {
	cfg := m.Config()
	// Resolve account name: explicit `account` field, falling back to the monitor key.
	account := monitorCfg.Account
	if account == "" {
		account = key
	}

	nitterBase := cfg.ResolveNitterURL(monitorCfg)
	rssURL := fmt.Sprintf("%s/%s/rss", nitterBase, account)

	log.Printf("%s Fetching Nitter RSS: %s", prefix, rssURL)

	items, displayName, err := scr.FetchRSS(rssURL)
	if err != nil {
		log.Printf("%s Error fetching RSS: %v", prefix, err)
		return
	}

	if len(items) == 0 {
		log.Printf("%s No items in RSS feed.", prefix)
		return
	}

	log.Printf("%s Got %d items (display: %q).", prefix, len(items), displayName)

	// Pre-compile regexes for keywords and excludes
	var keywordRegexes []*regexp.Regexp
	for _, kw := range monitorCfg.Keywords {
		if kw != "" {
			re, err := regexp.Compile("(?i)" + kw)
			if err != nil {
				log.Printf("%s Error compiling keyword regex %q: %v", prefix, kw, err)
				continue
			}
			keywordRegexes = append(keywordRegexes, re)
		}
	}

	var excludeRegexes []*regexp.Regexp
	for _, ex := range monitorCfg.Excludes {
		if ex != "" {
			re, err := regexp.Compile("(?i)" + ex)
			if err != nil {
				log.Printf("%s Error compiling exclude regex %q: %v", prefix, ex, err)
				continue
			}
			excludeRegexes = append(excludeRegexes, re)
		}
	}

	// Process items in chronological order (oldest first so Discord timeline is correct).
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]

		// 0. Exclude reposts if configured
		if cfg.ResolveExcludeReposts(monitorCfg) && strings.HasPrefix(item.Title, "RT by @") {
			continue
		}

		// 1. Exclude filter (regex-compatible)
		if len(excludeRegexes) > 0 {
			matchedExclude := false
			for _, re := range excludeRegexes {
				if re.MatchString(item.Title) {
					matchedExclude = true
					break
				}
			}
			if matchedExclude {
				continue
			}
		}

		// 2. Keyword filter (regex-compatible)
		if len(keywordRegexes) > 0 {
			matchedKeyword := false
			for _, re := range keywordRegexes {
				if re.MatchString(item.Title) {
					matchedKeyword = true
					break
				}
			}
			if !matchedKeyword {
				continue
			}
		}

		tweetID := discord.ExtractTweetIDFromURL(item.Link)
		if tweetID == "" {
			// Fallback: use GUID as the dedup key.
			tweetID = item.GUID
			if tweetID == "" {
				log.Printf("%s Skipping item with no tweet ID: %q", prefix, item.Link)
				continue
			}
		}

		if m.db.IsTweetSeen(account, tweetID) {
			continue
		}

		pubAt := discord.ParseNitterRSSPubDate(item.PubDate)
		embedSvc := cfg.ResolveEmbedService(monitorCfg)
		rewrittenLink := discord.RewriteTweetURL(item.Link, embedSvc)
		canonicalLink := normaliseToXLink(account, tweetID)

		postData := discord.TwitterPostData{
			Account:      account,
			DisplayName:  displayName,
			TweetID:      tweetID,
			Link:         rewrittenLink,
			OriginalLink: canonicalLink,
			Title:        item.Title,
			PublishedAt:  pubAt,
		}

		channelID := string(monitorCfg.Discord.Channel)

		if !m.DumpComments {
			log.Printf("%s Announcing tweet %s by @%s", prefix, tweetID, account)
			if err := m.bot.AnnounceTwitterPost(channelID, postData, monitorCfg); err != nil {
				log.Printf("%s Error announcing tweet %s: %v", prefix, tweetID, err)
				// Don't mark as seen on failure so we retry on the next cycle.
				continue
			}
		} else {
			log.Printf("%s [DUMP] Tweet %s by @%s: %s", prefix, tweetID, account, item.Title)
		}

		if err := m.db.StoreTweet(account, tweetID, item.Title, canonicalLink, pubAt); err != nil {
			log.Printf("%s Error storing tweet %s: %v", prefix, tweetID, err)
		}
	}
}

// normaliseToXLink returns the canonical x.com URL for a tweet.
func normaliseToXLink(account, tweetID string) string {
	return fmt.Sprintf("https://x.com/%s/status/%s", account, tweetID)
}
