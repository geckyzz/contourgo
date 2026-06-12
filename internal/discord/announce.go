package discord

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/geckyzz/contourgo/internal/scraper"
)

func (b *DiscordBot) AnnounceNyaaComment(channelID string, service, torrentID, torrentTitle string, comment scraper.NyaaComment, embedThumbnail string) error {
	targetChannel := b.AnnounceChannel
	if channelID != "" {
		targetChannel = channelID
	}

	var commentURL, userAvatarURL, authorName, authorURL string
	var embedColor int

	siteBase := "nyaa.si"
	if service == "sukebei" {
		siteBase = "sukebei.nyaa.si"
	}

	commentURL = fmt.Sprintf("https://%s/view/%s#com-%d", siteBase, torrentID, comment.Pos)

	if comment.Avatar != "" {
		userAvatarURL = comment.Avatar
		if strings.HasPrefix(userAvatarURL, "/static/") {
			userAvatarURL = "https://" + siteBase + userAvatarURL
		}
	} else {
		userAvatarURL = "https://" + siteBase + "/static/img/avatar/default.png"
	}

	authorName = comment.Username
	if comment.Role != "" {
		authorName = fmt.Sprintf("%s (%s)", comment.Username, comment.Role)
	}

	authorURL = fmt.Sprintf("https://%s/user/%s", siteBase, urlPathEscape(comment.Username))

	if service == "sukebei" {
		embedColor = 0xff0051
	} else {
		embedColor = 0x0085ff
	}

	description := comment.Text
	if len(description) > 4096 {
		description = description[:4093] + "..."
	}

	torrentURL := fmt.Sprintf("https://%s/view/%s", siteBase, torrentID)

	embed := &discordgo.MessageEmbed{
		Title:       trimField(authorName),
		URL:         authorURL,
		Color:       embedColor,
		Description: description,
		Author: &discordgo.MessageEmbedAuthor{
			Name:    trimField(torrentTitle),
			URL:     torrentURL,
			IconURL: userAvatarURL,
		},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Position", Value: fmt.Sprintf("[#%d](%s)", comment.Pos, commentURL), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: strings.Title(service) + " Comments",
		},
	}

	if imgURL := extractImageURL(comment.Text); imgURL != "" {
		if strings.HasPrefix(imgURL, "/") {
			imgURL = "https://" + siteBase + imgURL
		}
		embed.Image = &discordgo.MessageEmbedImage{URL: imgURL}
		embed.Description = ""
	}

	if embedThumbnail != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: embedThumbnail,
		}
	} else if userAvatarURL != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: userAvatarURL,
		}
	}

	if comment.Timestamp != "" {
		t, err := time.Parse(time.RFC3339, comment.Timestamp)
		if err == nil {
			embed.Timestamp = t.Format(time.RFC3339)
		}
	}

	log.Printf("[POST] Sending Nyaa/Sukebei Announcement embed to channel %s for torrent '%s'", targetChannel, torrentTitle)
	_, err := b.Session.ChannelMessageSendEmbed(targetChannel, embed)
	return err
}

func (b *DiscordBot) AnnounceATComment(channelID string, service string, torrentID, torrentTitle string, comment scraper.ATComment, embedThumbnail string) error {
	targetChannel := b.AnnounceChannel
	if channelID != "" {
		targetChannel = channelID
	}

	siteBase := "animetosho.org"
	if service == "animetosho_new" {
		siteBase = "animetosho.xyz"
	}

	var torrentURL, commentURL string
	if strings.HasPrefix(torrentID, "feedback") {
		torrentURL = fmt.Sprintf("https://%s/feedback", siteBase)
		commentURL = fmt.Sprintf("https://%s/%s#comment%s", siteBase, torrentID, comment.ID)
	} else {
		torrentURL = fmt.Sprintf("https://%s/view/%s", siteBase, torrentID)
		commentURL = fmt.Sprintf("https://%s/view/%s#comment%s", siteBase, torrentID, comment.ID)
	}

	description := comment.Message
	if len(description) > 4096 {
		description = description[:4093] + "..."
	}

	var embedImage *discordgo.MessageEmbedImage
	if imgURL := extractImageURL(comment.Message); imgURL != "" {
		embedImage = &discordgo.MessageEmbedImage{URL: imgURL}
		description = ""
	}

	embedColor := 0x00073a
	if service == "animetosho_new" {
		embedColor = 0x60a0c0
	}

	embed := &discordgo.MessageEmbed{
		Title:       trimField(comment.Username),
		URL:         commentURL,
		Color:       embedColor,
		Description: description,
		Image:       embedImage,
		Author: &discordgo.MessageEmbedAuthor{
			Name: trimField(torrentTitle),
			URL:  torrentURL,
		},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Comment ID", Value: fmt.Sprintf("[%s](%s)", comment.ID, commentURL), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "AnimeTosho Comments",
		},
	}

	if embedThumbnail != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: embedThumbnail,
		}
	}

	if comment.Timestamp > 0 {
		embed.Timestamp = time.Unix(comment.Timestamp, 0).UTC().Format(time.RFC3339)
	}

	log.Printf("[POST] Sending AnimeTosho Announcement embed to channel %s for torrent '%s'", targetChannel, torrentTitle)
	_, err := b.Session.ChannelMessageSendEmbed(targetChannel, embed)
	return err
}

