package monitor

import (
	"fmt"
	"log"
	"time"

	"github.com/geckyzz/contourgo/internal/scraper"
)

func (m *Monitor) checkAnirena(force bool) {
	cfg := m.Config()
	monitorMap, exists := cfg.Monitors["anirena"]
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
	apiKey := cfg.Config.Anirena.API.Key

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

		sort := cfg.ResolveAnirenaSort(monitorCfg)
		order := cfg.ResolveAnirenaOrder(monitorCfg)

		type target struct {
			user  string
			group string
			q     string
		}
		var targets []target

		for _, u := range monitorCfg.Uploaders {
			if u != "" {
				targets = append(targets, target{user: u})
			}
		}
		for _, g := range monitorCfg.Groups {
			if g != "" {
				targets = append(targets, target{group: g})
			}
		}
		for _, k := range monitorCfg.Keywords {
			if k != "" {
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
				m.logFetch(prefix, page, map[string]any{
					"user":  tRef.user,
					"group": tRef.group,
					"q":     tRef.q,
					"sort":  sort,
					"order": order,
				})
				torrents, totalPages, err := client.FetchTorrents(
					tRef.user,
					tRef.group,
					tRef.q,
					page,
					sort,
					order,
				)

				if err != nil {
					log.Printf("%s Error fetching torrents (page %d): %v", prefix, page, err)
					break
				}

				if len(torrents) == 0 {
					break
				}

				for _, t := range torrents {
					if sort == "comments" && order == "desc" && t.CommentCount == 0 {
						log.Printf(
							"%s Reached torrent with 0 comments (sorted by comments desc), breaking early.",
							prefix,
						)
						goto nextTarget
					}

					if m.isExcluded(t.FullTitle(), monitorCfg.Excludes) {
						continue
					}

					storedCount, exists := m.db.GetStoredCommentCount("anirena", t.ID)

					if !exists || t.CommentCount > storedCount {
						if !exists {
							log.Printf(
								"%s Found new torrent: %s (%d comments)",
								prefix,
								t.ID,
								t.CommentCount,
							)
						} else {
							log.Printf("%s Torrent %s has new comments: %d -> %d", prefix, t.ID, storedCount, t.CommentCount)
						}

						var torrentUploadedAt int64 = m.parseGenericDate(t.CreatedAt)
						m.db.UpdateTorrent(
							"anirena",
							t.ID,
							t.FullTitle(),
							t.CommentCount,
							torrentUploadedAt,
							t.Uploader,
						)

						comments, err := client.FetchComments(t.ID)
						if err != nil {
							log.Printf("%s Error fetching comments: %v", prefix, err)
							continue
						}

						for _, c := range comments {
							if !m.db.IsCommentStored("anirena", t.ID, c.ID) {
								var ts int64
								parsedTime, err := time.Parse("2006-01-02 15:04:05", c.CreatedAt)
								if err == nil {
									ts = parsedTime.Unix()
								} else {
									ts = time.Now().Unix()
								}
								m.db.StoreComment(
									"anirena",
									t.ID,
									c.ID,
									c.Username,
									c.Body,
									ts,
									0,
									c.Role,
									"",
									"",
									"",
								)

								m.enqueueAnnouncement(prefix, "anirena", t.ID, c.ID, monitorCfg)
							}
						}
						m.db.UpdateTorrent(
							"anirena",
							t.ID,
							t.FullTitle(),
							len(comments),
							torrentUploadedAt,
							t.Uploader,
						)
					}
				}

				maxPages := cfg.ResolveAnirenaPageMax(monitorCfg)
				if maxPages > 0 && page >= maxPages {
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
