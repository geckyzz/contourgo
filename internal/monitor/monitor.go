package monitor

import (
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/db"
	"github.com/geckyzz/contourgo/internal/discord"
	"github.com/geckyzz/contourgo/internal/scraper"
)

type Monitor struct {
	config         *config.Config
	db             *db.DB
	bot            *discord.DiscordBot
	forceCheckChan chan bool
	lastCheckTime  time.Time
	DumpComments   bool
	lastCheckMap   map[string]map[string]time.Time
	mu             sync.Mutex
}

func NewMonitor(cfg *config.Config, database *db.DB, bot *discord.DiscordBot, forceCheckChan chan bool) *Monitor {
	return &Monitor{
		config:         cfg,
		db:             database,
		bot:            bot,
		forceCheckChan: forceCheckChan,
		lastCheckMap:   make(map[string]map[string]time.Time),
	}
}

func (m *Monitor) isDue(service, key string, monitorCfg config.MonitorConfig, force bool) bool {
	if force {
		return true
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	lastCheck, ok := m.lastCheckMap[service][key]
	if !ok {
		return true
	}

	interval := config.ParseISO8601Duration(m.config.Config.Monitor.By)
	if monitorCfg.Monitor.By != "" {
		interval = config.ParseISO8601Duration(monitorCfg.Monitor.By)
	}

	return time.Since(lastCheck) >= interval
}

func (m *Monitor) updateLastCheck(service, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.lastCheckMap == nil {
		m.lastCheckMap = make(map[string]map[string]time.Time)
	}
	if m.lastCheckMap[service] == nil {
		m.lastCheckMap[service] = make(map[string]time.Time)
	}
	m.lastCheckMap[service][key] = time.Now()
}

func (m *Monitor) hasActiveMonitorsDue(service string, force bool) bool {
	inner, ok := m.config.Monitors[service]
	if !ok || len(inner) == 0 {
		return false
	}
	for key, monitorCfg := range inner {
		if m.isDue(service, key, monitorCfg, force) {
			return true
		}
	}
	return false
}

func (m *Monitor) Start() {
	log.Println("Performing initial check on startup...")
	m.CheckAll(true)

	// Tick every 10 seconds to process due monitors
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.CheckAll(false)
		case <-m.forceCheckChan:
			log.Println("Manual check triggered.")
			m.CheckAll(true)
		}
	}
}

func (m *Monitor) isExcluded(title string, excludes []string) bool {
	if len(excludes) == 0 {
		return false
	}

	titleLower := strings.ToLower(title)
	for _, pattern := range excludes {
		patternLower := strings.ToLower(pattern)
		matched, err := filepath.Match(patternLower, titleLower)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func (m *Monitor) CheckAll(force bool) {
	m.lastCheckTime = time.Now()

	var wg sync.WaitGroup

	// 1. Nyaa
	if m.hasActiveMonitorsDue("nyaa", force) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.checkNyaa(force)
		}()
	}

	// 2. Sukebei
	if m.hasActiveMonitorsDue("sukebei", force) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.checkSukebei(force)
		}()
	}

	// 3. AnimeTosho Old
	if m.hasActiveMonitorsDue("animetosho_old", force) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.checkAnimeToshoOld(force)
		}()
	}

	// 4. AnimeTosho New
	if m.hasActiveMonitorsDue("animetosho_new", force) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.checkAnimeToshoNew(force)
		}()
	}

	// 5. NekoBT
	if m.hasActiveMonitorsDue("nekobt", force) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.checkNekoBT(force)
		}()
	}

	// 6. AniRena
	if m.hasActiveMonitorsDue("anirena", force) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.checkAnirena(force)
		}()
	}

	wg.Wait()
}

func (m *Monitor) hasActiveMonitors(service string) bool {
	inner, ok := m.config.Monitors[service]
	return ok && len(inner) > 0
}

func (m *Monitor) checkNyaa(force bool) {
	m.checkNyaaSukebeiService("nyaa", force)
}

func (m *Monitor) checkSukebei(force bool) {
	m.checkNyaaSukebeiService("sukebei", force)
}

