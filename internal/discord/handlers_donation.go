package discord

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"strings"
	"text/template"
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
	case "check":
		b.handleDonationCheck(s, i)
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

func parseCustomEndDate(dateStr string) (time.Time, error) {
	dateStr = strings.TrimSpace(dateStr)
	formats := []string{
		"2006-01-02",
		"January 2, 2006",
		"Jan 2, 2006",
		"2006/01/02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, dateStr, time.Local); err == nil {
			return t, nil
		}
	}

	// Try without year (infer current year or next year depending on date)
	currentYear := time.Now().Year()
	shortFormats := []string{
		"January 2",
		"Jan 2",
		"01-02",
		"01/02",
	}
	for _, sf := range shortFormats {
		if t, err := time.ParseInLocation(sf, dateStr, time.Local); err == nil {
			// Construct time with current year
			parsedTime := time.Date(currentYear, t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
			return parsedTime, nil
		}
	}

	return time.Time{}, fmt.Errorf(
		"could not parse date %q. Use formats like '2026-07-17', 'July 17', or 'Jul 17 2026'",
		dateStr,
	)
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
		)
	}

	msg := fmt.Sprintf(
		"✅ Role has been added/synced for %s (%s) for %s. It will be removed/modified at %s.",
		targetUser.Username, targetUser.ID, durationDesc, expiryTimeStr,
	)
	if notifyAdd {
		msg += " DM notification sent."
	} else {
		msg += " DM notification was silenced."
	}
	b.sendFollowupMessage(s, i.Interaction, msg)
}

func (b *DiscordBot) syncUserDonatorRoles(
	s *discordgo.Session,
	serverID, userID string,
	totalUSD float64,
	expiry int64,
) {
	now := time.Now().Unix()

	// Determine active state
	isActive := expiry > now

	// Manage Multi-tier Roles

	for tierRoleID, minAmount := range b.Config.Donation.Tiers {
		if tierRoleID == "" {
			continue
		}
		if isActive && totalUSD >= minAmount {
			s.GuildMemberRoleAdd(serverID, userID, tierRoleID)
		} else {
			s.GuildMemberRoleRemove(serverID, userID, tierRoleID)
		}
	}
}

func (b *DiscordBot) renderDonationTemplate(tplStr string, data any, defaultVal string) string {
	if tplStr == "" {
		return defaultVal
	}
	t, err := template.New("donation_msg").Parse(tplStr)
	if err != nil {
		log.Printf("⚠️ Failed to parse donation DM template %q: %v", tplStr, err)
		return defaultVal
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		log.Printf("⚠️ Failed to execute donation DM template %q: %v", tplStr, err)
		return defaultVal
	}
	return buf.String()
}

