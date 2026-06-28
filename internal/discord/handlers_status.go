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

func (b *DiscordBot) handleSlashMonitors(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		b.handleMonitorList(s, i)
		return
	}

	subCmd := options[0]
	subOptionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption)
	for _, opt := range subCmd.Options {
		subOptionMap[opt.Name] = opt
	}

	switch subCmd.Name {
	case "list":
		b.handleMonitorList(s, i)
	case "pause":
		b.handleMonitorPauseResume(s, i, subOptionMap, true)
	case "resume":
		b.handleMonitorPauseResume(s, i, subOptionMap, false)
	case "force":
		b.handleMonitorForce(s, i, subOptionMap)
	default:
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Unknown subcommand.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
}

func (b *DiscordBot) handleMonitorList(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

			statusEmoji := "🟢"
			if b.IsMonitorPaused(svc, key) {
				statusEmoji = "⏸️"
			}

			sb.WriteString(fmt.Sprintf("- %s `%s`: %s\n", statusEmoji, key, filterStr))
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

func (b *DiscordBot) handleMonitorPauseResume(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
	paused bool,
) {
	svcOpt, okSvc := optionMap["service"]
	keyOpt, okKey := optionMap["key"]
	if !okSvc || !okKey {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Both service and key options are required.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	svc := strings.ToLower(svcOpt.StringValue())
	key := strings.ToLower(keyOpt.StringValue())

	// Validate service and key
	inner, ok := b.Config.Monitors[svc]
	if !ok {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Service `%s` not found in config.", svc),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// We look up monitor case-insensitively or exactly. Since monitorMap has exact keys, let's look up exactly or lowercase.
	var matchedKey string
	for k := range inner {
		if strings.ToLower(k) == key {
			matchedKey = k
			break
		}
	}

	if matchedKey == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Monitor key `%s` not found under service `%s`.", key, svc),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	b.SetMonitorPaused(svc, matchedKey, paused)

	action := "paused"
	statusEmoji := "⏸️"
	if !paused {
		action = "resumed"
		statusEmoji = "▶️"
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf(
				"%s Monitor **%s/%s** has been %s.",
				statusEmoji,
				svc,
				matchedKey,
				action,
			),
		},
	})
}

func (b *DiscordBot) handleMonitorForce(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	svcOpt, okSvc := optionMap["service"]
	keyOpt, okKey := optionMap["key"]
	if !okSvc || !okKey {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Both service and key options are required.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	svc := strings.ToLower(svcOpt.StringValue())
	key := strings.ToLower(keyOpt.StringValue())

	inner, ok := b.Config.Monitors[svc]
	if !ok {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Service `%s` not found in config.", svc),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	var matchedKey string
	for k := range inner {
		if strings.ToLower(k) == key {
			matchedKey = k
			break
		}
	}

	if matchedKey == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Monitor key `%s` not found under service `%s`.", key, svc),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	select {
	case b.ForceMonitorChan <- [2]string{svc, matchedKey}:
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf(
					"🔄 Triggered immediate force-check for monitor **%s/%s**.",
					svc,
					matchedKey,
				),
			},
		})
	default:
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "⚠️ Force-check queue is full. Please try again later.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
}
