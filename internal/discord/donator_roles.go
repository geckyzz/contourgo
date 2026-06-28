package discord

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"text/template"
	"time"

	"github.com/bwmarrin/discordgo"
)

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

		// Send expired notification DM using beautiful Embed (unless silenced or below threshold)
		if !b.Config.Donation.Silent.Globally && !b.Config.Donation.Silent.OnExpiry &&
			b.hasDonatorRole(donator.TotalUSD) {
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
				// If expiry is within the warning threshold and they haven't been warned yet (and they qualify for a role)
				if donator.ExpiryTime-now <= warnSecondsThreshold && donator.WarnedAt == 0 &&
					b.hasDonatorRole(donator.TotalUSD) {
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

func (b *DiscordBot) getServerName(s *discordgo.Session) string {
	serverID := string(b.Config.Discord.Server)
	if guild, err := s.State.Guild(serverID); err == nil && guild != nil {
		return guild.Name
	}
	if guild, err := s.Guild(serverID); err == nil && guild != nil {
		return guild.Name
	}
	return "our server"
}

func (b *DiscordBot) getHighestRoleID(totalUSD float64, isActive bool) string {
	if !isActive {
		return ""
	}
	var highestRole string
	var maxAmount float64 = -1.0
	for tierRole, minAmount := range b.Config.Donation.Tiers {
		if tierRole == "" {
			continue
		}
		if totalUSD >= minAmount && minAmount > maxAmount {
			maxAmount = minAmount
			highestRole = tierRole
		}
	}
	return highestRole
}

func (b *DiscordBot) hasDonatorRole(totalUSD float64) bool {
	return b.getHighestRoleID(totalUSD, true) != ""
}

func (b *DiscordBot) buildTplData(
	s *discordgo.Session,
	username, userID string,
	totalUSD float64,
	expiry int64,
) map[string]any {
	expiryStr := "Expired"
	if expiry > 0 {
		expiryStr = time.Unix(expiry, 0).Format("January 2, 2006 at 3:04 PM MST")
	}
	return map[string]any{
		"Username":        username,
		"UserID":          userID,
		"Cumulative":      fmt.Sprintf("%.2f %s", totalUSD, b.getCurrency()),
		"CumulativeValue": totalUSD,
		"Expiry":          expiryStr,
		"ExpiryUnix":      expiry,
		"ServerName":      b.getServerName(s),
		"RoleID":          b.getHighestRoleID(totalUSD, expiry > time.Now().Unix()),
	}
}

func (b *DiscordBot) getCurrency() string {
	if b.Config.Donation.Currency != "" {
		return b.Config.Donation.Currency
	}
	return "USD"
}
