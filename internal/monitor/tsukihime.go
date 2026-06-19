package monitor

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/scraper"
)

// animetoshoDumpIDThreshold is the lower bound of IDs imported from the
// AnimeTosho legacy dump (e.g. 1000703115). Native Tsukihime IDs are in the
// low thousands, so any ID at or above this value belongs to the dump.
// Dump entries are still cached for comment matching, but must not update
// LastKnownID or trigger the native-sequence early-break.
const animetoshoDumpIDThreshold = 1_000_000_000

type TsukihimeTorrentCache struct {
	LastKnownID int                                 `json:"last_known_id"`
	Torrents    map[string]scraper.TsukihimeTorrent `json:"torrents"`
}

func (m *Monitor) checkTsukihime(force bool) {
	monitorMap, exists := m.config.Monitors["tsukihime"]
	if !exists || len(monitorMap) == 0 {
		return
	}

	hasDueMonitors := false
	for key, monitorCfg := range monitorMap {
		if m.isDue("tsukihime", key, monitorCfg, force) {
			hasDueMonitors = true
			break
		}
	}
	if !hasDueMonitors {
		return
	}

	log.Println("[TSUKIHIME] Starting check...")
	scr := scraper.NewTsukihimeScraper()

	for key, monitorCfg := range monitorMap {
		if m.isDue("tsukihime", key, monitorCfg, force) {
			m.updateLastCheck("tsukihime", key)
		}
	}

	maxPages := 1
	for _, monitorCfg := range monitorMap {
		if monitorCfg.Page.Max > maxPages {
			maxPages = monitorCfg.Page.Max
		}
	}
	if maxPages <= 0 {
		maxPages = 1
	}

	// 1. Load Cache (Dump File)
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	cachePath := filepath.Join(cacheDir, "contourgo", "tsukihime", "torrents.json")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		log.Printf("[TSUKIHIME] Error creating cache directory: %v", err)
	}

	var cache TsukihimeTorrentCache
	cache.Torrents = make(map[string]scraper.TsukihimeTorrent)

	cacheData, err := os.ReadFile(cachePath)
	if err == nil {
		if err := json.Unmarshal(cacheData, &cache); err != nil {
			log.Printf("[TSUKIHIME] Error parsing cache file: %v. Re-initializing cache.", err)
			cache.Torrents = make(map[string]scraper.TsukihimeTorrent)
		}
	}

	// Self-healing: scrub any AnimeTosho dump IDs that an older binary may
	// have written into the cache. Dump IDs must never appear in Torrents or
	// LastKnownID, as they would poison the incremental sync logic.
	if cache.LastKnownID >= animetoshoDumpIDThreshold {
		log.Printf("[TSUKIHIME] Resetting LastKnownID from dump range (%d) to 0", cache.LastKnownID)
		cache.LastKnownID = 0
	}
	for k, t := range cache.Torrents {
		if t.ID >= animetoshoDumpIDThreshold {
			delete(cache.Torrents, k)
		}
	}

	// Sync Cache using direct group and anime lookup
	newTorrents := m.syncTsukihimeCache(monitorMap, scr, &cache)

	maxID := cache.LastKnownID
	for _, t := range newTorrents {
		// Don't let AnimeTosho dump IDs (>= 1 billion) inflate LastKnownID.
		if t.ID < animetoshoDumpIDThreshold && t.ID > maxID {
			maxID = t.ID
		}
		// Match against non-feedback monitors
		matched := false
		for mKey, monitorCfg := range monitorMap {
			if mKey == "feedback" {
				continue
			}
			if m.matchTsukihimeTorrent(t, monitorCfg) {
				matched = true
				break
			}
		}
		if matched {
			cache.Torrents[strconv.Itoa(t.ID)] = t
		}
	}

	cache.LastKnownID = maxID

	// Write cache back
	newCacheData, err := json.MarshalIndent(cache, "", "  ")
	if err == nil {
		if err := os.WriteFile(cachePath, newCacheData, 0644); err != nil {
			log.Printf("[TSUKIHIME] Error saving cache to %s: %v", cachePath, err)
		} else {
			log.Printf("[TSUKIHIME] Saved cache file to %s. Total matched torrents: %d", cachePath, len(cache.Torrents))
		}
	}

	// 2. Fetch Latest Comments
	var allComments []scraper.TsukihimeComment
	for page := 0; page < maxPages; page++ {
		m.logFetch("[TSUKIHIME]", page+1, map[string]any{
			"offset": page * 100,
			"limit":  100,
		})
		resp, err := scr.FetchLatestComments(100, page*100)
		if err != nil {
			log.Printf("[TSUKIHIME] Error fetching latest comments (offset: %d): %v", page*100, err)
			break
		}
		if len(resp.Comments) == 0 {
			break
		}
		allComments = append(allComments, resp.Comments...)
		if len(resp.Comments) < 100 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	commentMap := make(map[string]scraper.TsukihimeComment)
	for _, c := range allComments {
		commentMap[c.GetID()] = c
	}

	// Resolve parent comments text
	for i := range allComments {
		parentID := allComments[i].GetParentID()
		if parentID != "" {
			if parent, ok := commentMap[parentID]; ok {
				allComments[i].ParentText = parent.GetText()
			}
		}
	}

	// 3. Process Comments
	for _, c := range allComments {
		isFeedbackComment := c.TargetType == "feedback"
		isTorrentComment := c.TargetType == "torrent"

		if !isFeedbackComment && !isTorrentComment {
			continue
		}

		commentID := c.GetID()

		for key, monitorCfg := range monitorMap {
			isFeedbackMonitor := key == "feedback"
			if isFeedbackComment != isFeedbackMonitor {
				continue
			}

			if isFeedbackComment {
				if m.db.IsCommentStored("tsukihime", "feedback", commentID) {
					continue
				}

				hasKeywordFilter := len(monitorCfg.Keywords) > 0
				hasUploaderFilter := len(monitorCfg.Uploaders) > 0

				keywordMatched := false
				if hasKeywordFilter {
					commentTextLower := strings.ToLower(c.GetText())
					for _, kw := range monitorCfg.Keywords {
						if strings.Contains(commentTextLower, strings.ToLower(kw)) {
							keywordMatched = true
							break
						}
					}
				} else {
					keywordMatched = true
				}

				uploaderMatched := false
				if hasUploaderFilter {
					uUsername := c.GetUsername()
					uDisplayName := c.GetDisplayName()
					uID := c.GetUserID()
					for _, up := range monitorCfg.Uploaders {
						if strings.EqualFold(up, uUsername) ||
							strings.EqualFold(up, uDisplayName) ||
							up == uID {
							uploaderMatched = true
							break
						}
					}
				} else {
					uploaderMatched = true
				}

				if (hasKeywordFilter || hasUploaderFilter) &&
					(!keywordMatched && !uploaderMatched) {
					continue
				}

				var ts int64
				if pt, err := time.Parse(time.RFC3339, c.CreatedAt); err == nil {
					ts = pt.Unix()
				} else if pt, err := time.ParseInLocation(time.DateTime, c.CreatedAt, time.UTC); err == nil {
					// API returns timestamps without timezone suffix (e.g. "2026-06-19T10:20:53")
					ts = pt.Unix()
				} else {
					ts = time.Now().Unix()
				}
				avatarURL := ""
				if c.Author != nil && c.Author.AvatarHash != "" {
					avatarURL = fmt.Sprintf("https://tsukihime.org/cdn/pfp/%s", c.Author.AvatarHash)
				}

				m.db.UpdateTorrent("tsukihime", "feedback", "Feedback", 1, 0, "")
				m.db.StoreComment(
					"tsukihime",
					"feedback",
					commentID,
					c.GetDisplayName(),
					c.GetText(),
					ts,
					0,
					"",
					avatarURL,
					c.GetParentID(),
					c.ParentText,
				)

				m.enqueueAnnouncement(
					fmt.Sprintf("[TSUKIHIME][%s]", key),
					"tsukihime",
					"feedback",
					commentID,
					monitorCfg,
				)
			} else {
				torrentID := c.GetTargetID()
				if torrentID == "" {
					continue
				}

				if m.db.IsCommentStored("tsukihime", torrentID, commentID) {
					continue
				}

				t, ok := cache.Torrents[torrentID]
				if !ok {
					// Skip if not matched and recorded in cache
					continue
				}

				hasUploaderFilter := len(monitorCfg.Uploaders) > 0
				uploaderMatched := false
				if hasUploaderFilter {
					uUsername := c.GetUsername()
					uDisplayName := c.GetDisplayName()
					uID := c.GetUserID()
					for _, up := range monitorCfg.Uploaders {
						if strings.EqualFold(up, uUsername) || strings.EqualFold(up, uDisplayName) || up == uID {
							uploaderMatched = true
							break
						}
					}
				} else {
					uploaderMatched = true
				}

				if !uploaderMatched {
					continue
				}

				if m.isExcluded(t.Name, monitorCfg.Excludes) {
					continue
				}

				var ts int64
				if pt, err := time.Parse(time.RFC3339, c.CreatedAt); err == nil {
					ts = pt.Unix()
				} else if pt, err := time.ParseInLocation(time.DateTime, c.CreatedAt, time.UTC); err == nil {
					// API returns timestamps without timezone suffix (e.g. "2026-06-19T10:20:53")
					ts = pt.Unix()
				} else {
					ts = time.Now().Unix()
				}
				avatarURL := ""
				if c.Author != nil && c.Author.AvatarHash != "" {
					avatarURL = fmt.Sprintf("https://tsukihime.org/cdn/pfp/%s", c.Author.AvatarHash)
				}

				m.db.UpdateTorrent("tsukihime", torrentID, t.Name, 1, t.AddedDate, "")
				m.db.StoreComment(
					"tsukihime",
					torrentID,
					commentID,
					c.GetDisplayName(),
					c.GetText(),
					ts,
					0,
					"",
					avatarURL,
					c.GetParentID(),
					c.ParentText,
				)

				m.enqueueAnnouncement(
					fmt.Sprintf("[TSUKIHIME][%s]", key),
					"tsukihime",
					torrentID,
					commentID,
					monitorCfg,
				)
			}
		}
	}
}