func (m *Monitor) checkNyaaSukebeiService(service string, force bool) {
	svcName := strings.ToUpper(service)
	proxyURL := m.config.Config.Nyaa.Proxy.URL
	if proxyURL == "" {
		log.Printf("[%s] Warning: nyaa.proxy.url is empty, skipping.", svcName)
		return
	}

	var client interface {
		FetchTorrents(user, q string, page int, sort string, order string) ([]scraper.NyaaTorrent, int, error)
		FetchComments(torrentID string) ([]scraper.NyaaComment, error)
	}

	client = scraper.NewNyaaScraper(proxyURL, service)

	monitorMap := m.config.Monitors[service]
	for key, monitorCfg := range monitorMap {
		if !m.isDue(service, key, monitorCfg, force) {
			continue
		}
		m.updateLastCheck(service, key)

		log.Printf("[%s] Starting check...", svcName)
		prefix := fmt.Sprintf("[%s][%s]", svcName, key)
		log.Printf("%s Processing monitor", prefix)

		// Sorting & Ordering
		sort := monitorCfg.Sort
		if sort == "" {
			sort = "id"
		}
		order := monitorCfg.Order
		if order == "" {
			order = "desc"
		}

		// Create targets: combination of uploaders and keywords
		type target struct {
			user string
			q    string
		}
		var targets []target

		if len(monitorCfg.Uploaders) > 0 {
			for _, u := range monitorCfg.Uploaders {
				if len(monitorCfg.Keywords) > 0 {
					for _, k := range monitorCfg.Keywords {
						targets = append(targets, target{user: u, q: k})
					}
				} else {
					targets = append(targets, target{user: u, q: ""})
				}
			}
		} else if len(monitorCfg.Keywords) > 0 {
			for _, k := range monitorCfg.Keywords {
				targets = append(targets, target{user: "", q: k})
			}
		}

		if len(targets) == 0 {
			log.Printf("Monitor [%s.%s] has no keywords or uploaders, skipping.", service, key)
			continue
		}

		for _, tRef := range targets {
			page := 1
			for {
				log.Printf("%s Fetching page %d (user: %q, q: %q, sort: %s, order: %s)", prefix, page, tRef.user, tRef.q, sort, order)
				torrents, totalPages, err := client.FetchTorrents(tRef.user, tRef.q, page, sort, order)

				if err != nil {
					log.Printf("%s Error fetching torrents (page %d): %v", prefix, page, err)
					break
				}

				if len(torrents) == 0 {
					break
				}

				for _, t := range torrents {
					if sort == "comments" && order == "desc" && t.Comments == 0 {
						log.Printf("%s Reached torrent with 0 comments (sorted by comments desc), breaking early.", prefix)
						goto nextTarget
					}

					if m.isExcluded(t.Name, monitorCfg.Excludes) {
						continue
					}

					torrentIDStr := strconv.Itoa(t.ID)
					storedCount, exists := m.db.GetStoredCommentCount(service, torrentIDStr)

					if !exists || t.Comments > storedCount {
						if !exists {
							log.Printf("%s Found new torrent: %s (%d comments)", prefix, torrentIDStr, t.Comments)
						} else {
							log.Printf("%s Torrent %s has new comments: %d -> %d", prefix, torrentIDStr, storedCount, t.Comments)
						}

						var comments []scraper.NyaaComment
						comments, err = client.FetchComments(torrentIDStr)
						if err != nil {
							log.Printf("%s Error fetching comments: %v", prefix, err)
							continue
						}

						for _, c := range comments {
							commentIDStr := strconv.Itoa(c.ID)
							if !m.db.IsCommentStored(service, torrentIDStr, commentIDStr) {
								if !m.DumpComments {
									log.Printf("%s Announcing comment %d by %s on torrent %s", prefix, c.ID, c.Username, torrentIDStr)
									err := m.bot.AnnounceNyaaComment("", service, torrentIDStr, t.Name, c, monitorCfg.Discord.Embed.Thumbnail)
									if err != nil {
										log.Printf("%s Error announcing comment: %v", prefix, err)
									}
									time.Sleep(500 * time.Millisecond)
								} else {
									log.Printf("%s [DUMP] Storing comment %d by %s on torrent %s", prefix, c.ID, c.Username, torrentIDStr)
								}
								var ts int64
								parsedTime, err := time.Parse(time.RFC3339, c.Timestamp)
								if err == nil {
									ts = parsedTime.Unix()
								} else {
									ts = time.Now().Unix()
								}
								m.db.StoreComment(service, torrentIDStr, commentIDStr, c.Username, c.Text, ts, c.Pos, c.Role, c.Avatar)
							}
						}

						m.db.UpdateTorrent(service, torrentIDStr, t.Name, t.Comments)
					}
				}

				if monitorCfg.Page.Max > 0 && page >= monitorCfg.Page.Max {
					break
				}
				if page >= totalPages {
					break
				}
				page++
				time.Sleep(1 * time.Second)
			}
		nextTarget:
		}
	}
}

