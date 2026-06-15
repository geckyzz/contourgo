package discord

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/geckyzz/contourgo/internal/db"
	"github.com/geckyzz/contourgo/internal/scraper"
)

func (b *DiscordBot) handleSlashTest(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	log.Printf("[POST] Sending deferment response for /test command")
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	service := optionMap["service"].StringValue()
	query := optionMap["query"].StringValue()

	pageMax := 1
	if opt, ok := optionMap["page_max"]; ok {
		pageMax = max(int(opt.IntValue()), 0)
	}

	inspect := "result"
	if opt, ok := optionMap["inspect"]; ok {
		inspect = opt.StringValue()
	}

	b.sendFollowupMessage(
		s,
		i.Interaction,
		fmt.Sprintf(
			"🔍 Testing query `%s` on `%s` (page_max=%d, inspect=%s)...",
			query,
			service,
			pageMax,
			inspect,
		),
	)

	if service == "nyaa" || service == "sukebei" {
		proxyURL := b.Config.Config.Nyaa.Proxy.URL
		if proxyURL == "" {
			b.sendFollowupMessage(s, i.Interaction, "❌ Nyaa proxy URL is not configured.")
			return
		}

		var username string
		var keyword string
		if after, ok := strings.CutPrefix(query, "@"); ok {
			username = after
		} else {
			keyword = query
		}

		var allTorrents []scraper.NyaaTorrent
		var err error
		var totalPages int

		client := scraper.NewNyaaScraper(proxyURL, service)
		for p := 1; ; p++ {
			log.Printf(
				"[%s] Fetching page %d (user: %q, q: %q)",
				strings.ToUpper(service),
				p,
				username,
				keyword,
			)
			torrents, pages, fetchErr := client.FetchTorrents(username, keyword, p, "id", "desc")
			if fetchErr != nil {
				b.sendFollowupMessage(
					s,
					i.Interaction,
					fmt.Sprintf("❌ Error querying API on page %d: %v", p, fetchErr),
				)
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
			limit := min(len(allTorrents), 3)
			jsonData, _ := json.MarshalIndent(allTorrents[:limit], "", "  ")
			b.sendFollowupMessage(
				s,
				i.Interaction,
				fmt.Sprintf("📋 **Raw Torrents JSON:**\n```json\n%s\n```", string(jsonData)),
			)
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
			b.sendFollowupMessage(
				s,
				i.Interaction,
				fmt.Sprintf(
					"💬 Fetching comments for torrent: **%s** (ID: %s)...",
					targetTorrent.Name,
					torrentIDStr,
				),
			)

			var comments []scraper.NyaaComment
			client := scraper.NewNyaaScraper(proxyURL, service)
			comments, err = client.FetchComments(torrentIDStr)
			if err != nil {
				b.sendFollowupMessage(
					s,
					i.Interaction,
					fmt.Sprintf("❌ Error fetching comments: %v", err),
				)
				return
			}

			if len(comments) == 0 {
				b.sendFollowupMessage(s, i.Interaction, "📭 No comments found on this torrent.")
				return
			}

			firstComment := comments[0]
			var ts int64
			parsedTime, err := time.Parse(time.RFC3339, firstComment.Timestamp)
			if err == nil {
				ts = parsedTime.Unix()
			} else {
				ts = time.Now().Unix()
			}

			dbTorrent := db.Torrent{
				Service:   service,
				TorrentID: torrentIDStr,
				Title:     targetTorrent.Name,
			}
			dbComment := db.Comment{
				Service:   service,
				TorrentID: torrentIDStr,
				CommentID: strconv.Itoa(firstComment.ID),
				Username:  firstComment.Username,
				Message:   firstComment.Text,
				Timestamp: ts,
				Position:  firstComment.Pos,
				UserRole:  firstComment.Role,
				AvatarURL: firstComment.Avatar,
			}

			err = b.AnnounceNyaaComment(
				i.ChannelID,
				service,
				dbTorrent,
				dbComment,
				"",
				false,
				false,
			)
			if err != nil {
				b.sendFollowupMessage(
					s,
					i.Interaction,
					fmt.Sprintf("❌ Error creating test embed: %v", err),
				)
			} else {
				mentions := b.ResolveMentionsPlain(dbComment.Message)
				msg := fmt.Sprintf("✅ Test successful! Sent most recent comment from **%s**.", targetTorrent.Name)
				if mentions != "" {
					msg += fmt.Sprintf(" Mentions found: `%s`", mentions)
				}
				b.sendFollowupMessage(s, i.Interaction, msg)
			}
			return
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("✅ **Query Results (Service: %s):**\n", service))
		limit := min(len(allTorrents), 5)
		for idx := range limit {
			t := allTorrents[idx]
			sb.WriteString(
				fmt.Sprintf(
					"- **%s** (ID: %d)\n  - Comments: `%d` | Seeders: `%d` | Size: `%s` | Uploaded: `%s`\n",
					t.Name,
					t.ID,
					t.Comments,
					t.Seeders,
					t.Size,
					t.UploadDate,
				),
			)
		}
		b.sendFollowupMessage(s, i.Interaction, sb.String())

	} else if service == "animetosho_old" || service == "animetosho_new" || service == "animetosho_old_feedback" || service == "animetosho_new_feedback" {
		var allComments []scraper.ATComment
		isFeedback := strings.HasSuffix(service, "_feedback")
		actualService := strings.TrimSuffix(service, "_feedback")

		if actualService == "animetosho_old" {
			client := scraper.NewAnimeToshoOldScraper()
			for p := 1; ; p++ {
				if isFeedback {
					log.Printf("[%s][FEEDBACK] Fetching global feedback comments feed (page %d)", strings.ToUpper(actualService), p)
				} else {
					log.Printf("[%s] Fetching global comments feed (page %d)", strings.ToUpper(actualService), p)
				}
				comments, hasNext, scrapeErr := client.ScrapeComments(p, isFeedback)
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
				if isFeedback {
					log.Printf("[%s][FEEDBACK] Fetching global feedback comments feed (page %d)", strings.ToUpper(actualService), p)
				} else {
					if query == "" {
						log.Printf("[%s] Fetching global comments feed (page %d)", strings.ToUpper(actualService), p)
					} else {
						log.Printf("[%s] Fetching comments feed for query %q (page %d)", strings.ToUpper(actualService), query, p)
					}
				}
				comments, hasNext, scrapeErr := client.ScrapeComments(p, query, isFeedback)
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
			limit := min(len(matchingComments), 3)
			jsonData, _ := json.MarshalIndent(matchingComments[:limit], "", "  ")
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("📋 **Raw Comments JSON:**\n```json\n%s\n```", string(jsonData)))
			return
		}

		if inspect == "comments" {
			firstComment := matchingComments[0]
			dbTorrent := db.Torrent{Service: service, TorrentID: firstComment.TorrentID, Title: firstComment.Title}
			dbComment := db.Comment{Service: service, TorrentID: firstComment.TorrentID, CommentID: firstComment.ID, Username: firstComment.Username, Message: firstComment.Message, Timestamp: firstComment.Timestamp}

			err := b.AnnounceATComment(i.ChannelID, service, dbTorrent, dbComment, "", false, false)
			if err != nil {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error creating test embed: %v", err))
			} else {
				mentions := b.ResolveMentionsPlain(dbComment.Message)
				msg := fmt.Sprintf("✅ Test successful! Sent most recent comment from **%s**.", firstComment.Title)
				if mentions != "" {
					msg += fmt.Sprintf(" Mentions found: `%s`", mentions)
				}
				b.sendFollowupMessage(s, i.Interaction, msg)
			}
			return
		}

		var sb strings.Builder
		sb.WriteString("✅ **AnimeTosho Matching Recent Comments:**\n")
		limit := min(len(matchingComments), 5)
		for idx := range limit {
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
		if after, ok := strings.CutPrefix(query, "@"); ok {
			usernameFilter = after
		} else if after, ok := strings.CutPrefix(query, "g:"); ok {
			groupFilter = after
		} else if after, ok := strings.CutPrefix(query, "group:"); ok {
			groupFilter = after
		} else {
			keywordFilter = query
		}

		var allTorrents []scraper.AnirenaTorrent
		var totalPages int

		for p := 1; ; p++ {
			log.Printf("[ANIRENA] Fetching page %d (user: %q, group: %q, q: %q)", p, usernameFilter, groupFilter, keywordFilter)
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
			limit := min(len(allTorrents), 3)
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
			var ts int64
			parsedTime, err := time.Parse("2006-01-02 15:04:05", firstComment.CreatedAt)
			if err == nil {
				ts = parsedTime.Unix()
			} else {
				ts = time.Now().Unix()
			}

			dbTorrent := db.Torrent{Service: "anirena", TorrentID: targetTorrent.ID, Title: targetTorrent.FullTitle(), Uploader: targetTorrent.Uploader}
			dbComment := db.Comment{Service: "anirena", TorrentID: targetTorrent.ID, CommentID: firstComment.ID, Username: firstComment.Username, Message: firstComment.Body, Timestamp: ts, UserRole: firstComment.Role}

			err = b.AnnounceAnirenaComment(i.Interaction.ChannelID, dbTorrent, dbComment, "", false, false)
			if err != nil {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error creating test embed: %v", err))
			} else {
				mentions := b.ResolveMentionsPlain(dbComment.Message)
				msg := fmt.Sprintf("✅ Test successful! Sent most recent comment from **%s**.", targetTorrent.FullTitle())
				if mentions != "" {
					msg += fmt.Sprintf(" Mentions found: `%s`", mentions)
				}
				b.sendFollowupMessage(s, i.Interaction, msg)
			}
			return
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("✅ **Query Results (Service: %s):**\n", service))
		limit := min(len(allTorrents), 5)
		for idx := range limit {
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
		if after, ok := strings.CutPrefix(query, "g:"); ok {
			params.Set("group_id", after)
		} else if after, ok := strings.CutPrefix(query, "u:"); ok {
			params.Set("uploader_id", after)
		} else if after, ok := strings.CutPrefix(query, "m:"); ok {
			mid := after
			if after, ok := strings.CutPrefix(mid, "tmdb:"); ok {
				params.Set("tmdbid", after)
			} else if after, ok := strings.CutPrefix(mid, "tvdb:"); ok {
				params.Set("tvdbid", after)
			} else {
				params.Set("media_id", mid)
			}
		} else {
			params.Set("query", query)
		}

		torrents, err := scr.SearchTorrents(params)
		if err != nil {
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ nekoBT Search Error: %v", err))
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
			limit := min(len(comments), 3)
			jsonData, _ := json.MarshalIndent(comments[:limit], "", "  ")
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("📋 **Raw nekoBT Comments JSON:**\n```json\n%s\n```", string(jsonData)))
			return
		}

		if inspect == "comments" || inspect == "result" {
			// Show the most recent comment in an embed (using the announcer logic)
			lastComment := comments[0]

			dbTorrent := db.Torrent{Service: "nekobt", TorrentID: firstTorrent.ID, Title: firstTorrent.Title, UploadedAt: firstTorrent.UploadedAt / 1000}

			pfpHash := "null"
			if lastComment.PfpHash != nil && *lastComment.PfpHash != "" {
				pfpHash = *lastComment.PfpHash
			}
			avatarURL := fmt.Sprintf("https://nekobt.to/cdn/pfp/%s", pfpHash)
			parentID := ""
			if lastComment.ReplyingTo != nil {
				parentID = *lastComment.ReplyingTo
			}
			dbComment := db.Comment{Service: "nekobt", TorrentID: firstTorrent.ID, CommentID: lastComment.ID, Username: lastComment.DisplayName, Message: lastComment.Text, Timestamp: lastComment.CreatedAt / 1000, AvatarURL: avatarURL, ParentID: parentID, ParentMessage: lastComment.ParentText}

			err := b.AnnounceNekoBTComment(i.ChannelID, dbTorrent, dbComment, "", false, false)
			if err != nil {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error creating test embed: %v", err))
			} else {
				mentions := b.ResolveMentionsPlain(dbComment.Message)
				msg := fmt.Sprintf("✅ Test successful! Sent most recent comment from **%s**.", firstTorrent.Title)
				if mentions != "" {
					msg += fmt.Sprintf(" Mentions found: `%s`", mentions)
				}
				b.sendFollowupMessage(s, i.Interaction, msg)
			}
			return
		}
	} else if service == "tsukihime" {
		scr := scraper.NewTsukihimeScraper()
		torrents, err := scr.SearchTorrents(query, 100, 0)
		if err != nil {
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ TsukiHime Search Error: %v", err))
			return
		}

		if len(torrents) == 0 {
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("📭 No torrents found for query: `%s`", query))
			return
		}

		if inspect == "raw" {
			limit := min(len(torrents), 3)
			jsonData, _ := json.MarshalIndent(torrents[:limit], "", "  ")
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("📋 **Raw TsukiHime Torrents JSON:**\n```json\n%s\n```", string(jsonData)))
			return
		}

		if inspect == "comments" {
			firstTorrent := torrents[0]
			torrentIDStr := strconv.Itoa(firstTorrent.ID)
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("💬 Fetching comments for torrent: **%s** (ID: %s)...", firstTorrent.Name, torrentIDStr))

			comments, err := scr.FetchComments(torrentIDStr, firstTorrent.Name)
			if err != nil {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error fetching comments: %v", err))
				return
			}

			if len(comments) == 0 {
				b.sendFollowupMessage(s, i.Interaction, "📭 No comments found on this torrent (within latest 100).")
				return
			}

			firstComment := comments[0]
			var ts int64
			parsedTime, err := time.Parse(time.RFC3339, firstComment.CreatedAt)
			if err == nil {
				ts = parsedTime.Unix()
			} else {
				ts = time.Now().Unix()
			}

			dbTorrent := db.Torrent{Service: "tsukihime", TorrentID: torrentIDStr, Title: firstTorrent.Name}

			avatarURL := ""
			if firstComment.Author != nil && firstComment.Author.AvatarHash != "" {
				avatarURL = fmt.Sprintf("https://tsukihime.org/cdn/pfp/%s", firstComment.Author.AvatarHash)
			}
			dbComment := db.Comment{Service: "tsukihime", TorrentID: torrentIDStr, CommentID: firstComment.GetID(), Username: firstComment.GetDisplayName(), Message: firstComment.GetText(), Timestamp: ts, AvatarURL: avatarURL, ParentID: firstComment.GetParentID(), ParentMessage: firstComment.ParentText}

			err = b.AnnounceTsukihimeComment(i.ChannelID, dbTorrent, dbComment, "", false, false)
			if err != nil {
				b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error creating test embed: %v", err))
			} else {
				mentions := b.ResolveMentionsPlain(dbComment.Message)
				msg := fmt.Sprintf("✅ Test successful! Sent most recent comment from **%s**.", firstTorrent.Name)
				if mentions != "" {
					msg += fmt.Sprintf(" Mentions found: `%s`", mentions)
				}
				b.sendFollowupMessage(s, i.Interaction, msg)
			}
			return
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("✅ **TsukiHime Query Results for `%s`:**\n", query))
		limit := min(len(torrents), 5)
		for idx := range limit {
			t := torrents[idx]
			sb.WriteString(fmt.Sprintf("- **%s** (ID: %d)\n", t.Name, t.ID))
		}
		b.sendFollowupMessage(s, i.Interaction, sb.String())
	}
}
