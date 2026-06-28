package discord

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/geckyzz/contourgo/internal/scraper"
)

func (b *DiscordBot) registerSlashCommands(s *discordgo.Session) {
	guildID := "" // Global registration
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "status",
			Description: "Show bot status, system diagnostics, and database statistics",
		},
		{
			Name:        "reload",
			Description: "Reload configuration or trigger a manual monitors check",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "target",
					Description: "What to reload (default: monitors)",
					Required:    false,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Monitors check (force check now)", Value: "monitors"},
						{Name: "Configuration file (reload config.toml)", Value: "config"},
					},
				},
			},
		},
		{
			Name:        "monitors",
			Description: "Manage and inspect configured monitors",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "list",
					Description: "List all configured monitors and their status",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "pause",
					Description: "Pause a monitor (suppress checks and announcements)",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "service",
							Description: "Service name (e.g. nyaa, nekobt, twitter)",
							Required:    true,
						},
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "key",
							Description: "Monitor key as defined in config",
							Required:    true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "resume",
					Description: "Resume a previously paused monitor",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "service",
							Description: "Service name (e.g. nyaa, nekobt, twitter)",
							Required:    true,
						},
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "key",
							Description: "Monitor key as defined in config",
							Required:    true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "force",
					Description: "Force check a specific monitor immediately",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "service",
							Description: "Service name (e.g. nyaa, nekobt, twitter)",
							Required:    true,
						},
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "key",
							Description: "Monitor key as defined in config",
							Required:    true,
						},
					},
				},
			},
		},
		{
			Name:        "import",
			Description: "Import dumped JSON legacy data with key",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionAttachment,
					Name:        "file",
					Description: "The JSON database dump file to import",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "key",
					Description: "The Fernet decryption key (optional)",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "service",
					Description: "Target service (optional; defaults to auto-detect from filename)",
					Required:    false,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Nyaa", Value: "nyaa"},
						{Name: "Sukebei", Value: "sukebei"},
						{Name: "AnimeTosho", Value: "animetosho"},
						{Name: "AniRena", Value: "anirena"},
					},
				},
			},
		},
		{
			Name:        "test",
			Description: "Test a query search against Nyaa, Sukebei or AnimeTosho",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "service",
					Description: "Service to query",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Nyaa", Value: "nyaa"},
						{Name: "Sukebei", Value: "sukebei"},
						{Name: "AnimeTosho (Old)", Value: "animetosho_old"},
						{Name: "AnimeTosho (New)", Value: "animetosho_new"},
						{Name: "AnimeTosho Feedback (Old)", Value: "animetosho_old_feedback"},
						{Name: "AnimeTosho Feedback (New)", Value: "animetosho_new_feedback"},
						{Name: "nekoBT", Value: "nekobt"},
						{Name: "AniRena", Value: "anirena"},
						{Name: "TsukiHime", Value: "tsukihime"},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "query",
					Description: "Keyword or @username to query",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "page_max",
					Description: "Maximum pages to search (default: 1)",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "inspect",
					Description: "Choose what to inspect (default: result)",
					Required:    false,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Torrents list (result)", Value: "result"},
						{Name: "Fetch & Render comments", Value: "comments"},
						{Name: "Raw JSON snippet", Value: "raw"},
					},
				},
			},
		},
		{
			Name:        "help",
			Description: "Show help menu",
		},
		{
			Name:        "ping",
			Description: "Check bot health and latency",
		},
		{
			Name:        "logs",
			Description: "Show recent log entries",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "lines",
					Description: "Number of lines to show (default: 20, max: 100)",
					Required:    false,
				},
			},
		},
		{
			Name:        "latest",
			Description: "Show recently found torrents",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "limit",
					Description: "Number of torrents to show (default: 10, max: 50)",
					Required:    false,
				},
			},
		},
		{
			Name:        "update",
			Description: "Check for updates and update the bot binary if available",
		},
		{
			Name:        "donation",
			Description: "Manage donator roles and log contributions",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "add",
					Description: "Log a donation and add/extend role perks (+1 month per $9.99)",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionUser,
							Name:        "user",
							Description: "The Discord user to credit",
							Required:    true,
						},
						{
							Type:        discordgo.ApplicationCommandOptionNumber,
							Name:        "amount",
							Description: "Donation amount in USD",
							Required:    true,
						},
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "account",
							Description: "Payment method account (e.g. 'PayPal (Proxied)', 'Ethereum')",
							Required:    false,
						},
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "note",
							Description: "Contribution note/memo details",
							Required:    false,
						},
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "end_date",
							Description: "Custom end date to import old data (YYYY-MM-DD or 'July 17')",
							Required:    false,
						},
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "silent",
							Description: "Silence specific or all DM notifications (defaults to none)",
							Required:    false,
							Choices: []*discordgo.ApplicationCommandOptionChoice{
								{Name: "Silence All notifications", Value: "all"},
								{Name: "Silence Only donation add DM", Value: "only-add"},
								{Name: "Silence Only warning DM", Value: "on-warning"},
								{Name: "Silence Only expiry DM", Value: "on-expiry"},
							},
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "status",
					Description: "View donation subscription status for a user",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionUser,
							Name:        "user",
							Description: "The Discord user to inspect",
							Required:    true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "list",
					Description: "List all active donators and their role expiry times",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "export",
					Description: "Export all raw donation logs in TSV format as a file attachment",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionUser,
							Name:        "user",
							Description: "Filter logs by a specific user (optional)",
							Required:    false,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "check",
					Description: "Manually trigger checking for and clearing expired donator roles",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
					Name:        "manage",
					Description: "Manage existing donation records",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionSubCommand,
							Name:        "delete",
							Description: "Delete a donator record and all their logs (strips roles)",
							Options: []*discordgo.ApplicationCommandOption{
								{
									Type:        discordgo.ApplicationCommandOptionUser,
									Name:        "user",
									Description: "The Discord user to delete",
									Required:    true,
								},
							},
						},
						{
							Type:        discordgo.ApplicationCommandOptionSubCommand,
							Name:        "edit",
							Description: "Edit a donator's total amount and/or expiry date",
							Options: []*discordgo.ApplicationCommandOption{
								{
									Type:        discordgo.ApplicationCommandOptionUser,
									Name:        "user",
									Description: "The Discord user to edit",
									Required:    true,
								},
								{
									Type:        discordgo.ApplicationCommandOptionNumber,
									Name:        "total",
									Description: "Override the cumulative total amount",
									Required:    false,
								},
								{
									Type:        discordgo.ApplicationCommandOptionString,
									Name:        "expiry",
									Description: "Override the expiry date (YYYY-MM-DD or 'July 17')",
									Required:    false,
								},
							},
						},
						{
							Type:        discordgo.ApplicationCommandOptionSubCommand,
							Name:        "delete_log",
							Description: "Delete a single donation log entry by its numeric ID",
							Options: []*discordgo.ApplicationCommandOption{
								{
									Type:        discordgo.ApplicationCommandOptionInteger,
									Name:        "log_id",
									Description: "The numeric ID of the donation log to delete",
									Required:    true,
								},
							},
						},
					},
				},
			},
		},
	}

	// Fetch existing commands (guild-specific if guildID is set, otherwise global)
	existingCmds, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		log.Printf("Warning: Could not fetch slash commands: %v", err)
		existingCmds = []*discordgo.ApplicationCommand{}
	}

	// 0. Clean up guild-scoped commands if we are registering global commands but a server ID is configured
	if guildID == "" && string(b.Config.Discord.Server) != "" {
		guildCmds, err := s.ApplicationCommands(s.State.User.ID, string(b.Config.Discord.Server))
		if err == nil && len(guildCmds) > 0 {
			log.Printf(
				"Cleaning up %d guild-scoped commands from Server ID: %s",
				len(guildCmds),
				string(b.Config.Discord.Server),
			)
			for _, gCmd := range guildCmds {
				log.Printf(
					"[CLEANUP] Deleting guild-scoped command: %s (ID: %s)",
					gCmd.Name,
					gCmd.ID,
				)
				s.ApplicationCommandDelete(
					s.State.User.ID,
					string(b.Config.Discord.Server),
					gCmd.ID,
				)
			}
		}
	}

	// Create lookup maps
	existingMap := make(map[string]*discordgo.ApplicationCommand)
	for _, cmd := range existingCmds {
		existingMap[cmd.Name] = cmd
	}
	log.Printf(
		"Syncing %d desired global slash commands with %d existing ones",
		len(commands),
		len(existingCmds),
	)

	activeCmdNames := make(map[string]bool)
	for _, cmd := range commands {
		activeCmdNames[cmd.Name] = true
	}

	// 1. Delete commands that are registered but not in our activeCmdNames list
	for _, cmd := range existingCmds {
		if !activeCmdNames[cmd.Name] {
			log.Printf("Cleaning up stale slash command: %s (ID: %s)", cmd.Name, cmd.ID)
			err := s.ApplicationCommandDelete(s.State.User.ID, guildID, cmd.ID)
			if err != nil {
				log.Printf("Warning: Failed to delete stale slash command '%s': %v", cmd.Name, err)
			}
		}
	}

	// Helper function to compare options recursively
	var optionsEqual func(a, b []*discordgo.ApplicationCommandOption) bool
	optionsEqual = func(a, b []*discordgo.ApplicationCommandOption) bool {
		if len(a) != len(b) {
			return false
		}
		for idx := range a {
			optA := a[idx]
			optB := b[idx]
			if optA.Type != optB.Type ||
				optA.Name != optB.Name ||
				optA.Description != optB.Description ||
				optA.Required != optB.Required {
				return false
			}
			// Compare choices
			if len(optA.Choices) != len(optB.Choices) {
				return false
			}
			for cIdx := range optA.Choices {
				choiceA := optA.Choices[cIdx]
				choiceB := optB.Choices[cIdx]
				if choiceA.Name != choiceB.Name || choiceA.Value != choiceB.Value {
					return false
				}
			}
			// Compare nested options (e.g. subcommands)
			if !optionsEqual(optA.Options, optB.Options) {
				return false
			}
		}
		return true
	}

	// Helper to check if two command schemas are equal
	commandsEqual := func(a *discordgo.ApplicationCommand, b *discordgo.ApplicationCommand) bool {
		if a.Name != b.Name || a.Description != b.Description {
			return false
		}
		return optionsEqual(a.Options, b.Options)
	}

	// 2. Register new or updated commands
	hasFailure := false
	var registeredCmds []*discordgo.ApplicationCommand
	for _, desiredCmd := range commands {
		existing, exists := existingMap[desiredCmd.Name]
		if exists && commandsEqual(existing, desiredCmd) {
			log.Printf("[SYNC] %s (reusing ID: %s)", desiredCmd.Name, existing.ID)
			registeredCmds = append(registeredCmds, existing)
			continue
		}

		if exists {
			log.Printf("[UPDATE] %s (ID: %s)", desiredCmd.Name, existing.ID)
			updatedCmd, err := s.ApplicationCommandEdit(
				s.State.User.ID,
				guildID,
				existing.ID,
				desiredCmd,
			)
			if err != nil {
				log.Printf("ERROR: Cannot update slash command '%s': %v.", desiredCmd.Name, err)
				hasFailure = true
			} else {
				registeredCmds = append(registeredCmds, updatedCmd)
			}
		} else {
			log.Printf("[CREATE] %s", desiredCmd.Name)
			newCmd, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, desiredCmd)
			if err != nil {
				log.Printf("ERROR: Cannot create slash command '%s': %v.", desiredCmd.Name, err)
				hasFailure = true
			} else {
				registeredCmds = append(registeredCmds, newCmd)
			}
		}
	}

	b.Commands = registeredCmds

	if hasFailure {
		inviteURL := fmt.Sprintf(
			"https://discord.com/oauth2/authorize?client_id=%s&permissions=8&integration_type=0&scope=bot+applications.commands",
			s.State.User.ID,
		)
		log.Printf(
			"👉 Please authorize/re-authorize the bot using this link to grant the applications.commands scope: %s",
			inviteURL,
		)
	}
}

