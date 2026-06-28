package discord

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// handleSlashDonation dispatches subcommands for the /donation command.
func (b *DiscordBot) handleSlashDonation(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Missing subcommand.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	subCmd := options[0]
	subOptionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption)
	for _, opt := range subCmd.Options {
		subOptionMap[opt.Name] = opt
	}

	switch subCmd.Name {
	case "add":
		b.handleDonationAdd(s, i, subOptionMap)
	case "status":
		b.handleDonationStatus(s, i, subOptionMap)
	case "list":
		b.handleDonationList(s, i)
	case "export":
		b.handleDonationExport(s, i, subOptionMap)
	case "history":
		b.handleDonationHistory(s, i, subOptionMap)
	case "check":
		b.handleDonationCheck(s, i)
	case "manage":
		// SubCommandGroup: dispatch inner subcommand
		if len(subCmd.Options) == 0 {
			break
		}
		inner := subCmd.Options[0]
		innerOptionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption)
		for _, opt := range inner.Options {
			innerOptionMap[opt.Name] = opt
		}
		switch inner.Name {
		case "delete":
			b.handleDonationManageDelete(s, i, innerOptionMap)
		case "edit":
			b.handleDonationManageEdit(s, i, innerOptionMap)
		case "delete_log":
			b.handleDonationManageDeleteLog(s, i, innerOptionMap)
		}
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

func (b *DiscordBot) handleDonationManageDelete(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	targetUser := optionMap["user"].UserValue(s)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	if _, ok := b.DB.GetDonator(targetUser.ID); !ok {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("ℹ️ No donation record found for %s.", targetUser.Username),
		)
		return
	}

	// Strip all tier roles before deleting
	serverID := string(b.Config.Discord.Server)
	b.syncUserDonatorRoles(s, serverID, targetUser.ID, 0, 0)

	if err := b.DB.DeleteDonator(targetUser.ID); err != nil {
		b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Failed to delete record: %v", err))
		return
	}

	b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf(
		"✅ Deleted all donation records and logs for **%s** (`%s`) and stripped their roles.",
		targetUser.Username, targetUser.ID,
	))
}

func (b *DiscordBot) handleDonationManageEdit(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	targetUser := optionMap["user"].UserValue(s)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	existing, ok := b.DB.GetDonator(targetUser.ID)
	if !ok {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("ℹ️ No donation record found for %s.", targetUser.Username),
		)
		return
	}

	newTotal := existing.TotalUSD
	if opt, ok := optionMap["total"]; ok {
		newTotal = opt.FloatValue()
	}

	newExpiry := existing.ExpiryTime
	if opt, ok := optionMap["expiry"]; ok {
		t, err := parseCustomEndDate(opt.StringValue())
		if err != nil {
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Date error: %v", err))
			return
		}
		newExpiry = t.Unix()
	}

	if err := b.DB.UpdateDonator(targetUser.ID, targetUser.Username, newTotal, newExpiry); err != nil {
		b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Failed to update record: %v", err))
		return
	}

	// Re-sync roles with updated values
	serverID := string(b.Config.Discord.Server)
	b.syncUserDonatorRoles(s, serverID, targetUser.ID, newTotal, newExpiry)

	expiryStr := time.Unix(newExpiry, 0).Format("January 2, 2006 at 3:04 PM MST")
	b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf(
		"✅ Updated **%s** (`%s`):\n- Total: **%.2f %s**\n- Expiry: **%s**",
		targetUser.Username, targetUser.ID, newTotal, b.getCurrency(), expiryStr,
	))
}

func (b *DiscordBot) handleDonationManageDeleteLog(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	logID := int(optionMap["log_id"].IntValue())
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	if err := b.DB.DeleteDonationLog(logID); err != nil {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Failed to delete log entry #%d: %v", logID, err),
		)
		return
	}

	b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("✅ Deleted donation log entry #%d.", logID))
}

