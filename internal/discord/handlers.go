package discord

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/crypto"
	"github.com/geckyzz/contourgo/internal/scraper"
)

type OldComment struct {
	ID        int   `json:"id"`
	Pos       int   `json:"pos"`
	Timestamp int64 `json:"timestamp"`
	User      struct {
		Username string `json:"username"`
		Image    string `json:"image"`
	} `json:"user"`
	Message string `json:"message"`
}

// Slash Command Handlers
func (b *DiscordBot) handleSlashStatus(s *discordgo.Session, i *discordgo.InteractionCreate) {
	torrents, comments, err := b.DB.GetStats()
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Error retrieving status: %v", err),
			},
		})
		return
	}

	totalMonitors := 0
	for _, sub := range b.Config.Monitors {
		totalMonitors += len(sub)
	}

	embed := &discordgo.MessageEmbed{
		Title: "📊 Bot Status",
		Color: 0x00FF00,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Active Monitors", Value: fmt.Sprintf("%d", totalMonitors), Inline: true},
			{Name: "Torrents Tracked", Value: fmt.Sprintf("%d", torrents), Inline: true},
			{Name: "Comments Stored", Value: fmt.Sprintf("%d", comments), Inline: true},
			{Name: "Check Interval", Value: b.Config.Config.Monitor.By, Inline: true},
			{Name: "SQLite Database", Value: "Connected (SQLite)", Inline: true},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	log.Printf("[POST] Responding to /status command with embed: Title: %q", embed.Title)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (b *DiscordBot) handleSlashReload(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var content string
	var color int
	select {
	case b.ForceCheckChan <- true:
		log.Printf("[POST] Responding to /reload command with content: %q", "Triggered manual checks...")
		content = "🔄 Triggered manual checks of all monitors."
		color = 0x00FF00
	default:
		log.Printf("[POST] Responding to /reload command with content: %q", "A check is already in progress...")
		content = "⚠️ A check is already in progress or queued."
		color = 0xFFA500
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{
				{
					Description: content,
					Color:       color,
				},
			},
		},
	})
}

func (b *DiscordBot) handleSlashRefresh(s *discordgo.Session, i *discordgo.InteractionCreate) {
	newCfg, err := config.LoadConfig(b.ConfigPath)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Description: fmt.Sprintf("❌ Error reloading config: %v", err),
						Color:       0xFF0000,
					},
				},
			},
		})
		return
	}

	// Update existing config in-place
	*b.Config = *newCfg

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{
				{
					Description: "✅ Configuration reloaded successfully.",
					Color:       0x00FF00,
				},
			},
		},
	})
}

