package monitor

import (
	"fmt"
	"log"
	"net/url"

	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/scraper"
)

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
			sort = "date"
		}

		params := url.Values{}
		params.Set("sort_by", sort)
		params.Set("limit", "50")

		if len(monitorCfg.Keywords) > 0 {
			for _, kw := range monitorCfg.Keywords {
				params.Set("query", kw)
				m.checkNekoBTSearch(scr, params, monitorCfg, sort, prefix)
			}
		}
		if len(monitorCfg.Groups) > 0 {
			params.Del("query")
			for _, grp := range monitorCfg.Groups {
				params.Set("group_id", grp)
				m.checkNekoBTSearch(scr, params, monitorCfg, sort, prefix)
			}
		}
		if len(monitorCfg.Uploaders) > 0 {
			params.Del("query")
			params.Del("group_id")
			for _, up := range monitorCfg.Uploaders {
				params.Set("uploader_id", up)
				m.checkNekoBTSearch(scr, params, monitorCfg, sort, prefix)
			}
		}
		if len(monitorCfg.Media) > 0 {
			params.Del("query")
			params.Del("group_id")
			params.Del("uploader_id")
			for _, med := range monitorCfg.Media {
				params.Set("media_id", med)
				m.checkNekoBTSearch(scr, params, monitorCfg, sort, prefix)
			}
		}

		if len(monitorCfg.Keywords) == 0 && len(monitorCfg.Groups) == 0 &&
			len(monitorCfg.Uploaders) == 0 &&
			len(monitorCfg.Media) == 0 {
			m.checkNekoBTSearch(scr, params, monitorCfg, sort, prefix)
		}
	}
}

func (m *Monitor) checkNekoBTSearch(
	scr *scraper.NekoBTScraper,
	params url.Values,
	monitorCfg config.MonitorConfig,
	sort string,
	prefix string,
) {
	maxPages := monitorCfg.Page.Max
	if maxPages <= 0 {
		maxPages = 1
	}

	for page := 0; page < maxPages; page++ {
		offset := page * 50
		params.Set("offset", fmt.Sprintf("%d", offset))

		logParams := map[string]any{
			"offset":      offset,
			"sort":        sort,
			"query":       params.Get("query"),
			"group_id":    params.Get("group_id"),
			"uploader_id": params.Get("uploader_id"),
			"media_id":    params.Get("media_id"),
		}
		m.logFetch(prefix, page+1, logParams)

		torrents, err := scr.SearchTorrents(params)
		if err != nil {
			log.Printf("%s Search error: %v", prefix, err)
			return
		}

		if len(torrents) == 0 {
			break
		}

		for _, t := range torrents {
			commentCount := 0
			fmt.Sscanf(t.CommentCount, "%d", &commentCount)
			if sort == "comments" && commentCount == 0 {
				log.Printf(
					"%s Reached torrent with 0 comments (sorted by comments desc), breaking early.",
					prefix,
				)
				return
			}

			if m.isExcluded(t.Title, monitorCfg.Excludes) {
				continue
			}

			storedCount, exists := m.db.GetStoredCommentCount("nekobt", t.ID)

			if !exists || commentCount > storedCount {

				if !exists {
					log.Printf(
						"%s Found new torrent: %s (%s) with %d comments",
						prefix,
						t.ID,
						t.Title,
						commentCount,
					)
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
						var ts int64 = c.CreatedAt / 1000
						pfpHash := "null"
						if c.PfpHash != nil && *c.PfpHash != "" {
							pfpHash = *c.PfpHash
						}
						avatarURL := fmt.Sprintf("https://nekobt.to/cdn/pfp/%s", pfpHash)
						parentID := ""
						if c.ReplyingTo != nil {
							parentID = *c.ReplyingTo
						}
						m.db.StoreComment(
							"nekobt",
							t.ID,
							c.ID,
							c.DisplayName,
							c.Text,
							ts,
							0,
							"",
							avatarURL,
							parentID,
							c.ParentText,
						)

						m.enqueueAnnouncement(prefix, "nekobt", t.ID, c.ID, monitorCfg)
					}
				}
				m.db.UpdateTorrent("nekobt", t.ID, t.Title, len(comments), t.UploadedAt, "")
			}
		}
	}
}