func (b *DiscordBot) handleDonationAdd(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	targetUser := optionMap["user"].UserValue(s)
	amount := optionMap["amount"].FloatValue()

	account := ""
	if opt, ok := optionMap["account"]; ok {
		account = opt.StringValue()
	}

	note := ""
	if opt, ok := optionMap["note"]; ok {
		note = opt.StringValue()
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	multiplier := b.Config.Donation.PerkMultiplier
	if multiplier <= 0 {
		multiplier = 9.99
	}

	maxStacks := b.Config.Donation.MaxStacks
	if maxStacks <= 0 {
		maxStacks = 12 // Default fallback
	}

	// Read existing donator status
	existing, ok := b.DB.GetDonator(targetUser.ID)
	var newTotalUSD float64
	var currentExpiry int64
	now := time.Now().Unix()
	isRenewal := false

	if ok {
		newTotalUSD = existing.TotalUSD + amount
		currentExpiry = existing.ExpiryTime
		if currentExpiry > now {
			isRenewal = true
		} else {
			currentExpiry = now
		}
	} else {
		newTotalUSD = amount
		currentExpiry = now
	}

	var newExpiry int64
	var isCustomEnd bool
	var secondsAdded int64

	// If custom end_date option was provided
	if opt, customDateProvided := optionMap["end_date"]; customDateProvided {
		customT, err := parseCustomEndDate(opt.StringValue())
		if err != nil {
			b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Date error: %v", err))
			return
		}
		newExpiry = customT.Unix()
		secondsAdded = newExpiry - currentExpiry
		if secondsAdded < 0 {
			secondsAdded = newExpiry - now
		}
		isCustomEnd = true
	} else {
		// Calculate standard stacked duration in days
		daysToAdd := (amount / multiplier) * 30.0
		secondsAdded = int64(math.Round(daysToAdd * 24 * 60 * 60))
		newExpiry = currentExpiry + secondsAdded
	}

	// Check max stacks constraint (unless importing via custom end date)
	if !isCustomEnd {
		maxAllowedExpiry := now + int64(maxStacks)*30*24*60*60
		if newExpiry > maxAllowedExpiry {
			newExpiry = maxAllowedExpiry
			secondsAdded = newExpiry - currentExpiry
			if secondsAdded < 0 {
				secondsAdded = 0
			}
		}
	}

	// Save to DB (update cumulative donator record)
	err := b.DB.UpdateDonator(targetUser.ID, targetUser.Username, newTotalUSD, newExpiry)
	if err != nil {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Failed to update database donators: %v", err),
		)
		return
	}

	// Save to detailed logs
	err = b.DB.AddDonationLog(targetUser.ID, amount, account, note)
	if err != nil {
		log.Printf("⚠️ Failed to write donation log to DB: %v", err)
	}

	// Update guild roles based on tiers
	serverID := string(b.Config.Discord.Server)
	b.syncUserDonatorRoles(s, serverID, targetUser.ID, newTotalUSD, newExpiry)

	durationDesc := "custom date"
	if secondsAdded > 0 {
		durationDesc = formatDuration(secondsAdded)
	}
	expiryTimeStr := time.Unix(newExpiry, 0).Format("January 2, 2006 at 3:04 PM MST")

	// Determine command-specific silent constraints
	silentVal := ""
	if opt, ok := optionMap["silent"]; ok {
		silentVal = opt.StringValue()
	}

	// Globals check from config (using dot notation struct)
	notifyAdd := true
	if b.Config.Donation.Silent.Globally || silentVal == "all" || silentVal == "only-add" {
		notifyAdd = false
	}

	if notifyAdd {
		b.sendDonatorNotificationEmbed(
			s,
			targetUser,
			amount,
			newTotalUSD,
			durationDesc,
			newExpiry,
			isRenewal,
			account,
			note,
			now,
		)
	}

	msg := fmt.Sprintf(
		"QA: Role has been added/synced for %s (%s) for %s. It will be removed/modified at %s.",
		targetUser.Username, targetUser.ID, durationDesc, expiryTimeStr,
	)
	if notifyAdd {
		msg += " DM notification sent."
	} else {
		msg += " DM notification was silenced."
	}
	b.sendFollowupMessage(s, i.Interaction, msg)
}

func (b *DiscordBot) handleDonationStatus(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	targetUser := optionMap["user"].UserValue(s)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	donator, ok := b.DB.GetDonator(targetUser.ID)
	if !ok {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("ℹ️ %s has no logged donations.", targetUser.Username),
		)
		return
	}

	now := time.Now().Unix()
	var statusStr string
	if donator.ExpiryTime <= now {
		statusStr = "❌ Expired"
	} else {
		timeLeft := donator.ExpiryTime - now
		statusStr = fmt.Sprintf("✅ Active (expires in %s)", formatDuration(timeLeft))
	}

	expiryTimeStr := "Never"
	if donator.ExpiryTime > 0 {
		expiryTimeStr = time.Unix(donator.ExpiryTime, 0).Format("January 2, 2006 at 3:04 PM MST")
	}

	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("💰 Donation Status: %s", targetUser.Username),
		Color: 0xf1c40f,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Total Donated",
				Value:  fmt.Sprintf("%.2f %s", donator.TotalUSD, b.getCurrency()),
				Inline: true,
			},
			{Name: "Status", Value: statusStr, Inline: true},
			{Name: "Expiration Date", Value: expiryTimeStr, Inline: false},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	b.sendFollowupEmbed(s, i.Interaction, embed)
}