func (b *DiscordBot) AnnounceNekoBTComment(channelID string, torrentTitle string, comment scraper.NekoBTComment, embedThumbnail string) error {
	targetChannel := b.AnnounceChannel
	if channelID != "" {
		targetChannel = channelID
	}

	torrentURL := fmt.Sprintf("https://nekobt.to/view/%s", comment.TorrentID)
	userURL := fmt.Sprintf("https://nekobt.to/users/%s", comment.UserID)

	pfpHash := "null"
	if comment.PfpHash != nil && *comment.PfpHash != "" {
		pfpHash = *comment.PfpHash
	}
	userAvatarURL := fmt.Sprintf("https://nekobt.to/cdn/pfp/%s", pfpHash)

	displayName := comment.DisplayName
	if comment.UserID == comment.UploaderID {
		displayName = fmt.Sprintf("%s (Uploader)", displayName)
	} else {
		for _, cid := range comment.ContributorIDs {
			if comment.UserID == cid {
				displayName = fmt.Sprintf("%s (Contributor)", displayName)
				break
			}
		}
	}

	description := resolveNekoBTMentions(comment.Text)
	if len(description) > 4096 {
		description = description[:4093] + "..."
	}

	embed := &discordgo.MessageEmbed{
		Title: trimField(displayName),
		URL:   userURL,
		Author: &discordgo.MessageEmbedAuthor{
			Name:    trimField(torrentTitle),
			URL:     torrentURL,
			IconURL: userAvatarURL,
		},
		Description: description,
		Color:       0xfc913a, // nekoBT Orange
		Footer: &discordgo.MessageEmbedFooter{
			Text: "nekoBT Comments",
		},
	}

	if imgURL := extractImageURL(comment.Text); imgURL != "" {
		if strings.HasPrefix(imgURL, "/") {
			imgURL = "https://nekobt.to" + imgURL
		}
		embed.Image = &discordgo.MessageEmbedImage{URL: imgURL}
		embed.Description = ""
	}

	if comment.CreatedAt > 0 {
		embed.Timestamp = time.Unix(comment.CreatedAt/1000, (comment.CreatedAt%1000)*1000000).UTC().Format(time.RFC3339)
	}

	if embedThumbnail != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: embedThumbnail,
		}
	} else if userAvatarURL != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: userAvatarURL,
		}
	}

	// Add parent comment field if it's a reply
	if comment.ReplyingTo != nil && *comment.ReplyingTo != "" && comment.ParentText != "" {
		parentText := resolveNekoBTMentions(comment.ParentText)
		if len(parentText) > 1000 {
			parentText = parentText[:997] + "..."
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  "↪️ Replying to",
			Value: parentText,
		})
	}

	log.Printf("[POST] Announcing nekoBT comment on torrent '%s' to channel %s", torrentTitle, targetChannel)
	_, err := b.Session.ChannelMessageSendEmbed(targetChannel, embed)
	return err
}

func (b *DiscordBot) AnnounceTsukihimeComment(channelID string, torrentTitle string, comment scraper.TsukihimeComment, parentText string, embedThumbnail string) error {
	targetChannel := b.AnnounceChannel
	if channelID != "" {
		targetChannel = channelID
	}

	var torrentURL string
	if comment.TargetType == "feedback" {
		torrentURL = "https://tsukihime.org/feedback"
	} else {
		torrentURL = fmt.Sprintf("https://tsukihime.org/view/%s", comment.GetTargetID())
	}
	// userURL := fmt.Sprintf("https://tsukihime.org/u/%s", comment.GetUsername())

	var userAvatarURL string
	if comment.Author != nil && comment.Author.AvatarHash != "" {
		userAvatarURL = fmt.Sprintf("https://tsukihime.org/cdn/pfp/%s", comment.Author.AvatarHash)
	} else {
		userAvatarURL = "https://tsukihime.org/static/img/avatar/default.png"
	}

	displayName := comment.GetDisplayName()
	description := comment.GetText() // Removed resolveTsukihimeMentions
	if len(description) > 4096 {
		description = description[:4093] + "..."
	}

	embed := &discordgo.MessageEmbed{
		Title: trimField(displayName),
		// URL:   userURL, // Removed as there are no user pages
		Author: &discordgo.MessageEmbedAuthor{
			Name:    trimField(torrentTitle),
			URL:     torrentURL,
			IconURL: userAvatarURL,
		},
		Description: description,
		Color:       0xf25aa6, // TsukiHime Pink
		Footer: &discordgo.MessageEmbedFooter{
			Text: "TsukiHime Comments",
		},
	}

	if imgURL := extractImageURL(comment.GetText()); imgURL != "" {
		if strings.HasPrefix(imgURL, "/") {
			imgURL = "https://tsukihime.org" + imgURL
		}
		embed.Image = &discordgo.MessageEmbedImage{URL: imgURL}
		embed.Description = ""
	}

	if comment.CreatedAt != "" {
		t, err := time.Parse(time.RFC3339, comment.CreatedAt)
		if err == nil {
			embed.Timestamp = t.Format(time.RFC3339)
		}
	}

	if embedThumbnail != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: embedThumbnail,
		}
	} else if userAvatarURL != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: userAvatarURL,
		}
	}

	if comment.GetParentID() != "" && parentText != "" {
		// parentText = resolveTsukihimeMentions(parentText) // Removed as there are no user pages
		if len(parentText) > 1000 {
			parentText = parentText[:997] + "..."
		}
		embed.Fields = []*discordgo.MessageEmbedField{
			{
				Name:  "↪️ Replying to",
				Value: parentText,
			},
		}
	}

	log.Printf("[POST] Announcing TsukiHime comment on torrent '%s' to channel %s", torrentTitle, targetChannel)
	_, err := b.Session.ChannelMessageSendEmbed(targetChannel, embed)
	return err
}