func (b *DiscordBot) handleSlashStats(s *discordgo.Session, i *discordgo.InteractionCreate) {
	torrents, comments, err := b.DB.GetStats()
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Error retrieving statistics: %v", err),
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "📈 Detailed Statistics",
		Color: 0x3498db,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Total Torrents Tracked", Value: strconv.Itoa(torrents), Inline: true},
			{Name: "Total Comments Stored", Value: strconv.Itoa(comments), Inline: true},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (b *DiscordBot) handleSlashPing(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// 1. WebSocket Latency
	wsLatency := s.HeartbeatLatency()

	// 2. Bot Response Time (Interaction creation vs Now)
	// Snowflake ID bits [63-22] represent timestamp
	interactionID, _ := strconv.ParseUint(i.ID, 10, 64)
	interactionTime := time.UnixMilli(int64(interactionID>>22) + 1420070400000)
	botTime := time.Since(interactionTime)

	// 3. Database Read Time
	dbStart := time.Now()
	_, _, _ = b.DB.GetStats() // Simple read operation
	dbReadTime := time.Since(dbStart)

	embed := &discordgo.MessageEmbed{
		Title:       "Pong!",
		Description: "Here's the results from my tests!",
		Color:       0x3498db,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "🤝 Websocket API",
				Value:  fmt.Sprintf("`%.2f`ms\n-# *Discord's Websocket API latency*", float64(wsLatency.Microseconds())/1000),
				Inline: true,
			},
			{
				Name:   "🤖 Bot",
				Value:  fmt.Sprintf("`%.2f`ms\n-# *Compares interaction timestamp to current timestamp*", float64(botTime.Microseconds())/1000),
				Inline: true,
			},
			{
				Name:   "🔎 Database Read Time",
				Value:  fmt.Sprintf("`%.2f`ms\n-# *Read time for SQLite database*", float64(dbReadTime.Microseconds())/1000),
				Inline: true,
			},
			{
				Name:   "📅 Uptime",
				Value:  fmt.Sprintf("Bot has been running since <t:%d:R>", b.StartTime.Unix()),
				Inline: true,
			},
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (b *DiscordBot) handleSlashInfo(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	dbFile, err := os.Stat("bot.db")
	dbSize := "Unknown"
	if err == nil {
		dbSize = fmt.Sprintf("%.2f MB", float64(dbFile.Size())/1024/1024)
	}

	embed := &discordgo.MessageEmbed{
		Title: "🤖 Bot Information",
		Color: 0x9b59b6,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Memory Usage", Value: fmt.Sprintf("%.2f MB", float64(m.Alloc)/1024/1024), Inline: true},
			{Name: "System Memory", Value: fmt.Sprintf("%.2f MB", float64(m.Sys)/1024/1024), Inline: true},
			{Name: "Database Size", Value: dbSize, Inline: true},
			{Name: "Goroutines", Value: strconv.Itoa(runtime.NumGoroutine()), Inline: true},
			{Name: "Go Version", Value: runtime.Version(), Inline: true},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (b *DiscordBot) handleSlashLogs(s *discordgo.Session, i *discordgo.InteractionCreate, optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption) {
	lines := 20
	if opt, ok := optionMap["lines"]; ok {
		lines = int(opt.IntValue())
		if lines > 100 {
			lines = 100
		}
		if lines < 1 {
			lines = 1
		}
	}

	file, err := os.Open("bot.log")
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Description: fmt.Sprintf("❌ Error opening log file: %v", err),
						Color:       0xFF0000,
					},
				},
			},
		})
		return
	}
	defer file.Close()

	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	start := len(allLines) - lines
	if start < 0 {
		start = 0
	}

	recentLogs := strings.Join(allLines[start:], "\n")
	if len(recentLogs) > 1900 {
		recentLogs = recentLogs[len(recentLogs)-1900:]
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("📋 **Recent Logs (last %d lines):**\n```\n%s\n```", lines, recentLogs),
		},
	})
}

func (b *DiscordBot) handleSlashLatest(s *discordgo.Session, i *discordgo.InteractionCreate, optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption) {
	limit := 10
	if opt, ok := optionMap["limit"]; ok {
		limit = int(opt.IntValue())
		if limit > 50 {
			limit = 50
		}
		if limit < 1 {
			limit = 1
		}
	}

	torrents, err := b.DB.GetLatestTorrents(limit)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Description: fmt.Sprintf("❌ Error fetching latest torrents: %v", err),
						Color:       0xFF0000,
					},
				},
			},
		})
		return
	}

	if len(torrents) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Description: "📭 No torrents tracked yet.",
						Color:       0xFFA500,
					},
				},
			},
		})
		return
	}

	var sb strings.Builder
	for _, t := range torrents {
		sb.WriteString(fmt.Sprintf("- [%s] **%s** (%s)\n", t.Service, t.Title, t.LastScrapedAt.Format("2006-01-02 15:04")))
	}

	description := sb.String()
	if len(description) > 4096 {
		description = description[:4093] + "..."
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       fmt.Sprintf("🆕 Latest %d Tracked Torrents", len(torrents)),
					Description: description,
					Color:       0x2ecc71,
				},
			},
		},
	})
}

