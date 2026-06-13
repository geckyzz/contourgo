package monitor

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/scraper"
)

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

	// 1. Proactive Search & Cache Torrents
	targetedTorrents := make(map[string]scraper.TsukihimeTorrent)

	for key, monitorCfg := range monitorMap {
		if key == "feedback" {
			continue
		}
		prefix := fmt.Sprintf("[TSUKIHIME][%s]", key)
		log.Printf("%s Processing monitor", prefix)

		// 1.1 Keywords -> SearchTorrents
		for _, kw := range monitorCfg.Keywords {
			if kw == "" {
				continue
			}
			for page := 0; page < maxPages; page++ {
				m.logFetch(prefix, page+1, map[string]any{
					"keyword": kw,
					"offset":  page * 100,
					"limit":   100,
				})
				results, err := scr.SearchTorrents(kw, 100, page*100)
				if err != nil {
					log.Printf("%s Search error for keyword %q: %v", prefix, kw, err)
					break
				}
				for _, t := range results {
					targetedTorrents[strconv.Itoa(t.ID)] = t
				}
				if len(results) < 100 {
					break
				}
				time.Sleep(300 * time.Millisecond)
			}
		}

		// 1.2 Groups -> FetchTorrentsByGroup
		for _, g := range monitorCfg.Groups {
			if g == "" {
				continue
			}
			for page := 0; page < maxPages; page++ {
				m.logFetch(prefix, page+1, map[string]any{
					"group":  g,
					"offset": page * 100,
					"limit":  100,
				})
				results, err := scr.FetchTorrentsByGroup(g, 100, page*100)
				if err != nil {
					log.Printf("%s Error fetching by group %q: %v", prefix, g, err)
					break
				}
				for _, t := range results {
					targetedTorrents[strconv.Itoa(t.ID)] = t
				}
				if len(results) < 100 {
					break
				}
				time.Sleep(300 * time.Millisecond)
			}
		}

		// 1.3 Media -> FetchTorrentsByAnime (Resolving if needed)
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
				log.Printf("%s Resolving anime ID for %q via %s", prefix, med, service)
				var err error
				internalID, err = scr.ResolveAnimeID(service, id)
				if err != nil {
					log.Printf("%s Error resolving anime ID for %q: %v", prefix, med, err)
					continue
				}
				if internalID == "" {
					log.Printf("%s Could not resolve internal ID for anime %q", prefix, med)
					continue
				}
				time.Sleep(300 * time.Millisecond)
			}

			for page := 0; page < maxPages; page++ {
				m.logFetch(prefix, page+1, map[string]any{
					"media_id": internalID,
					"offset":   page * 100,
					"limit":    100,
				})
				results, err := scr.FetchTorrentsByAnime(internalID, 100, page*100)
				if err != nil {
					log.Printf(
						"%s Error fetching by anime %q (internal: %s): %v",
						prefix,
						med,
						internalID,
						err,
					)
					break
				}
				for _, t := range results {
					targetedTorrents[strconv.Itoa(t.ID)] = t
				}
				if len(results) < 100 {
					break
				}
				time.Sleep(300 * time.Millisecond)
			}
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

	// 3. Process Comments
	for _, c := range allComments {
		isFeedbackComment := c.TargetType == "feedback"
		isTorrentComment := c.TargetType == "torrent"

		if !isFeedbackComment && !isTorrentComment {
			continue
		}

		var torrentID string
		if isFeedbackComment {
			torrentID = "feedback"
		} else {
			torrentID = c.GetTargetID()
		}
		if torrentID == "" {
			continue
		}

		commentID := c.GetID()

		for key, monitorCfg := range monitorMap {
			isFeedbackMonitor := key == "feedback"
			if isFeedbackComment != isFeedbackMonitor {
				continue
			}

			if m.db.IsCommentStored("tsukihime", torrentID, commentID) {
				continue
			}

			var details *scraper.TsukihimeTorrentDetails
			if isFeedbackComment {
				details = &scraper.TsukihimeTorrentDetails{Name: "Feedback"}
			} else {
				// Use targetedTorrents cache
				t, ok := targetedTorrents[torrentID]
				if !ok {
					// Optional: Fallback to direct fetch if we really care about all comments
					// But user suggested search-first strategy, so we only process targeted ones.
					continue
				}
				details = &scraper.TsukihimeTorrentDetails{
					ID:        t.ID,
					Name:      t.Name,
					AddedDate: t.AddedDate,
					Anime:     t.Anime,
					Group:     t.Group,
				}
			}

			if !m.matchTsukihimeComment(c, details, monitorCfg, isFeedbackComment) {
				continue
			}

			var ts int64
			parsedTime, err := time.Parse(time.RFC3339, c.CreatedAt)
			if err == nil {
				ts = parsedTime.Unix()
			} else {
				ts = time.Now().Unix()
			}
			avatarURL := ""
			if c.Author != nil && c.Author.AvatarHash != "" {
				avatarURL = fmt.Sprintf("https://tsukihime.org/cdn/pfp/%s", c.Author.AvatarHash)
			}

			m.db.UpdateTorrent("tsukihime", torrentID, details.Name, 1, details.AddedDate, "")
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

func (m *Monitor) matchTsukihimeComment(
	c scraper.TsukihimeComment,
	details *scraper.TsukihimeTorrentDetails,
	monitorCfg config.MonitorConfig,
	isFeedback bool,
) bool {
	if m.isExcluded(details.Name, monitorCfg.Excludes) {
		return false
	}

	if isFeedback {
		hasKeywordFilter := len(monitorCfg.Keywords) > 0

		if !hasKeywordFilter {
			return true
		}

		keywordMatched := false
		if hasKeywordFilter {
			commentTextLower := strings.ToLower(c.GetText())
			for _, kw := range monitorCfg.Keywords {
				if strings.Contains(commentTextLower, strings.ToLower(kw)) {
					keywordMatched = true
					break
				}
			}
		}

		return keywordMatched
	}

	hasGroupFilter := len(monitorCfg.Groups) > 0
	hasMediaFilter := len(monitorCfg.Media) > 0
	hasKeywordFilter := len(monitorCfg.Keywords) > 0

	if !hasGroupFilter && !hasMediaFilter && !hasKeywordFilter {
		return true
	}

	groupMatched := false
	if hasGroupFilter && details.Group != nil {
		gID := scraper.GetStringOrInt(details.Group.ID)
		gName := details.Group.Name
		for _, g := range monitorCfg.Groups {
			if strings.EqualFold(g, gID) || strings.EqualFold(g, gName) {
				groupMatched = true
				break
			}
		}
	}

	mediaMatched := false
	if hasMediaFilter && details.Anime != nil {
		animeID := scraper.GetStringOrInt(details.Anime.ID)
		malID := scraper.GetStringOrInt(details.Anime.MAL)
		anilistID := scraper.GetStringOrInt(details.Anime.Anilist)
		anidbID := scraper.GetStringOrInt(details.Anime.AniDB)
		animeTitle := details.Anime.Title
		animeEngTitle := details.Anime.EnglishTitle

		for _, med := range monitorCfg.Media {
			if after, ok := strings.CutPrefix(med, "mal:"); ok {
				val := after
				if val == malID {
					mediaMatched = true
					break
				}
			} else if after, ok := strings.CutPrefix(med, "anilist:"); ok {
				val := after
				if val == anilistID {
					mediaMatched = true
					break
				}
			} else if after, ok := strings.CutPrefix(med, "anidb:"); ok {
				val := after
				if val == anidbID {
					mediaMatched = true
					break
				}
			} else if after, ok := strings.CutPrefix(med, "tsukihime:"); ok {
				val := after
				if val == animeID {
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
		titleLower := strings.ToLower(details.Name)
		commentTextLower := strings.ToLower(c.GetText())
		for _, kw := range monitorCfg.Keywords {
			kwLower := strings.ToLower(kw)
			if strings.Contains(titleLower, kwLower) ||
				strings.Contains(commentTextLower, kwLower) {
				keywordMatched = true
				break
			}
		}
	}

	return (hasGroupFilter && groupMatched) ||
		(hasMediaFilter && mediaMatched) ||
		(hasKeywordFilter && keywordMatched)
}