func resolveTsukihimeMentions(text string) string {
	return text // Just return text as is
}

var nekoBTMentionRegex = regexp.MustCompile(`@([a-zA-Z0-9_-]+)`)

var (
	mdImageRegex = regexp.MustCompile(`(?i)^!\[.*?\]\((.+?)(?:\s+".*?")?\)$`)
	bbImageRegex = regexp.MustCompile(`(?i)^\[img\](.*?)\[/img\]$`)
)

func extractImageURL(text string) string {
	text = strings.TrimSpace(text)
	if matches := mdImageRegex.FindStringSubmatch(text); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	if matches := bbImageRegex.FindStringSubmatch(text); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
		if !strings.Contains(text, " ") && !strings.Contains(text, "\n") {
			client := &http.Client{
				Timeout: 5 * time.Second,
			}
			req, err := http.NewRequest("GET", text, nil)
			if err == nil {
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
				resp, err := client.Do(req)
				if err == nil {
					defer resp.Body.Close()
					contentType := resp.Header.Get("Content-Type")
					if strings.HasPrefix(contentType, "image/") {
						return text
					}
				}
			}
		}
	}
	return ""
}

func resolveNekoBTMentions(text string) string {
	return nekoBTMentionRegex.ReplaceAllString(text, "[@$1](https://nekobt.moe/u/$1)")
}

func (b *DiscordBot) AnnounceAnirenaComment(channelID, torrentID, torrentTitle string, comment scraper.AnirenaComment, embedThumbnail string, torrentUploader string) error {
	targetChannel := b.AnnounceChannel
	if channelID != "" {
		targetChannel = channelID
	}

	siteBase := "anirena.com"
	commentURL := fmt.Sprintf("https://www.%s/view/%s", siteBase, torrentID)

	description := comment.Body
	if len(description) > 4096 {
		description = description[:4093] + "..."
	}

	var embedImage *discordgo.MessageEmbedImage
	if imgURL := extractImageURL(comment.Body); imgURL != "" {
		embedImage = &discordgo.MessageEmbedImage{URL: imgURL}
		description = ""
	}

	authorName := comment.Username
	if torrentUploader != "" && strings.EqualFold(comment.Username, torrentUploader) {
		authorName = fmt.Sprintf("%s (Uploader)", comment.Username)
	} else if comment.Role != "" && !strings.EqualFold(comment.Role, "user") {
		authorName = fmt.Sprintf("%s (%s)", comment.Username, comment.Role)
	}

	authorURL := fmt.Sprintf("https://www.%s/user/%s", siteBase, urlPathEscape(comment.Username))

	embedColor := 0x4f46e5

	embed := &discordgo.MessageEmbed{
		Title:       trimField(authorName),
		URL:         authorURL,
		Color:       embedColor,
		Description: description,
		Image:       embedImage,
		Author: &discordgo.MessageEmbedAuthor{
			Name: trimField(torrentTitle),
			URL:  commentURL,
		},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Comment ID", Value: fmt.Sprintf("`%s`", comment.ID), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "AniRena Comments",
		},
	}

	if embedThumbnail != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: embedThumbnail,
		}
	}

	if comment.CreatedAt != "" {
		t, err := time.Parse("2006-01-02 15:04:05", comment.CreatedAt)
		if err == nil {
			embed.Timestamp = t.UTC().Format(time.RFC3339)
		}
	}

	log.Printf("[POST] Sending AniRena Announcement embed to channel %s for torrent '%s'", targetChannel, torrentTitle)
	_, err := b.Session.ChannelMessageSendEmbed(targetChannel, embed)
	return err
}

func urlPathEscape(s string) string {
	return strings.ReplaceAll(url.PathEscape(s), "+", "%20")
}

func trimField(s string) string {
	runes := []rune(s)
	if len(runes) > 253 {
		return string(runes[:253])
	}
	return s
}