func (b *DiscordBot) handleDonationList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	now := time.Now().Unix()
	activeDonators, err := b.DB.GetActiveDonators(now)
	if err != nil {
		b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error retrieving donators: %v", err))
		return
	}

	if len(activeDonators) == 0 {
		b.sendFollowupMessage(s, i.Interaction, "ℹ️ No active donators currently registered.")
		return
	}

	var sb strings.Builder
	for _, d := range activeDonators {
		expiryStr := time.Unix(d.ExpiryTime, 0).Format("2006-01-02")
		timeLeft := d.ExpiryTime - now
		sb.WriteString(
			fmt.Sprintf(
				"- **%s** (`%s`): %.2f %s total (expires in %s | `%s`)\n",
				d.Username,
				d.UserID,
				d.TotalUSD,
				b.getCurrency(),
				formatDuration(timeLeft),
				expiryStr,
			),
		)
	}

	embed := &discordgo.MessageEmbed{
		Title:       "💎 Active Donators List",
		Description: sb.String(),
		Color:       0x9b59b6,
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	b.sendFollowupEmbed(s, i.Interaction, embed)
}

func (b *DiscordBot) handleDonationExport(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	var filterUserID string
	if opt, ok := optionMap["user"]; ok {
		filterUserID = opt.UserValue(s).ID
	}

	logs, err := b.DB.GetDonationLogs(filterUserID)
	if err != nil {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Error fetching donation logs: %v", err),
		)
		return
	}

	if len(logs) == 0 {
		b.sendFollowupMessage(s, i.Interaction, "ℹ️ No donation logs found.")
		return
	}

	var buf bytes.Buffer
	buf.WriteString("date\tuser_id\tamount\taccount\tnote\n")
	for _, l := range logs {
		dateStr := l.CreatedAt.Format("2006-01-02")
		buf.WriteString(
			fmt.Sprintf("%s\t%s\t%.2f\t%s\t%s\n", dateStr, l.UserID, l.Amount, l.Account, l.Note),
		)
	}

	filename := "donations_export.tsv"
	if filterUserID != "" {
		filename = fmt.Sprintf("donations_%s_export.tsv", filterUserID)
	}

	s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: "📋 Exported donation logs in TSV format:",
		Files: []*discordgo.File{
			{
				Name:        filename,
				ContentType: "text/tab-separated-values",
				Reader:      bytes.NewReader(buf.Bytes()),
			},
		},
	})
}

func (b *DiscordBot) handleDonationCheck(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	count, err := b.CheckAndClearExpiredDonators()
	if err != nil {
		b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error during check: %v", err))
		return
	}

	b.sendFollowupMessage(
		s,
		i.Interaction,
		fmt.Sprintf("✅ Checked expired donators. Removed roles from %d user(s).", count),
	)
}

func (b *DiscordBot) handleDonationHistory(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	userOpt, ok := optionMap["user"]
	if !ok {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ User option is required.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	user := userOpt.UserValue(s)

	logs, err := b.DB.GetDonationLogs(user.ID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Failed to query donation logs: %v", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if len(logs) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("ℹ️ No donation logs found for %s.", user.Username),
			},
		})
		return
	}

	var sb strings.Builder
	for _, l := range logs {
		details := ""
		if l.Account != "" {
			details += fmt.Sprintf(" via `%s` ", l.Account)
		}
		if l.Note != "" {
			details += fmt.Sprintf(" (%s)", l.Note)
		}
		sb.WriteString(
			fmt.Sprintf(
				"- **%s**: %.2f %s%s (ID: #%d)\n",
				l.CreatedAt.Format("2006-01-02"),
				l.Amount,
				b.getCurrency(),
				details,
				l.ID,
			),
		)
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       fmt.Sprintf("📋 Donation History: %s", user.Username),
					Description: sb.String(),
					Color:       0x00aeef,
					Timestamp:   time.Now().Format(time.RFC3339),
				},
			},
		},
	})
}