func (b *DiscordBot) handleSlashMonitors(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title: "🌐 Configured Monitors",
		Color: 0x3498db,
	}

	if len(b.Config.Monitors) == 0 {
		embed.Description = "No active monitors configured."
	} else {
		for service, innerMap := range b.Config.Monitors {
			var sb strings.Builder
			for key, mc := range innerMap {
				var lines []string

				if len(mc.Keywords) > 0 {
					lines = append(lines, fmt.Sprintf("  - Keywords: `%s`", strings.Join(mc.Keywords, ", ")))
				}
				if len(mc.Excludes) > 0 {
					lines = append(lines, fmt.Sprintf("  - Excludes: `%s`", strings.Join(mc.Excludes, ", ")))
				}
				if len(mc.Uploaders) > 0 {
					lines = append(lines, fmt.Sprintf("  - Uploaders: `%s`", strings.Join(mc.Uploaders, ", ")))
				}

				// Platform specific
				if service == "nekobt" {
					if len(mc.Groups) > 0 {
						lines = append(lines, fmt.Sprintf("  - Groups: `%s`", strings.Join(mc.Groups, ", ")))
					}
					if len(mc.Media) > 0 {
						lines = append(lines, fmt.Sprintf("  - Media: `%s`", strings.Join(mc.Media, ", ")))
					}
				}

				sortInfo := "Default"
				if mc.Sort != "" {
					sortInfo = mc.Sort
					if mc.Order != "" {
						sortInfo += " (" + mc.Order + ")"
					}
				}
				lines = append(lines, fmt.Sprintf("  - Sort: `%s`", sortInfo))
				lines = append(lines, fmt.Sprintf("  - Max Pages: `%d`", mc.Page.Max))

				entry := fmt.Sprintf("🔹 **%s**:\n%s\n", key, strings.Join(lines, "\n"))

				if sb.Len()+len(entry) > 1024 {
					embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
						Name:   strings.ToUpper(service) + " (cont.)",
						Value:  sb.String(),
						Inline: true,
					})
					sb.Reset()
				}
				sb.WriteString(entry)
			}

			if sb.Len() > 0 {
				embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
					Name:   strings.ToUpper(service),
					Value:  sb.String(),
					Inline: true,
				})
			}
		}
	}

	log.Printf("[POST] Responding to /monitors command with embed summary")
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (b *DiscordBot) handleSlashImport(s *discordgo.Session, i *discordgo.InteractionCreate, optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption) {
	log.Printf("[POST] Sending deferment response for /import command")
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	fileOpt, exists := optionMap["file"]
	if !exists {
		b.sendFollowupMessage(s, i.Interaction, "❌ Attachment file is required.")
		return
	}

	attachmentID := fileOpt.Value.(string)
	var attachment *discordgo.MessageAttachment
	for _, att := range i.ApplicationCommandData().Resolved.Attachments {
		if att.ID == attachmentID {
			attachment = att
			break
		}
	}

	if attachment == nil {
		b.sendFollowupMessage(s, i.Interaction, "❌ Failed to resolve attachment.")
		return
	}

	var key string
	if keyOpt, exists := optionMap["key"]; exists {
		key = keyOpt.StringValue()
	}

	var service string
	if serviceOpt, exists := optionMap["service"]; exists {
		service = serviceOpt.StringValue()
	}

	if service == "" {
		lowerName := strings.ToLower(attachment.Filename)
		if strings.Contains(lowerName, "sukebei") {
			service = "sukebei"
		} else if strings.Contains(lowerName, "at.") || strings.Contains(lowerName, "animetosho") {
			service = "animetosho"
		} else {
			service = "nyaa"
		}
	}

	b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("📥 Downloading legacy database dump `%s`...", attachment.Filename))

	resp, err := http.Get(attachment.URL)
	if err != nil {
		b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Failed to download attachment: %v", err))
		return
	}
	defer resp.Body.Close()

	fileData, err := io.ReadAll(resp.Body)
	if err != nil {
		b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Failed to read attachment content: %v", err))
		return
	}

	var jsonBytes []byte
	if key != "" {
		b.sendFollowupMessage(s, i.Interaction, "🔑 Decrypting data...")
		jsonBytes, err = crypto.DecryptAndDecompress(fileData, key)
		if err != nil {
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Decryption/Decompression failed: %v. Check key.", err))
			return
		}
	} else {
		jsonBytes = fileData
	}

	var oldData map[string][]OldComment
	if err := json.Unmarshal(jsonBytes, &oldData); err != nil {
		b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Failed to parse JSON database structure: %v", err))
		return
	}

	b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("⚙️ Importing data for service `%s`...", service))

	importedTorrents := 0
	importedComments := 0

	for torrentID, comments := range oldData {
		count := len(comments)
		title := fmt.Sprintf("Imported Torrent %s", torrentID)

		err = b.DB.UpdateTorrent(service, torrentID, title, count)
		if err != nil {
			continue
		}
		importedTorrents++

		for _, c := range comments {
			commentIDStr := strconv.Itoa(c.ID)
			b.DB.StoreComment(service, torrentID, commentIDStr, c.User.Username, c.Message, c.Timestamp, c.Pos, "", c.User.Image)
			importedComments++
		}
	}

	b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("✅ **Import Completed!** Imported **%d** torrents and **%d** comments for service `%s`.", importedTorrents, importedComments, service))
}

