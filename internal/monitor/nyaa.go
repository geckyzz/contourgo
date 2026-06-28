package monitor

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/geckyzz/contourgo/internal/scraper"
)

func (m *Monitor) checkNyaa(force bool, targetKey string) {
	m.checkNyaaSukebeiService("nyaa", force, targetKey)
}

func (m *Monitor) checkSukebei(force bool, targetKey string) {
	m.checkNyaaSukebeiService("sukebei", force, targetKey)
}

func (m *Monitor) checkNyaaSukebeiService(service string, force bool, targetKey string) {
	cfg := m.Config()
	monitorMap, exists := cfg.Monitors[service]
	if !exists || len(monitorMap) == 0 {
		return
	}

	prefix := "[NYAA]"
	if service == "sukebei" {
		prefix = "[SUKEBEI]"
	}

	proxyURL := cfg.Config.Nyaa.Proxy.URL
	if proxyURL == "" {
		log.Printf("%s Warning: Nyaa proxy URL not configured, skipping.", prefix)
		return
	}

	client := scraper.NewNyaaScraper(proxyURL, service)

	for key, monitorCfg := range monitorMap {
		if targetKey != "" && key != targetKey {
			continue
		}
		if !m.isDue(service, key, monitorCfg, force) {
			continue
		}
		m.updateLastCheck(service, key)

		monitorPrefix := fmt.Sprintf("%s[%s]", prefix, key)

		sort := cfg.ResolveNyaaSort(monitorCfg)
		order := cfg.ResolveNyaaOrder(monitorCfg)

		type target struct {
			user string
			q    string
		}
		var targets []target

		for _, u := range monitorCfg.Uploaders {
			if u != "" {
				targets = append(targets, target{user: u})
			}
		}
		for _, k := range monitorCfg.Keywords {
			if k != "" {
				targets = append(targets, target{q: k})
			}
		}

		if len(targets) == 0 {
			log.Printf("Monitor [%s.%s] has no keywords or uploaders, skipping.", service, key)
			continue
		}

		for _, tRef := range targets {
			page := 1
			for {
				m.logFetch(monitorPrefix, page, map[string]any{
					"keyword": tRef.q,
					"user":    tRef.user,
					"sort":    sort,
					"order":   order,
				})
				torrents, totalPages, err := client.FetchTorrents(
					tRef.user,
					tRef.q,
					page,
					sort,
					order,
				)
				if err != nil {
					log.Printf("%s Error fetching page %d: %v", monitorPrefix, page, err)
					break
				}

				if len(torrents) == 0 {
					break
				}

				for _, t := range torrents {
					if sort == "comments" && order == "desc" && t.Comments == 0 {
						log.Printf(
							"%s Reached torrent with 0 comments (sorted by comments desc), breaking early.",
							monitorPrefix,
						)
						goto nextTarget
					}

					if m.isExcluded(t.Name, monitorCfg.Excludes) {
						continue
					}

					torrentIDStr := strconv.Itoa(t.ID)
					storedCount, exists := m.db.GetStoredCommentCount(service, torrentIDStr)

					if !exists || t.Comments > storedCount {
						if !exists {
							log.Printf(
								"%s Found new torrent: %s (%d comments)",
								monitorPrefix,
								torrentIDStr,
								t.Comments,
							)
						} else {
							log.Printf("%s Torrent %s has new comments: %d -> %d", monitorPrefix, torrentIDStr, storedCount, t.Comments)
						}

						var torrentUploadedAt int64 = m.parseGenericDate(t.UploadDate)
						m.db.UpdateTorrent(
							service,
							torrentIDStr,
							t.Name,
							t.Comments,
							torrentUploadedAt,
							"",
						)

						var comments []scraper.NyaaComment
						comments, err = client.FetchComments(torrentIDStr)
						if err != nil {
							log.Printf("%s Error fetching comments: %v", monitorPrefix, err)
							continue
						}

						for idx, c := range comments {
							commentIDStr := strconv.Itoa(c.ID)
							if !m.db.IsCommentStored(service, torrentIDStr, commentIDStr) {
								var ts int64
								parsedTime, err := time.Parse(time.RFC3339, c.Timestamp)
								if err == nil {
									ts = parsedTime.Unix()
								} else {
									ts = time.Now().Unix()
								}

								parentID, parentMessage := scraper.ResolveNyaaParent(
									comments,
									idx,
									c.Text,
								)

								m.db.StoreComment(
									service,
									torrentIDStr,
									commentIDStr,
									c.Username,
									c.Text,
									ts,
									c.Pos,
									c.Role,
									c.Avatar,
									parentID,
									parentMessage,
								)

								m.enqueueAnnouncement(
									monitorPrefix,
									service,
									torrentIDStr,
									commentIDStr,
									monitorCfg,
								)
							}
						}

						m.db.UpdateTorrent(
							service,
							torrentIDStr,
							t.Name,
							len(comments),
							torrentUploadedAt,
							"",
						)
					}
				}

				maxPages := cfg.ResolveNyaaPageMax(monitorCfg)
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
