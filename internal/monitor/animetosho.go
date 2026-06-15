package monitor

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/scraper"
)

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

	prefix := "[ANIMETOSHO-OLD]"
	globalMaxPages := m.config.Config.Animetosho.Old.Page.Max
	if service == "animetosho_new" {
		prefix = "[ANIMETOSHO-NEW]"
		globalMaxPages = m.config.Config.Animetosho.New.Page.Max
	}

	hasDueMonitors := false
	var activeKeys []string
	var torrentKeys []string
	isFeedbackDue := false

	for key, monitorCfg := range monitorMap {
		if m.isDue(service, key, monitorCfg, force) {
			hasDueMonitors = true
			activeKeys = append(activeKeys, key)
			if key == "feedback" {
				isFeedbackDue = true
			} else {
				torrentKeys = append(torrentKeys, key)
			}
			m.updateLastCheck(service, key)
			log.Printf("%s[%s] Processing monitor", prefix, key)
		}
	}
	if !hasDueMonitors {
		return
	}

	fetchPrefix := fmt.Sprintf("%s[%s]", prefix, strings.Join(activeKeys, ","))
	torrentFetchPrefix := fetchPrefix
	if len(torrentKeys) > 0 {
		torrentFetchPrefix = fmt.Sprintf("%s[%s]", prefix, strings.Join(torrentKeys, ","))
	}
	log.Printf("%s Starting check...", fetchPrefix)

	maxPages := globalMaxPages
	for _, k := range activeKeys {
		if monitorMap[k].Page.Max > maxPages {
			maxPages = monitorMap[k].Page.Max
		}
	}
	if maxPages <= 0 {
		maxPages = 1
	}

	// 1. Fetch Latest Comments (Global Feed now serves only torrent monitors)
	if len(torrentKeys) > 0 || force {
		page := 1
		for {
			m.logFetch(torrentFetchPrefix, page, map[string]any{"q": "Global Feed"})
			var comments []scraper.ATComment
			var hasNext bool
			var err error

			if service == "animetosho_new" {
				scr := scraper.NewAnimeToshoNewScraper()
				comments, hasNext, err = scr.ScrapeComments(page, "", false)
			} else {
				scr := scraper.NewAnimeToshoOldScraper()
				comments, hasNext, err = scr.ScrapeComments(page, false)
			}

			if err != nil {
				log.Printf(
					"%s Error fetching latest comments (page %d): %v",
					torrentFetchPrefix,
					page,
					err,
				)
				break
			}
			m.processATComments(service, comments, monitorMap)

			if !hasNext || page >= maxPages {
				break
			}
			page++
			time.Sleep(1 * time.Second)
		}
	}

	// 2. Fetch Feedback (Dedicated feed, only serves the feedback monitor)
	if _, ok := monitorMap["feedback"]; ok {
		if !isFeedbackDue && !force {
			goto skipFeedback
		}

		feedbackPrefix := fmt.Sprintf("%s[feedback]", prefix)
		page := 1
		maxPagesCfg := monitorMap["feedback"].Page.Max
		if maxPagesCfg <= 0 {
			if globalMaxPages > 0 {
				maxPagesCfg = globalMaxPages
			} else {
				maxPagesCfg = 1
			}
		}
		for {
			m.logFetch(feedbackPrefix, page, map[string]any{"q": "Feedback Feed"})
			var comments []scraper.ATComment
			var hasNext bool
			var err error

			if service == "animetosho_new" {
				scr := scraper.NewAnimeToshoNewScraper()
				comments, hasNext, err = scr.ScrapeComments(page, "", true)
			} else {
				scr := scraper.NewAnimeToshoOldScraper()
				comments, hasNext, err = scr.ScrapeComments(page, true)
			}

			if err != nil {
				log.Printf(
					"%s Error scraping feedback comments (page %d): %v",
					feedbackPrefix,
					page,
					err,
				)
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
skipFeedback:
}

func (m *Monitor) processATComments(
	service string,
	comments []scraper.ATComment,
	monitorMap map[string]config.MonitorConfig,
) {
	prefix := "[ANIMETOSHO-OLD]"
	if service == "animetosho_new" {
		prefix = "[ANIMETOSHO-NEW]"
	}

	for _, comment := range comments {
		for key, monitorCfg := range monitorMap {
			if m.isExcluded(comment.Title, monitorCfg.Excludes) {
				continue
			}

			isFeedbackMonitor := (key == "feedback")
			isFeedbackComment := (comment.Type == "Feedback")
			if isFeedbackMonitor != isFeedbackComment {
				continue
			}

			if !isFeedbackMonitor {
				keywordMatched := false
				if len(monitorCfg.Keywords) == 0 {
					keywordMatched = true
				} else {
					titleLower := strings.ToLower(comment.Title)
					for _, kw := range monitorCfg.Keywords {
						if strings.Contains(titleLower, strings.ToLower(kw)) {
							keywordMatched = true
							break
						}
					}
				}
				if !keywordMatched {
					continue
				}
			}

			dbTorrentID := comment.TorrentID
			if isFeedbackComment {
				dbTorrentID = "feedback"
			}

			if !m.db.IsCommentStored(service, dbTorrentID, comment.ID) {
				var parentID, parentMsg string
				targetURL := "https://animetosho.org"
				if service == "animetosho_new" {
					targetURL = "https://animetosho.xyz"
				}
				if isFeedbackComment {
					targetURL += "/feedback"
				} else {
					targetURL += "/view/" + comment.TorrentID
				}

				log.Printf(
					"%s[%s] Fetching comment thread to resolve parent: %s",
					prefix,
					key,
					targetURL,
				)
				client := &http.Client{
					Timeout: 15 * time.Second,
				}
				parentID, parentMsg = scraper.ResolveParentInfo(client, targetURL, comment.ID)
				if parentID != "" {
					log.Printf("%s[%s] Resolved parent comment ID=%s", prefix, key, parentID)
				}

				m.db.UpdateTorrent(service, dbTorrentID, comment.Title, 1, comment.Timestamp, "")
				m.db.StoreComment(
					service,
					dbTorrentID,
					comment.ID,
					comment.Username,
					comment.Message,
					comment.Timestamp,
					0,
					"",
					"",
					parentID,
					parentMsg,
				)

				m.enqueueAnnouncement(
					fmt.Sprintf("%s[%s]", prefix, key),
					service,
					dbTorrentID,
					comment.ID,
					monitorCfg,
				)
			}
		}
	}
}
