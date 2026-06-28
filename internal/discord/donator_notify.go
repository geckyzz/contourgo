package discord

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

func (b *DiscordBot) sendDonatorNotificationEmbed(
	s *discordgo.Session,
	user *discordgo.User,
	amount float64,
	totalUSD float64,
	duration string,
	expiry int64,
	isRenewal bool,
	account string,
	note string,
	addedTime int64,
) {
	channel, err := s.UserChannelCreate(user.ID)
	if err != nil {
		log.Printf("⚠️ Could not create DM channel for user %s: %v", user.ID, err)
		return
	}

	expiryStr := time.Unix(expiry, 0).Format("January 2, 2006 at 3:04 PM MST")

	tplData := b.buildTplData(s, user.Username, user.ID, totalUSD, expiry)
	tplData["Amount"] = fmt.Sprintf("%.2f %s", amount, b.getCurrency())
	tplData["AmountValue"] = amount
	tplData["Duration"] = duration
	tplData["Account"] = account
	tplData["Note"] = note
	tplData["AddedDate"] = time.Unix(addedTime, 0).Format("January 2, 2006 at 3:04 PM MST")
	tplData["AddedUnix"] = addedTime

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
			"💖 Donation Received{{if .RoleID}} & Perks Activated{{end}}!",
		)
		desc = b.renderDonationTemplate(
			b.Config.Donation.Format.Add.Desc,
			tplData,
			"Thank you so much for your support! Your donation has been recorded{{if .RoleID}}, and your perks are now active{{end}}.",
		)
	}

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Color:       0xe91e63, // Vibrant Pink
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Amount Added",
				Value:  fmt.Sprintf("%.2f %s", amount, b.getCurrency()),
				Inline: true,
			},
			{
				Name:   "Cumulative Total",
				Value:  fmt.Sprintf("%.2f %s", totalUSD, b.getCurrency()),
				Inline: true,
			},
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

	var totalUSD float64
	if donator, ok := b.DB.GetDonator(userID); ok {
		totalUSD = donator.TotalUSD
	}

	tplData := b.buildTplData(s, username, userID, totalUSD, expiry)
	tplData["TimeLeft"] = timeLeftStr

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
	var totalUSD float64
	if donator, ok := b.DB.GetDonator(userID); ok {
		expiry = donator.ExpiryTime
		totalUSD = donator.TotalUSD
	}

	tplData := b.buildTplData(s, username, userID, totalUSD, expiry)

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