func (m *Monitor) syncTsukihimeCache(
	monitorMap map[string]config.MonitorConfig,
	scr *scraper.TsukihimeScraper,
	cache *TsukihimeTorrentCache,
) []scraper.TsukihimeTorrent {
	var newTorrents []scraper.TsukihimeTorrent
	maxPages := 1000 // effectively infinite page limit for initial catalog fetch

	fetchedGroups := make(map[string]bool)
	fetchedAnime := make(map[string]bool)
	fetchedKeywords := make(map[string]bool)

	isFirstInit := len(cache.Torrents) == 0

	for mKey, monitorCfg := range monitorMap {
		if mKey == "feedback" {
			continue
		}

		// 1. Groups
		for _, g := range monitorCfg.Groups {
			if g == "" || fetchedGroups[g] {
				continue
			}
			fetchedGroups[g] = true

			log.Printf("[TSUKIHIME] Syncing group %s...", g)
			offset := 0
			reachedLastKnown := false
			for page := 0; page < maxPages; page++ {
				m.logFetch("[TSUKIHIME]["+mKey+"]", page+1, map[string]any{
					"group":  g,
					"offset": offset,
					"limit":  100,
				})
				results, err := scr.FetchTorrentsByGroup(g, 100, offset)
				if err != nil {
					log.Printf("[TSUKIHIME] Error fetching group %s: %v", g, err)
					break
				}
				if len(results) == 0 {
					break
				}
				for _, t := range results {
					// Only apply the LastKnownID early-break to native IDs;
					// dump entries (ID >= 1B) are outside the native sequence.
					if t.ID < animetoshoDumpIDThreshold && !isFirstInit &&
						t.ID <= cache.LastKnownID {
						log.Printf(
							"[TSUKIHIME][%s] Reached last known ID %d (torrent %d, group %s), stopping.",
							mKey,
							cache.LastKnownID,
							t.ID,
							g,
						)
						reachedLastKnown = true
						break
					}
					// Inject synthetic group info since the API does not
					// embed a group object in /v1/groups/{id} responses.
					if t.Group == nil {
						t.Group = &scraper.TsukihimeTorrentGroup{ID: g}
					}
					newTorrents = append(newTorrents, t)
				}
				if reachedLastKnown || len(results) < 100 {
					break
				}
				offset += 100
				time.Sleep(300 * time.Millisecond)
			}
		}

		// 2. Anime/Media
		for _, med := range monitorCfg.Media {
			if med == "" {
				continue
			}
			service := "tsukihime"
			id := med
			if strings.Contains(med, ":") {
				parts := strings.SplitN(med, ":", 2)
				service = parts[0]
				id = parts[1]
			}

			internalID := id
			if service != "tsukihime" {
				log.Printf("[TSUKIHIME] Resolving anime ID for %q via %s", med, service)
				var err error
				internalID, err = scr.ResolveAnimeID(service, id)
				if err != nil || internalID == "" {
					log.Printf("[TSUKIHIME] Error resolving anime ID for %q: %v", med, err)
					continue
				}
				time.Sleep(300 * time.Millisecond)
			}

			if fetchedAnime[internalID] {
				continue
			}
			fetchedAnime[internalID] = true

			log.Printf("[TSUKIHIME] Syncing anime ID %s...", internalID)
			offset := 0
			reachedLastKnown := false
			for page := 0; page < maxPages; page++ {
				m.logFetch("[TSUKIHIME]["+mKey+"]", page+1, map[string]any{
					"anime_id": internalID,
					"offset":   offset,
					"limit":    100,
				})
				results, err := scr.FetchTorrentsByAnime(internalID, 100, offset)
				if err != nil {
					log.Printf("[TSUKIHIME] Error fetching anime %s: %v", internalID, err)
					break
				}
				if len(results) == 0 {
					break
				}
				for _, t := range results {
					// Only apply the LastKnownID early-break to native IDs;
					// dump entries (ID >= 1B) are outside the native sequence.
					if t.ID < animetoshoDumpIDThreshold && !isFirstInit &&
						t.ID <= cache.LastKnownID {
						log.Printf(
							"[TSUKIHIME][%s] Reached last known ID %d (torrent %d, anime %s), stopping.",
							mKey,
							cache.LastKnownID,
							t.ID,
							internalID,
						)
						reachedLastKnown = true
						break
					}
					// Inject synthetic anime info since the API does not
					// embed an anime object in /v1/animes/{id} responses.
					if t.Anime == nil {
						t.Anime = &scraper.TsukihimeTorrentAnime{ID: internalID}
					}
					newTorrents = append(newTorrents, t)
				}
				if reachedLastKnown || len(results) < 100 {
					break
				}
				offset += 100
				time.Sleep(300 * time.Millisecond)
			}
		}

		// 3. Keywords
		for _, kw := range monitorCfg.Keywords {
			if kw == "" || fetchedKeywords[kw] {
				continue
			}
			fetchedKeywords[kw] = true

			log.Printf("[TSUKIHIME] Syncing keyword %q...", kw)
			offset := 0
			reachedLastKnown := false
			for page := 0; page < maxPages; page++ {
				m.logFetch("[TSUKIHIME]["+mKey+"]", page+1, map[string]any{
					"keyword": kw,
					"offset":  offset,
					"limit":   100,
				})
				results, err := scr.SearchTorrents(kw, 100, offset)
				if err != nil {
					log.Printf("[TSUKIHIME] Error searching keyword %q: %v", kw, err)
					break
				}
				if len(results) == 0 {
					break
				}
				for _, t := range results {
					// Only apply the LastKnownID early-break to native IDs;
					// dump entries (ID >= 1B) are outside the native sequence.
					if t.ID < animetoshoDumpIDThreshold && !isFirstInit &&
						t.ID <= cache.LastKnownID {
						log.Printf(
							"[TSUKIHIME][%s] Reached last known ID %d (torrent %d, keyword %q), stopping.",
							mKey,
							cache.LastKnownID,
							t.ID,
							kw,
						)
						reachedLastKnown = true
						break
					}
					newTorrents = append(newTorrents, t)
				}
				if reachedLastKnown || len(results) < 100 {
					break
				}
				offset += 100
				time.Sleep(300 * time.Millisecond)
			}
		}
	}

	return newTorrents
}

