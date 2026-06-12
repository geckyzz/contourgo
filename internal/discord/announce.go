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

func (b *DiscordBot) AnnounceNyaaComment(channelID string, service, torrentID, torrentTitle string, comment scraper.NyaaComment, authorIconURL string, showCommentID bool) error {
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
		embedColor = 0x003b73
	} else {
		embedColor = 0x0085ff
	}

	description := comment.Text
	if len(description) > 4096 {
		description = description[:4093] + "..."
	}

	torrentURL := fmt.Sprintf("https://%s/view/%s#com-%d", siteBase, torrentID, comment.Pos)

	embed := &discordgo.MessageEmbed{
		Title:       trimField(authorName),
		URL:         authorURL,
		Color:       embedColor,
		Description: description,
		Author: &discordgo.MessageEmbedAuthor{
			Name:    trimField(torrentTitle),
			URL:     torrentURL,
			IconURL: authorIconURL,
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: strings.Title(service) + " Comments",
		},
	}

	if showCommentID {
		embed.Fields = []*discordgo.MessageEmbedField{
			{Name: "Comment ID", Value: fmt.Sprintf("[#%d](%s)", comment.Pos, commentURL), Inline: true},
		}
	}

	if imgURL := extractImageURL(comment.Text); imgURL != "" {
		if strings.HasPrefix(imgURL, "/") {
			imgURL = "https://" + siteBase + imgURL
		}
		embed.Image = &discordgo.MessageEmbedImage{URL: imgURL}
		embed.Description = ""
	}

	if userAvatarURL != "" && !strings.HasSuffix(userAvatarURL, "default.png") {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: userAvatarURL,
		}
	}

	if comment.Timestamp != "" {
		t, err := time.Parse(time.RFC3339, ensureUTC(comment.Timestamp))
		if err == nil {
			embed.Timestamp = t.Format(time.RFC3339)
		}
	}

	log.Printf("[POST] Sending Nyaa/Sukebei Announcement embed to channel %s for torrent '%s'", targetChannel, torrentTitle)
	_, err := b.Session.ChannelMessageSendEmbed(targetChannel, embed)
	return err
}

func (b *DiscordBot) AnnounceATComment(channelID string, service string, torrentID, torrentTitle string, comment scraper.ATComment, authorIconURL string, showCommentID bool) error {
	targetChannel := b.AnnounceChannel
	if channelID != "" {
		targetChannel = channelID
	}

	siteBase := "animetosho.org"
	if strings.HasPrefix(service, "animetosho_new") {
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

	embedColor := 0x52d345
	if strings.HasPrefix(service, "animetosho_new") {
		embedColor = 0x4d4d4d
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
		Footer: &discordgo.MessageEmbedFooter{
			Text: "AnimeTosho Comments",
		},
	}

	if showCommentID {
		embed.Fields = []*discordgo.MessageEmbedField{
			{Name: "Comment ID", Value: fmt.Sprintf("[%s](%s)", comment.ID, commentURL), Inline: true},
		}
	}

	if comment.Timestamp > 0 {
		embed.Timestamp = time.Unix(comment.Timestamp, 0).UTC().Format(time.RFC3339)
	}

	log.Printf("[POST] Sending AnimeTosho Announcement embed to channel %s for torrent '%s'", targetChannel, torrentTitle)
	_, err := b.Session.ChannelMessageSendEmbed(targetChannel, embed)
	return err
}

func (b *DiscordBot) AnnounceNekoBTComment(channelID string, torrentTitle string, comment scraper.NekoBTComment, authorIconURL string, showCommentID bool) error {
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
			IconURL: authorIconURL,
		},
		Description: description,
		Color:       0x8c4fff, // nekoBT Purple
		Footer: &discordgo.MessageEmbedFooter{
			Text: "nekoBT Comments",
		},
	}

	if showCommentID {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  "Comment ID",
			Value: fmt.Sprintf("`%s`", comment.ID),
		})
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

	if userAvatarURL != "" && !strings.HasSuffix(userAvatarURL, "/null") {
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

func (b *DiscordBot) AnnounceTsukihimeComment(channelID string, torrentTitle string, comment scraper.TsukihimeComment, parentText string, authorIconURL string, showCommentID bool) error {
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
			Name: trimField(torrentTitle),
			URL:  torrentURL,
		},
		Description: description,
		Color:       0xf25aa6, // TsukiHime Pink
		Footer: &discordgo.MessageEmbedFooter{
			Text: "TsukiHime Comments",
		},
	}

	if showCommentID {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  "Comment ID",
			Value: fmt.Sprintf("`%s`", comment.GetID()),
		})
	}

	if imgURL := extractImageURL(comment.GetText()); imgURL != "" {
		if strings.HasPrefix(imgURL, "/") {
			imgURL = "https://tsukihime.org" + imgURL
		}
		embed.Image = &discordgo.MessageEmbedImage{URL: imgURL}
		embed.Description = ""
	}

	if comment.CreatedAt != "" {
		t, err := time.Parse(time.RFC3339, ensureUTC(comment.CreatedAt))
		if err == nil {
			embed.Timestamp = t.Format(time.RFC3339)
		}
	}

	if userAvatarURL != "" && !strings.HasSuffix(userAvatarURL, "default.png") {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: userAvatarURL,
		}
	}

	if comment.GetParentID() != "" && parentText != "" {
		// parentText = resolveTsukihimeMentions(parentText) // Removed as there are no user pages
		if len(parentText) > 1000 {
			parentText = parentText[:997] + "..."
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  "↪️ Replying to",
			Value: parentText,
		})
	}

	log.Printf("[POST] Announcing TsukiHime comment on torrent '%s' to channel %s", torrentTitle, targetChannel)
	_, err := b.Session.ChannelMessageSendEmbed(targetChannel, embed)
	return err
}

func ensureUTC(s string) string {
	if s == "" {
		return ""
	}
	s = strings.Replace(s, " ", "T", 1)
	// If it doesn't end with Z and doesn't contain a + or - offset near the end
	if !strings.HasSuffix(s, "Z") && !regexp.MustCompile(`[+-]\d{2}:\d{2}$`).MatchString(s) {
		return s + "Z"
	}
	return s
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

func (b *DiscordBot) AnnounceAnirenaComment(channelID, torrentID, torrentTitle string, comment scraper.AnirenaComment, authorIconURL string, torrentUploader string, showCommentID bool) error {
	targetChannel := b.AnnounceChannel
	if channelID != "" {
		targetChannel = channelID
	}

	siteBase := "www.anirena.com"
	commentURL := fmt.Sprintf("https://%s/torrent/%s?tab=comments#tc-comment-%s", siteBase, torrentID, comment.ID)

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

	authorURL := fmt.Sprintf("https://%s/?q=user%%253A%%22%s%%22", siteBase, urlPathEscape(comment.Username))

	embedColor := 0xde3d20

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
		Footer: &discordgo.MessageEmbedFooter{
			Text: "AniRena Comments",
		},
	}

	if showCommentID {
		embed.Fields = []*discordgo.MessageEmbedField{
			{Name: "Comment ID", Value: fmt.Sprintf("[%s](%s)", comment.ID, commentURL), Inline: true},
		}
	}

	if comment.CreatedAt != "" {
		t, err := time.Parse(time.RFC3339, ensureUTC(comment.CreatedAt))
		if err == nil {
			embed.Timestamp = t.Format(time.RFC3339)
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