func (m *Monitor) checkAnimeToshoOld(force bool) {
	m.checkAnimeToshoService("animetosho_old", force)
}

func (m *Monitor) checkAnimeToshoNew(force bool) {
	m.checkAnimeToshoService("animetosho_new", force)
}

func (m *Monitor) checkAnimeToshoService(service string, force bool) {
	monitorMap, exists := m.config.Monitors[service]
	if !exists || len(monitorMap) == 0 {
		return
	}

	svcName := strings.ToUpper(service)

	var oldScraper *scraper.AnimeToshoOldScraper
	var newScraper *scraper.AnimeToshoNewScraper
	maxPagesCfg := 5

	if service == "animetosho_old" {
		oldScraper = scraper.NewAnimeToshoOldScraper()
		maxPagesCfg = m.config.Config.Animetosho.Old.Page.Max
	} else {
		newScraper = scraper.NewAnimeToshoNewScraper()
		maxPagesCfg = m.config.Config.Animetosho.New.Page.Max
	}

	if maxPagesCfg <= 0 {
		maxPagesCfg = 5
	}

	// Collect unique queries to perform (only relevant for New, but safe to collect for both)
	queries := make(map[string]bool)
	hasDueMonitors := false
	for key, monitorCfg := range monitorMap {
		if !m.isDue(service, key, monitorCfg, force) {
			continue
		}
		hasDueMonitors = true
		m.updateLastCheck(service, key)
		if key == "feedback" {
			continue // Handled separately
		}
		if service == "animetosho_old" {
			queries[""] = true // Global list
		} else {
			if len(monitorCfg.Keywords) == 0 {
				queries[""] = true
			} else {
				for _, kw := range monitorCfg.Keywords {
					queries[kw] = true
				}
			}
		}
	}

	if !hasDueMonitors {
		return
	}

	log.Printf("[%s] Starting check...", svcName)

	// Perform scraping for each unique query
	for q := range queries {
		page := 1
		for {
			var comments []scraper.ATComment
			var hasNext bool
			var err error

			if service == "animetosho_old" {
				log.Printf("[%s] Fetching page %d", service, page)
				comments, hasNext, err = oldScraper.ScrapeComments(page, false)
			} else {
				log.Printf("[%s] Fetching page %d for query: %q", service, page, q)
				comments, hasNext, err = newScraper.ScrapeComments(page, q, false)
			}

			if err != nil {
				log.Printf("Error scraping %s comments (page %d): %v", service, page, err)
				break
			}
			m.processATComments(service, comments, monitorMap)

			if !hasNext || page >= maxPagesCfg {
				break
			}
			page++
			time.Sleep(1 * time.Second)
		}
	}

	// If a feedback monitor exists, run a separate feedback scrape cycle
	if feedbackCfg, hasFeedback := monitorMap["feedback"]; hasFeedback && m.isDue(service, "feedback", feedbackCfg, force) {
		m.updateLastCheck(service, "feedback")
		page := 1
		for {
			var comments []scraper.ATComment
			var hasNext bool
			var err error

			if service == "animetosho_old" {
				log.Printf("[%s][FEEDBACK] Fetching page %d", service, page)
				comments, hasNext, err = oldScraper.ScrapeComments(page, true)
			} else {
				log.Printf("[%s][FEEDBACK] Fetching page %d", service, page)
				comments, hasNext, err = newScraper.ScrapeComments(page, "", true)
			}

			if err != nil {
				log.Printf("Error scraping %s feedback comments (page %d): %v", service, page, err)
				break
			}
			m.processATComments(service, comments, monitorMap)

			if !hasNext || page >= maxPagesCfg {
				break
			}
			page++
			time.Sleep(1 * time.Second)
		}
	}
}