func (b *DiscordBot) handleSlashTest(s *discordgo.Session, i *discordgo.InteractionCreate, optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption) {
	log.Printf("[POST] Sending deferment response for /test command")
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	service := optionMap["service"].StringValue()
	query := optionMap["query"].StringValue()

	pageMax := 1
	if opt, ok := optionMap["page_max"]; ok {
		pageMax = int(opt.IntValue())
		if pageMax < 0 {
			pageMax = 0
		}
	}

	inspect := "result"
	if opt, ok := optionMap["inspect"]; ok {
		inspect = opt.StringValue()
	}

	b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("🔍 Testing query `%s` on `%s` (page_max=%d, inspect=%s)...", query, service, pageMax, inspect))

	if service == "nyaa" || service == "sukebei" {
		proxyURL := b.Config.Config.Nyaa.Proxy.URL
		if proxyURL == "" {
			b.sendFollowupMessage(s, i.Interaction, "❌ Nyaa proxy URL is not configured.")
			return
		}

		var username string
		var keyword string
		if strings.HasPrefix(query, "@") {
			username = strings.TrimPrefix(query, "@")
		} else {
			keyword = query
		}

		var allTorrents []scraper.NyaaTorrent
		var err error
		var totalPages int

		if service == "nyaa" {
			client := scraper.NewNyaaScraper(proxyURL)
			for p := 1; ; p++ {
				log.Printf("Fetching page %d for service nyaa (query: %s, user: %s)", p, keyword, username)
				torrents, pages, fetchErr := client.FetchTorrents(username, keyword, p, "id", "desc")
				if fetchErr != nil {
					b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error querying API on page %d: %v", p, fetchErr))
					return
				}
				allTorrents = append(allTorrents, torrents...)
				totalPages = pages
				if pageMax > 0 && p >= pageMax {
					break
				}
				if p >= totalPages {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
		} else {
			client := scraper.NewSukebeiScraper(proxyURL)
			for p := 1; ; p++ {
				log.Printf("Fetching page %d for service sukebei (query: %s, user: %s)", p, keyword, username)
				torrents, pages, fetchErr := client.FetchTorrents(username, keyword, p, "id", "desc")
				if fetchErr != nil {
					b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error querying API on page %d: %v", p, fetchErr))
					return
				}
				allTorrents = append(allTorrents, torrents...)
				totalPages = pages
				if pageMax > 0 && p >= pageMax {
					break
				}
				if p >= totalPages {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
		}

		if len(allTorrents) == 0 {
			b.sendFollowupMessage(s, i.Interaction, "📭 No torrents found matching query.")
			return
		}

		if inspect == "raw" {
			limit := len(allTorrents)
			if limit > 3 {
				limit = 3
			}
			jsonData, _ := json.MarshalIndent(allTorrents[:limit], "", "  ")
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("📋 **Raw Torrents JSON:**\n```json\n%s\n```", string(jsonData)))
			return
		}

		if inspect == "comments" {
			// Find the first torrent that has comments count > 0
			var targetTorrent scraper.NyaaTorrent
			found := false
			for _, t := range allTorrents {
				if t.Comments > 0 {
					targetTorrent = t
					found = true
					break
				}
			}
			if !found {
				// Fallback to first torrent if none have comments
				targetTorrent = allTorrents[0]
			}

			torrentIDStr := strconv.Itoa(targetTorrent.ID)
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("💬 Fetching comments for torrent: **%s** (ID: %s)...", targetTorrent.Name, torrentIDStr))

			var comments []scraper.NyaaComment
			if service == "nyaa" {
				client := scraper.NewNyaaScraper(proxyURL)
				comments, err = client.FetchComments(torrentIDStr)
			} else {
				client := scraper.NewSukebeiScraper(proxyURL)
				comments, err = client.FetchComments(torrentIDStr)
			}
			if err != nil {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error fetching comments: %v", err))
				return
			}

			if len(comments) == 0 {
				b.sendFollowupMessage(s, i.Interaction, "📭 No comments found on this torrent.")
				return
			}

			firstComment := comments[0]
			err := b.AnnounceNyaaComment(i.ChannelID, service, torrentIDStr, targetTorrent.Name, firstComment, "")
			if err != nil {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error creating test embed: %v", err))
			} else {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("✅ Test successful! Sent most recent comment from **%s**.", targetTorrent.Name))
			}
			return
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("✅ **Query Results (Service: %s):**\n", service))
		limit := len(allTorrents)
		if limit > 5 {
			limit = 5
		}
		for idx := 0; idx < limit; idx++ {
			t := allTorrents[idx]
			sb.WriteString(fmt.Sprintf("- **%s** (ID: %d)\n  - Comments: `%d` | Seeders: `%d` | Size: `%s` | Uploaded: `%s`\n",
				t.Name, t.ID, t.Comments, t.Seeders, t.Size, t.UploadDate))
		}
		b.sendFollowupMessage(s, i.Interaction, sb.String())

	} else if service == "animetosho_old" || service == "animetosho_new" {
		var allComments []scraper.ATComment

		if service == "animetosho_old" {
			client := scraper.NewAnimeToshoOldScraper()
			for p := 1; ; p++ {
				log.Printf("Fetching page %d for service animetosho_old", p)
				comments, hasNext, scrapeErr := client.ScrapeComments(p)
				if scrapeErr != nil {
					b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error scraping AnimeTosho page %d: %v", p, scrapeErr))
					return
				}
				allComments = append(allComments, comments...)
				if !hasNext || (pageMax > 0 && p >= pageMax) {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
		} else {
			client := scraper.NewAnimeToshoNewScraper()
			for p := 1; ; p++ {
				log.Printf("Fetching page %d for service animetosho_new (query: %q)", p, query)
				comments, hasNext, scrapeErr := client.ScrapeComments(p, query)
				if scrapeErr != nil {
					b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error scraping AnimeTosho page %d: %v", p, scrapeErr))
					return
				}
				allComments = append(allComments, comments...)
				if !hasNext || (pageMax > 0 && p >= pageMax) {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
		}

		// Filter in-memory as backup, although search query was already sent to API
		var matchingComments []scraper.ATComment
		for _, c := range allComments {
			if strings.Contains(strings.ToLower(c.Title), strings.ToLower(query)) || strings.Contains(strings.ToLower(c.Message), strings.ToLower(query)) {
				matchingComments = append(matchingComments, c)
			}
		}

		if len(matchingComments) == 0 {
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("📭 No matching comments found for query: `%s`", query))
			return
		}

		if inspect == "raw" {
			limit := len(matchingComments)
			if limit > 3 {
				limit = 3
			}
			jsonData, _ := json.MarshalIndent(matchingComments[:limit], "", "  ")
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("📋 **Raw Comments JSON:**\n```json\n%s\n```", string(jsonData)))
			return
		}

		if inspect == "comments" {
			firstComment := matchingComments[0]
			err := b.AnnounceATComment(i.ChannelID, service, firstComment.TorrentID, firstComment.Title, firstComment, "")
			if err != nil {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error creating test embed: %v", err))
			} else {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("✅ Test successful! Sent most recent comment from **%s**.", firstComment.Title))
			}
			return
		}

		var sb strings.Builder
		sb.WriteString("✅ **AnimeTosho Matching Recent Comments:**\n")
		limit := len(matchingComments)
		if limit > 5 {
			limit = 5
		}
		for idx := 0; idx < limit; idx++ {
			c := matchingComments[idx]
			sb.WriteString(fmt.Sprintf("- **Torrent**: %s\n  - Comment by **%s** (ID: %s)\n  - Message: `%s`\n",
				c.Title, c.Username, c.ID, c.Message))
		}
		b.sendFollowupMessage(s, i.Interaction, sb.String())
	} else if service == "anirena" {
		apiKey := b.Config.Config.Anirena.API.Key

		if apiKey == "" {
			b.sendFollowupMessage(s, i.Interaction, "❌ AniRena API Key is not configured.")
			return
		}

		client := scraper.NewAnirenaScraper(apiKey)

		var usernameFilter string
		var groupFilter string
		var keywordFilter string
		if strings.HasPrefix(query, "@") {
			usernameFilter = strings.TrimPrefix(query, "@")
		} else if strings.HasPrefix(query, "g:") {
			groupFilter = strings.TrimPrefix(query, "g:")
		} else if strings.HasPrefix(query, "group:") {
			groupFilter = strings.TrimPrefix(query, "group:")
		} else {
			keywordFilter = query
		}

		var allTorrents []scraper.AnirenaTorrent
		var totalPages int

		for p := 1; ; p++ {
			log.Printf("Fetching page %d for service anirena (query: %s, user: %s, group: %s)", p, keywordFilter, usernameFilter, groupFilter)
			torrents, pages, fetchErr := client.FetchTorrents(usernameFilter, groupFilter, keywordFilter, p, "date", "desc")
			if fetchErr != nil {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error querying AniRena API on page %d: %v", p, fetchErr))
				return
			}
			allTorrents = append(allTorrents, torrents...)
			totalPages = pages
			if pageMax > 0 && p >= pageMax {
				break
			}
			if p >= totalPages {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		if len(allTorrents) == 0 {
			b.sendFollowupMessage(s, i.Interaction, "📭 No torrents found matching query.")
			return
		}

		if inspect == "raw" {
			limit := len(allTorrents)
			if limit > 3 {
				limit = 3
			}
			jsonData, _ := json.MarshalIndent(allTorrents[:limit], "", "  ")
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("📋 **Raw AniRena Torrents JSON:**\n```json\n%s\n```", string(jsonData)))
			return
		}

		if inspect == "comments" {
			var targetTorrent scraper.AnirenaTorrent
			found := false
			for _, t := range allTorrents {
				if t.CommentCount > 0 {
					targetTorrent = t
					found = true
					break
				}
			}
			if !found {
				targetTorrent = allTorrents[0]
			}

			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("💬 Fetching comments for torrent: **%s** (ID: %s)...", targetTorrent.FullTitle(), targetTorrent.ID))

			comments, err := client.FetchComments(targetTorrent.ID)
			if err != nil {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error fetching comments: %v", err))
				return
			}

			if len(comments) == 0 {
				b.sendFollowupMessage(s, i.Interaction, "📭 No comments found on this torrent.")
				return
			}

			firstComment := comments[0]
			err = b.AnnounceAnirenaComment(i.ChannelID, targetTorrent.ID, targetTorrent.FullTitle(), firstComment, "")
			if err != nil {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error creating test embed: %v", err))
			} else {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("✅ Test successful! Sent most recent comment from **%s**.", targetTorrent.FullTitle()))
			}
			return
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("✅ **Query Results (Service: %s):**\n", service))
		limit := len(allTorrents)
		if limit > 5 {
			limit = 5
		}
		for idx := 0; idx < limit; idx++ {
			t := allTorrents[idx]
			sb.WriteString(fmt.Sprintf("- **%s** (ID: %s)\n  - Comments: `%d` | Seeders: `%d` | Size: `%s` | Uploaded: `%s`\n",
				t.FullTitle(), t.ID, t.CommentCount, t.Seeders, t.SizeFmt, t.CreatedAt))
		}
		b.sendFollowupMessage(s, i.Interaction, sb.String())
	} else if service == "nekobt" {
		apiKey := b.Config.Config.Nekobt.API.Key
		scr := scraper.NewNekoBTScraper(apiKey)
		params := url.Values{}
		params.Set("sort_by", "latest")

		// Map query to various possible NekoBT filters for testing
		if strings.HasPrefix(query, "g:") {
			params.Set("group_id", strings.TrimPrefix(query, "g:"))
		} else if strings.HasPrefix(query, "u:") {
			params.Set("uploader_id", strings.TrimPrefix(query, "u:"))
		} else if strings.HasPrefix(query, "m:") {
			mid := strings.TrimPrefix(query, "m:")
			if strings.HasPrefix(mid, "tmdb:") {
				params.Set("tmdbid", strings.TrimPrefix(mid, "tmdb:"))
			} else if strings.HasPrefix(mid, "tvdb:") {
				params.Set("tvdbid", strings.TrimPrefix(mid, "tvdb:"))
			} else {
				params.Set("media_id", mid)
			}
		} else {
			params.Set("query", query)
		}

		torrents, err := scr.SearchTorrents(params)
		if err != nil {
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ NekoBT Search Error: %v", err))
			return
		}

		if len(torrents) == 0 {
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("📭 No torrents found for criteria: `%s`", query))
			return
		}

		firstTorrent := torrents[0]
		comments, err := scr.FetchComments(firstTorrent.ID, firstTorrent.Title)
		if err != nil {
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error fetching comments for torrent %s: %v", firstTorrent.ID, err))
			return
		}

		if len(comments) == 0 {
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("✅ Found torrent: **%s**, but it has no comments yet.", firstTorrent.Title))
			return
		}

		if inspect == "raw" {
			limit := len(comments)
			if limit > 3 {
				limit = 3
			}
			jsonData, _ := json.MarshalIndent(comments[:limit], "", "  ")
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("📋 **Raw NekoBT Comments JSON:**\n```json\n%s\n```", string(jsonData)))
			return
		}

		if inspect == "comments" || inspect == "result" {
			// Show the most recent comment in an embed (using the announcer logic)
			lastComment := comments[0]
			err := b.AnnounceNekoBTComment(i.ChannelID, firstTorrent.Title, lastComment, "")
			if err != nil {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error creating test embed: %v", err))
			} else {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("✅ Test successful! Sent most recent comment from **%s**.", firstTorrent.Title))
			}
			return
		}
	}
}

func (b *DiscordBot) handleSlashHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var sb strings.Builder
	for _, cmd := range b.Commands {
		if cmd.ID != "" {
			sb.WriteString(fmt.Sprintf("- </%s:%s> - %s\n", cmd.Name, cmd.ID, cmd.Description))
		} else {
			sb.WriteString(fmt.Sprintf("- `/%s` - %s\n", cmd.Name, cmd.Description))
		}
	}

	helpMsg := sb.String()
	if helpMsg == "" {
		helpMsg = "No commands available."
	}

	log.Printf("[POST] Responding to /help command with adaptive embed help menu")
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       "📜 Available Slash Commands",
					Description: helpMsg,
					Color:       0xf1c40f,
				},
			},
		},
	})
}