func (b *DiscordBot) hasInteractionAccess(i *discordgo.InteractionCreate) bool {
	var userID string
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	if b.OwnerID != "" && userID == b.OwnerID {
		return true
	}

	if slices.Contains(b.Config.Discord.Members.Others.Allow, userID) {
		return true
	}

	if i.GuildID == "" {
		return false
	}

	perms, err := b.Session.UserChannelPermissions(userID, i.ChannelID)
	if err != nil {
		return false
	}

	if b.Config.Discord.Members.Admins.Allow && (perms&discordgo.PermissionAdministrator) != 0 {
		return true
	}

	if b.Config.Discord.Members.Moderators.Allow &&
		(perms&discordgo.PermissionManageMessages) != 0 {
		return true
	}

	return false
}

func (b *DiscordBot) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type == discordgo.InteractionMessageComponent {
		b.handleInteractionComponent(s, i)
		return
	}
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	var userName string
	var userID string
	if i.Member != nil {
		userName = i.Member.User.Username
		userID = i.Member.User.ID
	} else if i.User != nil {
		userName = i.User.Username
		userID = i.User.ID
	}

	cmdName := i.ApplicationCommandData().Name
	var params []string
	for _, opt := range i.ApplicationCommandData().Options {
		params = append(params, fmt.Sprintf("%s: %v", opt.Name, opt.Value))
	}
	log.Printf(
		"[Command] User %s (%s) triggered /%s with params: {%s}",
		userName,
		userID,
		cmdName,
		strings.Join(params, ", "),
	)

	if !b.hasInteractionAccess(i) {
		log.Printf(
			"[Action] Permission denied for user %s (%s) attempting command /%s",
			userName,
			userID,
			cmdName,
		)
		log.Printf("[POST] Responding to /%s command (Permission Denied)", cmdName)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ You do not have permission to use this bot.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	switch cmdName {
	case "status":
		log.Printf("[Action] Processing status request")
		b.handleSlashStatus(s, i)
	case "reload":
		log.Printf("[Action] Processing reload request")
		b.handleSlashReload(s, i, optionMap)
	case "monitors":
		log.Printf("[Action] Processing monitors command")
		b.handleSlashMonitors(s, i, optionMap)
	case "import":
		log.Printf("[Action] Processing data import request")
		b.handleSlashImport(s, i, optionMap)
	case "test":
		log.Printf("[Action] Processing test query request")
		b.handleSlashTest(s, i, optionMap)
	case "help":
		log.Printf("[Action] Processing help command")
		b.handleSlashHelp(s, i)
	case "ping":
		log.Printf("[Action] Processing ping command")
		b.handleSlashPing(s, i)
	case "logs":
		log.Printf("[Action] Processing logs command")
		b.handleSlashLogs(s, i, optionMap)
	case "latest":
		log.Printf("[Action] Processing latest command")
		b.handleSlashLatest(s, i, optionMap)
	case "update":
		log.Printf("[Action] Processing update command")
		b.handleSlashUpdate(s, i)
	case "donation":
		log.Printf("[Action] Processing donation command")
		b.handleSlashDonation(s, i, optionMap)
	}
}