func (m *Monitor) checkAnirena(force bool) {
	monitorMap, exists := m.config.Monitors["anirena"]
	if !exists || len(monitorMap) == 0 {
		return
	}

	hasDueMonitors := false
	for key, monitorCfg := range monitorMap {
		if m.isDue("anirena", key, monitorCfg, force) {
			hasDueMonitors = true
			break
		}
	}
	if !hasDueMonitors {
		return
	}

	log.Println("[ANIRENA] Starting check...")
	apiKey := m.config.Config.Anirena.API.Key

	if apiKey == "" {
		log.Printf("[ANIRENA] Warning: anirena.api.key is not configured, skipping.")
		return
	}

	client := scraper.NewAnirenaScraper(apiKey)

	for key, monitorCfg := range monitorMap {
		if !m.isDue("anirena", key, monitorCfg, force) {
			continue
		}
		m.updateLastCheck("anirena", key)

		prefix := fmt.Sprintf("[ANIRENA][%s]", key)
		log.Printf("%s Processing monitor", prefix)

		sort := monitorCfg.Sort
		if sort == "" {
			sort = "date"
		}
		order := monitorCfg.Order
		if order == "" {
			order = "desc"
		}

		type target struct {
			user  string
			group string
			q     string
		}
		var targets []target

		if len(monitorCfg.Uploaders) > 0 {
			for _, u := range monitorCfg.Uploaders {
				if len(monitorCfg.Keywords) > 0 {
					for _, k := range monitorCfg.Keywords {
						targets = append(targets, target{user: u, q: k})
					}
				} else {
					targets = append(targets, target{user: u, q: ""})
				}
			}
		}
		if len(monitorCfg.Groups) > 0 {
			for _, g := range monitorCfg.Groups {
				if len(monitorCfg.Keywords) > 0 {
					for _, k := range monitorCfg.Keywords {
						targets = append(targets, target{group: g, q: k})
					}
				} else {
					targets = append(targets, target{group: g, q: ""})
				}
			}
		}
		if len(monitorCfg.Uploaders) == 0 && len(monitorCfg.Groups) == 0 && len(monitorCfg.Keywords) > 0 {
			for _, k := range monitorCfg.Keywords {
				targets = append(targets, target{q: k})
			}
		}

		if len(targets) == 0 {
			log.Printf("Monitor [anirena.%s] has no keywords, uploaders or groups, skipping.", key)
			continue
		}

		for _, tRef := range targets {
			page := 1
			for {
				log.Printf("%s Fetching page %d (user: %q, group: %q, q: %q, sort: %s, order: %s)", prefix, page, tRef.user, tRef.group, tRef.q, sort, order)
				torrents, totalPages, err := client.FetchTorrents(tRef.user, tRef.group, tRef.q, page, sort, order)

				if err != nil {
					log.Printf("%s Error fetching torrents (page %d): %v", prefix, page, err)
					break
				}

				if len(torrents) == 0 {
					break
				}

				for _, t := range torrents {
					if sort == "comments" && order == "desc" && t.CommentCount == 0 {
						log.Printf("%s Reached torrent with 0 comments (sorted by comments desc), breaking early.", prefix)
						goto nextTarget
					}

					if m.isExcluded(t.FullTitle(), monitorCfg.Excludes) {
						continue
					}

					storedCount, exists := m.db.GetStoredCommentCount("anirena", t.ID)

					if !exists || t.CommentCount > storedCount {
						if !exists {
							log.Printf("%s Found new torrent: %s (%d comments)", prefix, t.ID, t.CommentCount)
						} else {
							log.Printf("%s Torrent %s has new comments: %d -> %d", prefix, t.ID, storedCount, t.CommentCount)
						}

						comments, err := client.FetchComments(t.ID)
						if err != nil {
							log.Printf("%s Error fetching comments: %v", prefix, err)
							continue
						}

						for _, c := range comments {
							if !m.db.IsCommentStored("anirena", t.ID, c.ID) {
								if !m.DumpComments {
									log.Printf("%s Announcing comment %s by %s on torrent %s", prefix, c.ID, c.Username, t.ID)
									err := m.bot.AnnounceAnirenaComment("", t.ID, t.FullTitle(), c, monitorCfg.Discord.Embed.Thumbnail, t.Uploader)
									if err != nil {
										log.Printf("%s Error announcing comment: %v", prefix, err)
									}
									time.Sleep(500 * time.Millisecond)
								} else {
									log.Printf("%s [DUMP] Storing comment %s by %s on torrent %s", prefix, c.ID, c.Username, t.ID)
								}
								var ts int64
								parsedTime, err := time.Parse("2006-01-02 15:04:05", c.CreatedAt)
								if err == nil {
									ts = parsedTime.Unix()
								} else {
									ts = time.Now().Unix()
								}
								m.db.StoreComment("anirena", t.ID, c.ID, c.Username, c.Body, ts, 0, c.Role, "")
							}
						}

						m.db.UpdateTorrent("anirena", t.ID, t.FullTitle(), t.CommentCount)
					}
				}

				if monitorCfg.Page.Max > 0 && page >= monitorCfg.Page.Max {
					break
				}
				if page >= totalPages {
					break
				}
				page++
				time.Sleep(1 * time.Second)
			}
		nextTarget:
		}
	}
}