func (b *DiscordBot) sendDonatorNotificationEmbed(
	s *discordgo.Session,
	user *discordgo.User,
	amount float64,
	totalUSD float64,
	duration string,
	expiry int64,
	isRenewal bool,
) {
	channel, err := s.UserChannelCreate(user.ID)
	if err != nil {
		log.Printf("⚠️ Could not create DM channel for user %s: %v", user.ID, err)
		return
	}

	expiryStr := time.Unix(expiry, 0).Format("January 2, 2006 at 3:04 PM MST")

	tplData := map[string]any{
		"Username":        user.Username,
		"UserID":          user.ID,
		"Amount":          fmt.Sprintf("$%.2f USD", amount),
		"AmountValue":     amount,
		"Cumulative":      fmt.Sprintf("$%.2f USD", totalUSD),
		"CumulativeValue": totalUSD,
		"Duration":        duration,
		"Expiry":          expiryStr,
		"ExpiryUnix":      expiry,
	}

	var title, desc string
	if isRenewal {
		title = b.renderDonationTemplate(
			b.Config.Donation.Format.Renew.Title,
			tplData,
			"🔁 Perks Renewed & Subscription Extended!",
		)
		desc = b.renderDonationTemplate(
			b.Config.Donation.Format.Renew.Desc,
			tplData,
			"Thank you so much for renewing your support! We've extended your perks accordingly.",
		)
	} else {
		title = b.renderDonationTemplate(
			b.Config.Donation.Format.Add.Title,
			tplData,
			"💖 Donation Received & Perks Activated!",
		)
		desc = b.renderDonationTemplate(
			b.Config.Donation.Format.Add.Desc,
			tplData,
			"Thank you so much for your support! Your donation has been recorded, and your perks are now active.",
		)
	}

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Color:       0xe91e63, // Vibrant Pink
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Amount Added", Value: fmt.Sprintf("$%.2f USD", amount), Inline: true},
			{Name: "Cumulative Total", Value: fmt.Sprintf("$%.2f USD", totalUSD), Inline: true},
			{Name: "Duration Granted", Value: duration, Inline: true},
			{Name: "Expiry Date", Value: expiryStr, Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "This automated DM was sent because you donated to our Discord Server.",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, err = s.ChannelMessageSendEmbed(channel.ID, embed)
	if err != nil {
		log.Printf("⚠️ Could not send DM to user %s: %v", user.ID, err)
	}
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
				Value:  fmt.Sprintf("$%.2f USD", donator.TotalUSD),
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
				"- **%s** (`%s`): $%.2f total (expires in %s | `%s`)\n",
				d.Username,
				d.UserID,
				d.TotalUSD,
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

// CheckAndClearExpiredDonators verifies database records against current time, strips roles, and handles warnings.
func (b *DiscordBot) CheckAndClearExpiredDonators() (int, error) {
	now := time.Now().Unix()
	expiredList, err := b.DB.GetExpiredDonators(now)
	if err != nil {
		return 0, err
	}

	serverID := string(b.Config.Discord.Server)
	removedCount := 0
	for _, donator := range expiredList {
		// Sync roles to strip expired state
		b.syncUserDonatorRoles(b.Session, serverID, donator.UserID, donator.TotalUSD, 0)

		// Send expired notification DM using beautiful Embed (unless silenced)
		if !b.Config.Donation.Silent.Globally && !b.Config.Donation.Silent.OnExpiry {
			b.sendExpiredNotificationEmbed(b.Session, donator.UserID)
		}

		// Update database expiry time to 0 to mark as cleared
		err = b.DB.UpdateDonator(donator.UserID, donator.Username, donator.TotalUSD, 0)
		if err != nil {
			log.Printf("⚠️ Failed to update database expiry for user %s: %v", donator.UserID, err)
		} else {
			removedCount++
		}
	}

	// Warning notifications logic
	warnDays := b.Config.Donation.NotifyWarnDays
	if warnDays > 0 && !b.Config.Donation.Silent.Globally && !b.Config.Donation.Silent.OnWarning {
		activeList, err := b.DB.GetActiveDonators(now)
		if err == nil {
			warnSecondsThreshold := int64(warnDays * 24 * 60 * 60)
			for _, donator := range activeList {
				// If expiry is within the warning threshold and they haven't been warned yet
				if donator.ExpiryTime-now <= warnSecondsThreshold && donator.WarnedAt == 0 {
					b.sendWarningNotificationEmbed(b.Session, donator.UserID, donator.ExpiryTime)
					if updateErr := b.DB.UpdateDonatorWarned(donator.UserID, now); updateErr != nil {
						log.Printf(
							"⚠️ Failed to record warning timestamp for user %s: %v",
							donator.UserID,
							updateErr,
						)
					}
				}
			}
		}
	}

	return removedCount, nil
}

func (b *DiscordBot) sendWarningNotificationEmbed(
	s *discordgo.Session,
	userID string,
	expiry int64,
) {
	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		log.Printf("⚠️ Could not create DM channel for user %s: %v", userID, err)
		return
	}

	username := ""
	discordUser, err := s.User(userID)
	if err == nil {
		username = discordUser.Username
	}

	expiryStr := time.Unix(expiry, 0).Format("January 2, 2006 at 3:04 PM MST")
	timeLeft := expiry - time.Now().Unix()
	timeLeftStr := formatDuration(timeLeft)

	tplData := map[string]any{
		"Username":   username,
		"UserID":     userID,
		"Expiry":     expiryStr,
		"ExpiryUnix": expiry,
		"TimeLeft":   timeLeftStr,
	}

	title := b.renderDonationTemplate(
		b.Config.Donation.Format.Warn.Title,
		tplData,
		"⚠️ Donator Perks Expiring Soon",
	)
	desc := b.renderDonationTemplate(
		b.Config.Donation.Format.Warn.Desc,
		tplData,
		"Your donator perks will expire on {{.Expiry}} (in {{.TimeLeft}}). Renew your subscription to keep access!",
	)

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Color:       0xe67e22, // Orange Warning
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Expiration Date", Value: expiryStr, Inline: true},
			{Name: "Time Remaining", Value: timeLeftStr, Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "This automated DM was sent because you donated to our Discord Server.",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, err = s.ChannelMessageSendEmbed(channel.ID, embed)
	if err != nil {
		log.Printf("⚠️ Could not send DM warning to user %s: %v", userID, err)
	}
}

func (b *DiscordBot) sendExpiredNotificationEmbed(s *discordgo.Session, userID string) {
	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		log.Printf("⚠️ Could not create DM channel for user %s: %v", userID, err)
		return
	}

	username := ""
	discordUser, err := s.User(userID)
	if err == nil {
		username = discordUser.Username
	}

	var expiry int64
	expiryStr := "Expired"
	if donator, ok := b.DB.GetDonator(userID); ok {
		expiry = donator.ExpiryTime
		expiryStr = time.Unix(expiry, 0).Format("January 2, 2006 at 3:04 PM MST")
	}

	tplData := map[string]any{
		"Username":   username,
		"UserID":     userID,
		"Expiry":     expiryStr,
		"ExpiryUnix": expiry,
	}

	title := b.renderDonationTemplate(
		b.Config.Donation.Format.Expiry.Title,
		tplData,
		"🔒 Donator Perks Expired",
	)
	desc := b.renderDonationTemplate(
		b.Config.Donation.Format.Expiry.Desc,
		tplData,
		"Your donator role subscription has expired, and your perks have been disabled. We want to thank you sincerely for supporting the cause!",
	)

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Color:       0x7f8c8d, // Dark Gray
		Footer: &discordgo.MessageEmbedFooter{
			Text: "This automated DM was sent because you donated to our Discord Server.",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, err = s.ChannelMessageSendEmbed(channel.ID, embed)
	if err != nil {
		log.Printf("⚠️ Could not send DM to user %s: %v", userID, err)
	}
}

// Helper function to format duration in a readable way
func formatDuration(seconds int64) string {
	if seconds <= 0 {
		return "0s"
	}

	days := seconds / (24 * 60 * 60)
	seconds %= (24 * 60 * 60)
	hours := seconds / (60 * 60)
	seconds %= (60 * 60)
	minutes := seconds / 60

	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d days", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d hours", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%d minutes", minutes))
	}

	if len(parts) == 0 {
		return "less than a minute"
	}

	// Join parts
	return joinStrings(parts, ", ")
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	res := parts[0]
	for _, p := range parts[1:] {
		res += sep + p
	}
	return res
}