func (m *Monitor) matchTsukihimeTorrent(
	t scraper.TsukihimeTorrent,
	monitorCfg config.MonitorConfig,
) bool {
	if m.isExcluded(t.Name, monitorCfg.Excludes) {
		return false
	}

	hasGroupFilter := len(monitorCfg.Groups) > 0
	hasMediaFilter := len(monitorCfg.Media) > 0
	hasKeywordFilter := len(monitorCfg.Keywords) > 0

	if !hasGroupFilter && !hasMediaFilter && !hasKeywordFilter {
		return true
	}

	groupMatched := false
	if hasGroupFilter && t.Group != nil {
		gID := scraper.GetStringOrInt(t.Group.ID)
		gName := t.Group.Name
		for _, g := range monitorCfg.Groups {
			if strings.EqualFold(g, gID) || strings.EqualFold(g, gName) {
				groupMatched = true
				break
			}
		}
	}

	mediaMatched := false
	if hasMediaFilter && t.Anime != nil {
		animeID := scraper.GetStringOrInt(t.Anime.ID)
		malID := scraper.GetStringOrInt(t.Anime.MAL)
		anilistID := scraper.GetStringOrInt(t.Anime.Anilist)
		anidbID := scraper.GetStringOrInt(t.Anime.AniDB)
		animeTitle := t.Anime.Title
		animeEngTitle := t.Anime.EnglishTitle

		for _, med := range monitorCfg.Media {
			if after, ok := strings.CutPrefix(med, "mal:"); ok {
				if after == malID {
					mediaMatched = true
					break
				}
			} else if after, ok := strings.CutPrefix(med, "anilist:"); ok {
				if after == anilistID {
					mediaMatched = true
					break
				}
			} else if after, ok := strings.CutPrefix(med, "anidb:"); ok {
				if after == anidbID {
					mediaMatched = true
					break
				}
			} else {
				if med == animeID || med == malID || med == anilistID || med == anidbID || strings.EqualFold(med, animeTitle) || strings.EqualFold(med, animeEngTitle) {
					mediaMatched = true
					break
				}
			}
		}
	}

	keywordMatched := false
	if hasKeywordFilter {
		titleLower := strings.ToLower(t.Name)
		for _, kw := range monitorCfg.Keywords {
			if strings.Contains(titleLower, strings.ToLower(kw)) {
				keywordMatched = true
				break
			}
		}
	}

	return (hasGroupFilter && groupMatched) ||
		(hasMediaFilter && mediaMatched) ||
		(hasKeywordFilter && keywordMatched)
}