func (m *Monitor) checkNekoBT(force bool) {
	monitorMap, exists := m.config.Monitors["nekobt"]
	if !exists || len(monitorMap) == 0 {
		return
	}

	hasDueMonitors := false
	for key, monitorCfg := range monitorMap {
		if m.isDue("nekobt", key, monitorCfg, force) {
			hasDueMonitors = true
			break
		}
	}
	if !hasDueMonitors {
		return
	}

	log.Println("[NEKOBT] Starting check...")
	apiKey := m.config.Config.Nekobt.API.Key
	scr := scraper.NewNekoBTScraper(apiKey)

	for key, monitorCfg := range monitorMap {
		if !m.isDue("nekobt", key, monitorCfg, force) {
			continue
		}
		m.updateLastCheck("nekobt", key)

		prefix := fmt.Sprintf("[NEKOBT][%s]", key)
		log.Printf("%s Processing monitor", prefix)

		sort := monitorCfg.Sort
		if sort == "" {
			sort = "latest"
		}

		// 1. Group Searches
		for _, gid := range monitorCfg.Groups {
			params := url.Values{}
			params.Set("group_id", gid)
			params.Set("sort_by", sort)
			m.checkNekoBTSearch(scr, params, monitorCfg, sort, prefix)
		}

		// 2. Uploader Searches
		for _, uid := range monitorCfg.Uploaders {
			params := url.Values{}
			params.Set("uploader_id", uid)
			params.Set("sort_by", sort)
			m.checkNekoBTSearch(scr, params, monitorCfg, sort, prefix)
		}

		// 3. Media Searches
		for _, mid := range monitorCfg.Media {
			params := url.Values{}
			if strings.HasPrefix(mid, "tmdb:") {
				params.Set("tmdbid", strings.TrimPrefix(mid, "tmdb:"))
			} else if strings.HasPrefix(mid, "tvdb:") {
				params.Set("tvdbid", strings.TrimPrefix(mid, "tvdb:"))
			} else {
				params.Set("media_id", mid)
			}
			params.Set("sort_by", sort)
			m.checkNekoBTSearch(scr, params, monitorCfg, sort, prefix)
		}

		// 4. Keyword Searches
		if len(monitorCfg.Keywords) > 0 {
			for _, kw := range monitorCfg.Keywords {
				params := url.Values{}
				params.Set("query", kw)
				params.Set("sort_by", sort)
				m.checkNekoBTSearch(scr, params, monitorCfg, sort, prefix)
			}
		}
	}
}

