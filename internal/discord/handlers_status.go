package discord

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

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

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	dbFile, err := os.Stat("bot.db")
	dbSize := "Unknown"
	if err == nil {
		dbSize = fmt.Sprintf("%.2f MB", float64(dbFile.Size())/1024/1024)
	}

	v, sha := GetVersionInfo()

	embed := &discordgo.MessageEmbed{
		Title: "📊 Bot Status & Diagnostics",
		Color: 0x2ecc71,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Active Monitors", Value: fmt.Sprintf("%d", totalMonitors), Inline: true},
			{Name: "Torrents Tracked", Value: fmt.Sprintf("%d", torrents), Inline: true},
			{Name: "Comments Stored", Value: fmt.Sprintf("%d", comments), Inline: true},
			{Name: "Check Interval", Value: b.Config.Config.Monitor.By, Inline: true},
			{Name: "Database Size", Value: dbSize, Inline: true},
			{Name: "Uptime", Value: fmt.Sprintf("<t:%d:R>", b.StartTime.Unix()), Inline: true},
			{
				Name: "Memory (Alloc / Sys)",
				Value: fmt.Sprintf(
					"%.2f MB / %.2f MB",
					float64(m.Alloc)/1024/1024,
					float64(m.Sys)/1024/1024,
				),
				Inline: true,
			},
			{Name: "Bot Version", Value: fmt.Sprintf("v%s (%s)", v, sha), Inline: true},
			{
				Name:   "Environment",
				Value:  fmt.Sprintf("Go %s (%s)", runtime.Version(), runtime.GOOS),
				Inline: true,
			},
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

func (b *DiscordBot) handleSlashLatest(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	target := "torrents"
	if opt, ok := optionMap["target"]; ok {
		target = opt.StringValue()
	}

	limit := 10
	if opt, ok := optionMap["limit"]; ok {
		limit = int(opt.IntValue())
	}

	if target == "torrents" {
		torrents, err := b.DB.GetLatestTorrents(limit)
		if err != nil {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("❌ Error retrieving latest torrents: %v", err),
				},
			})
			return
		}

		var description string
		if len(torrents) == 0 {
			description = "No torrents tracked yet."
		} else {
			var sb strings.Builder
			for _, t := range torrents {
				sb.WriteString(fmt.Sprintf("- **[%s]** %s (ID: %s) - %d comments\n", strings.ToUpper(t.Service), t.Title, t.TorrentID, t.CommentCount))
			}
			description = sb.String()
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
	} else {
		comments, err := b.DB.GetLatestComments(limit)
		if err != nil {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("❌ Error retrieving latest comments: %v", err),
				},
			})
			return
		}

		var description string
		if len(comments) == 0 {
			description = "No comments stored yet."
		} else {
			var sb strings.Builder
			for _, c := range comments {
				sb.WriteString(fmt.Sprintf("- **[%s]** %s on %s: `%s`\n", strings.ToUpper(c.Service), c.Username, c.TorrentID, trimDescription(c.Message)))
			}
			description = sb.String()
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       fmt.Sprintf("💬 Latest %d Stored Comments", len(comments)),
						Description: description,
						Color:       0x3498db,
					},
				},
			},
		})
	}
}

func (b *DiscordBot) handleSlashMonitors(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var sb strings.Builder
	for svc, monitors := range b.Config.Monitors {
		sb.WriteString(fmt.Sprintf("📂 **%s**\n", strings.ToUpper(svc)))
		for key, cfg := range monitors {
			var filters []string
			if len(cfg.Keywords) > 0 {
				filters = append(filters, fmt.Sprintf("KW: %v", cfg.Keywords))
			}
			if len(cfg.Uploaders) > 0 {
				filters = append(filters, fmt.Sprintf("UP: %v", cfg.Uploaders))
			}
			if len(cfg.Groups) > 0 {
				filters = append(filters, fmt.Sprintf("GR: %v", cfg.Groups))
			}

			filterStr := strings.Join(filters, " | ")
			if filterStr == "" {
				filterStr = "Global"
			}
			sb.WriteString(fmt.Sprintf("- `%s`: %s\n", key, filterStr))
		}
		sb.WriteString("\n")
	}

	content := sb.String()
	if content == "" {
		content = "No monitors configured."
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       "🕵️ Active Monitors",
					Description: content,
					Color:       0x9b59b6,
				},
			},
		},
	})
}