func (b *DiscordBot) sendFollowupMessage(
	s *discordgo.Session,
	interaction *discordgo.Interaction,
	content string,
) {
	log.Printf("[POST] Sending Followup Message: %s", content)
	_, err := s.FollowupMessageCreate(interaction, true, &discordgo.WebhookParams{
		Content: content,
	})
	if err != nil {
		log.Printf("Error sending followup: %v", err)
	}
}

func (b *DiscordBot) handleInteractionComponent(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
) {
	data := i.MessageComponentData()
	customID := data.CustomID

	if strings.HasPrefix(customID, "nekobt_read:") {
		parts := strings.Split(customID, ":")
		if len(parts) < 4 {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "❌ Invalid interaction data.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		notificationID := parts[1]
		service := parts[2]
		monitorKey := parts[3]

		if !b.hasInteractionAccess(i) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "❌ You do not have permission to mark this notification as read.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		log.Printf(
			"[Component] User clicked Mark as Read for notification %s (service: %s, monitor: %s)",
			notificationID,
			service,
			monitorKey,
		)

		cfg := b.Config
		monitorMap, exists := cfg.Monitors[service]
		if !exists {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "❌ Monitor service not found in configuration.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		_, exists = monitorMap[monitorKey]
		if !exists {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "❌ Specific monitor not found in configuration.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		apiKey := cfg.Config.Nekobt.API.Key
		if apiKey == "" {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "❌ NekoBT API key is not configured.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		scr := scraper.NewNekoBTScraper(apiKey)
		err := scr.MarkNotificationsAsRead([]string{notificationID})
		if err != nil {
			log.Printf("[Component] Error marking notification %s as read: %v", notificationID, err)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("❌ Error marking notification as read: %v", err),
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		log.Printf("[Component] Successfully marked notification %s as read", notificationID)

		var components []discordgo.MessageComponent
		if i.Message != nil && len(i.Message.Components) > 0 {
			if row, ok := i.Message.Components[0].(*discordgo.ActionsRow); ok {
				var editedButtons []discordgo.MessageComponent
				for _, comp := range row.Components {
					if btn, ok := comp.(*discordgo.Button); ok {
						if btn.CustomID == customID {
							btn.Disabled = true
							btn.Label = "Read"
							btn.Style = discordgo.SecondaryButton
						}
						editedButtons = append(editedButtons, btn)
					} else {
						editedButtons = append(editedButtons, comp)
					}
				}
				components = []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: editedButtons,
					},
				}
			}
		}

		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Components: components,
			},
		})
		if err != nil {
			log.Printf("[Component] Error responding to interaction: %v", err)
		}
	}
}