func (m *Monitor) checkNekoBTSearch(scr *scraper.NekoBTScraper, params url.Values, monitorCfg config.MonitorConfig, sort string, prefix string) {
	maxPages := monitorCfg.Page.Max
	if maxPages <= 0 {
		maxPages = 1
	}

	for page := 0; page < maxPages; page++ {
		params.Set("offset", strconv.Itoa(page*50))
		torrents, err := scr.SearchTorrents(params)
		if err != nil {
			log.Printf("%s Search error: %v", prefix, err)
			return
		}

		if len(torrents) == 0 {
			break
		}

		for _, t := range torrents {
			commentCount, _ := strconv.Atoi(t.CommentCount)
			if sort == "comments" && commentCount == 0 {
				log.Printf("%s Reached torrent with 0 comments (sorted by comments desc), breaking early.", prefix)
				return
			}

			if m.isExcluded(t.Title, monitorCfg.Excludes) {
				continue
			}

			storedCount, exists := m.db.GetStoredCommentCount("nekobt", t.ID)

			if !exists || commentCount > storedCount {
				if !exists {
					log.Printf("%s Found new torrent: %s (%s) with %d comments", prefix, t.ID, t.Title, commentCount)
				} else {
					log.Printf("%s Torrent %s has new comments: %d -> %d", prefix, t.ID, storedCount, commentCount)
				}

				comments, err := scr.FetchComments(t.ID, t.Title)
				if err != nil {
					log.Printf("%s Error fetching comments: %v", prefix, err)
					continue
				}

				for _, c := range comments {
					if !m.db.IsCommentStored("nekobt", t.ID, c.ID) {
						if !m.DumpComments {
							log.Printf("%s Announcing comment %s by %s on torrent %s", prefix, c.ID, c.DisplayName, t.ID)
							m.bot.AnnounceNekoBTComment("", t.Title, c, monitorCfg.Discord.Embed.Thumbnail)
							time.Sleep(500 * time.Millisecond)
						} else {
							log.Printf("%s [DUMP] Storing comment %s by %s on torrent %s", prefix, c.ID, c.DisplayName, t.ID)
						}

						pfpHash := "null"
						if c.PfpHash != nil && *c.PfpHash != "" {
							pfpHash = *c.PfpHash
						}
						avatarURL := fmt.Sprintf("https://nekobt.to/cdn/pfp/%s", pfpHash)
						m.db.StoreComment("nekobt", t.ID, c.ID, c.DisplayName, c.Text, c.CreatedAt/1000, 0, "", avatarURL)
					}
				}
				m.db.UpdateTorrent("nekobt", t.ID, t.Title, commentCount)
			}
		}
		if len(torrents) < 50 {
			break
		}
		time.Sleep(1 * time.Second)
	}
}

func (m *Monitor) processATComments(service string, comments []scraper.ATComment, monitorMap map[string]config.MonitorConfig) {
	for _, comment := range comments {
		for key, monitorCfg := range monitorMap {
			isFeedbackComment := comment.TorrentID == "feedback"
			isFeedbackMonitor := key == "feedback"

			// Only process feedback comments with feedback monitor config, and vice versa
			if isFeedbackComment != isFeedbackMonitor {
				continue
			}

			matches := false
			if len(monitorCfg.Keywords) == 0 {
				matches = true
			} else {
				if isFeedbackMonitor {
					// Check keywords against comment body/message (case-insensitive)
					msgLower := strings.ToLower(comment.Message)
					for _, kw := range monitorCfg.Keywords {
						if strings.Contains(msgLower, strings.ToLower(kw)) {
							matches = true
							break
						}
					}
				} else {
					// Check keywords against torrent title (case-insensitive)
					titleLower := strings.ToLower(comment.Title)
					for _, kw := range monitorCfg.Keywords {
						if strings.Contains(titleLower, strings.ToLower(kw)) {
							matches = true
							break
						}
					}
				}
			}

			if !matches || m.isExcluded(comment.Title, monitorCfg.Excludes) {
				continue
			}

			dbTorrentID := comment.TorrentID
			if strings.HasPrefix(dbTorrentID, "feedback") {
				dbTorrentID = "feedback"
			}

			if !m.db.IsCommentStored(service, dbTorrentID, comment.ID) {
				if !m.DumpComments {
					log.Printf("[%s] Announcing comment %s by %s on %s", strings.ToUpper(service), comment.ID, comment.Username, comment.TorrentID)
					err := m.bot.AnnounceATComment("", service, comment.TorrentID, comment.Title, comment, monitorCfg.Discord.Embed.Thumbnail)
					if err != nil {
						log.Printf("[%s] Error announcing comment: %v", strings.ToUpper(service), err)
					}
					time.Sleep(500 * time.Millisecond)
				} else {
					log.Printf("[%s] [DUMP] Storing comment %s by %s on %s", strings.ToUpper(service), comment.ID, comment.Username, comment.TorrentID)
				}
				m.db.StoreComment(service, dbTorrentID, comment.ID, comment.Username, comment.Message, comment.Timestamp, 0, "", "")
				m.db.UpdateTorrent(service, dbTorrentID, comment.Title, 1)
			}
		}
	}
}
