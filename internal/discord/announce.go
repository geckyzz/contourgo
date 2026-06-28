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
	"github.com/geckyzz/contourgo/internal/db"
)

var nyaaMentionRegex = regexp.MustCompile(`\B@([a-zA-Z0-9-_]+)`)

func resolveNyaaMentions(message string, siteBase string) string {
	return nyaaMentionRegex.ReplaceAllString(
		message,
		fmt.Sprintf("[@$1](https://%s/user/$1)", siteBase),
	)
}

func (b *DiscordBot) EnqueueAnnouncement(
	service, channelID, torrentID, commentID, authorIconURL string,
	showCommentID, resolveImage bool,
	mentionsDisable bool,
) error {
	if channelID == "" {
		channelID = b.AnnounceChannel
	}
	return b.DB.EnqueueAnnouncement(
		service,
		channelID,
		torrentID,
		commentID,
		authorIconURL,
		showCommentID,
		resolveImage,
		mentionsDisable,
	)
}

func (b *DiscordBot) sendAnnouncementEmbed(
	channelID string,
	logMsg string,
	commentMsg string,
	mentionsDisable bool,
	embed *discordgo.MessageEmbed,
) error {
	log.Println(logMsg)
	content := b.GetMentionsForText(commentMsg, mentionsDisable)
	_, err := b.Session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: content,
		Embeds: []*discordgo.MessageEmbed{
			embed,
		},
	})
	return err
}

func trimDescription(s string) string {
	if len(s) > 4096 {
		return s[:4093] + "..."
	}
	return s
}

func appendReplyingToField(embed *discordgo.MessageEmbed, parentText string) {
	if parentText == "" {
		return
	}
	if len(parentText) > 1000 {
		parentText = parentText[:997] + "..."
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:  "↪️ Replying to",
		Value: parentText,
	})
}

func (b *DiscordBot) setEmbedTimestamp(embed *discordgo.MessageEmbed, timestamp int64) {
	if timestamp > 0 {
		embed.Timestamp = time.Unix(timestamp, 0).UTC().Format(time.RFC3339)
	}
}

func (b *DiscordBot) setEmbedThumbnail(
	embed *discordgo.MessageEmbed,
	avatarURL, defaultSuffix string,
) {
	if avatarURL != "" && (defaultSuffix == "" || !strings.HasSuffix(avatarURL, defaultSuffix)) {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: avatarURL,
		}
	}
}

func (b *DiscordBot) resolveEmbedImage(embed *discordgo.MessageEmbed, message, siteBase string) {
	if imgURL := extractImageURL(message); imgURL != "" {
		if strings.HasPrefix(imgURL, "/") && siteBase != "" {
			imgURL = "https://" + siteBase + imgURL
		}
		embed.Image = &discordgo.MessageEmbedImage{URL: imgURL}
		embed.Description = ""
	}
}

func (b *DiscordBot) BuildNyaaEmbed(
	service, authorIconURL string,
	torrent db.Torrent,
	comment db.Comment,
	showCommentID bool,
	resolveUserContentImage bool,
) *discordgo.MessageEmbed {
	var userAvatarURL, authorName, authorURL string
	var embedColor int

	siteBase := "nyaa.si"
	if service == "sukebei" {
		siteBase = "sukebei.nyaa.si"
	}

	commentURL := fmt.Sprintf(
		"https://%s/view/%s#com-%d",
		siteBase,
		torrent.TorrentID,
		comment.Position,
	)

	if comment.AvatarURL != "" {
		userAvatarURL = comment.AvatarURL
		if strings.HasPrefix(userAvatarURL, "/static/") {
			userAvatarURL = "https://" + siteBase + userAvatarURL
		}
	} else {
		userAvatarURL = "https://" + siteBase + "/static/img/avatar/default.png"
	}

	authorName = comment.Username
	if comment.UserRole != "" {
		authorName = fmt.Sprintf("%s (%s)", comment.Username, comment.UserRole)
	}

	authorURL = fmt.Sprintf("https://%s/user/%s", siteBase, urlPathEscape(comment.Username))

	if service == "sukebei" {
		embedColor = 0x003b73
	} else {
		embedColor = 0x0085ff
	}

	embed := &discordgo.MessageEmbed{
		Title:       trimField(authorName),
		URL:         authorURL,
		Color:       embedColor,
		Description: trimDescription(resolveNyaaMentions(comment.Message, siteBase)),
		Author: &discordgo.MessageEmbedAuthor{
			Name:    trimField(torrent.Title),
			URL:     commentURL,
			IconURL: authorIconURL,
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: strings.Title(service) + " Comments",
		},
	}

	if showCommentID {
		embed.Fields = []*discordgo.MessageEmbedField{
			{
				Name:   "Comment ID",
				Value:  fmt.Sprintf("[#%d](%s)", comment.Position, commentURL),
				Inline: true,
			},
		}
	}

	if resolveUserContentImage {
		b.resolveEmbedImage(embed, comment.Message, siteBase)
	}

	parentText := comment.ParentMessage
	if parentText == "" {
		matches := nyaaMentionRegex.FindAllStringSubmatch(comment.Message, -1)
		if len(matches) > 0 {
			var usernames []string
			seen := make(map[string]bool)
			for _, m := range matches {
				u := strings.ToLower(m[1])
				if !seen[u] {
					seen[u] = true
					usernames = append(usernames, m[1])
				}
			}
			if b.DB != nil {
				if parentComment, ok := b.DB.GetLatestCommentByUsersBeforePosition(service, torrent.TorrentID, usernames, comment.Position); ok {
					parentText = parentComment.Message
				}
			}
		}
	}

	appendReplyingToField(embed, parentText)

	b.setEmbedThumbnail(embed, userAvatarURL, "")
	b.setEmbedTimestamp(embed, comment.Timestamp)

	return embed
}

func (b *DiscordBot) AnnounceNyaaComment(
	channelID string,
	service string,
	torrent db.Torrent,
	comment db.Comment,
	authorIconURL string,
	showCommentID bool,
	resolveUserContentImage bool,
	mentionsDisable bool,
) error {
	if channelID == "" {
		return b.EnqueueAnnouncement(
			service,
			"",
			torrent.TorrentID,
			comment.CommentID,
			authorIconURL,
			showCommentID,
			resolveUserContentImage,
			mentionsDisable,
		)
	}

	embed := b.BuildNyaaEmbed(
		service,
		authorIconURL,
		torrent,
		comment,
		showCommentID,
		resolveUserContentImage,
	)

	logMsg := fmt.Sprintf(
		"[POST] Sending Nyaa/Sukebei Announcement embed to channel %s for torrent '%s'",
		channelID,
		torrent.Title,
	)
	return b.sendAnnouncementEmbed(channelID, logMsg, comment.Message, mentionsDisable, embed)
}

func (b *DiscordBot) BuildATEmbed(
	service string,
	torrent db.Torrent,
	comment db.Comment,
	showCommentID bool,
	resolveUserContentImage bool,
) *discordgo.MessageEmbed {
	siteBase := "animetosho.org"
	if strings.HasPrefix(service, "animetosho_new") {
		siteBase = "animetosho.xyz"
	}

	var torrentURL, commentURL string
	if strings.HasPrefix(torrent.TorrentID, "feedback") {
		torrentURL = fmt.Sprintf("https://%s/feedback", siteBase)
		commentURL = fmt.Sprintf(
			"https://%s/%s#comment%s",
			siteBase,
			torrent.TorrentID,
			comment.CommentID,
		)
	} else {
		torrentURL = fmt.Sprintf("https://%s/view/%s", siteBase, torrent.TorrentID)
		commentURL = fmt.Sprintf("https://%s/view/%s#comment%s", siteBase, torrent.TorrentID, comment.CommentID)
	}

	embedColor := 0x52d345
	if strings.HasPrefix(service, "animetosho_new") {
		embedColor = 0x4d4d4d
	}

	footerText := "AnimeTosho Comments"
	if strings.HasPrefix(service, "animetosho_new") {
		footerText = "AnimeTosho New Comments"
	} else if strings.HasPrefix(service, "animetosho_old") {
		footerText = "AnimeTosho Beta Comments"
	}

	embed := &discordgo.MessageEmbed{
		Title:       trimField(comment.Username),
		URL:         commentURL,
		Color:       embedColor,
		Description: trimDescription(comment.Message),
		Author: &discordgo.MessageEmbedAuthor{
			Name: trimField(torrent.Title),
			URL:  torrentURL,
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: footerText,
		},
	}

	if showCommentID {
		embed.Fields = []*discordgo.MessageEmbedField{
			{
				Name:   "Comment ID",
				Value:  fmt.Sprintf("[%s](%s)", comment.CommentID, commentURL),
				Inline: true,
			},
		}
	}

	if comment.ParentID != "" {
		appendReplyingToField(embed, comment.ParentMessage)
	}

	if resolveUserContentImage {
		b.resolveEmbedImage(embed, comment.Message, "")
	}

	b.setEmbedTimestamp(embed, comment.Timestamp)

	return embed
}

func (b *DiscordBot) AnnounceATComment(
	channelID string,
	service string,
	torrent db.Torrent,
	comment db.Comment,
	authorIconURL string,
	showCommentID bool,
	resolveUserContentImage bool,
	mentionsDisable bool,
) error {
	if channelID == "" {
		return b.EnqueueAnnouncement(
			service,
			"",
			torrent.TorrentID,
			comment.CommentID,
			authorIconURL,
			showCommentID,
			resolveUserContentImage,
			mentionsDisable,
		)
	}

	embed := b.BuildATEmbed(service, torrent, comment, showCommentID, resolveUserContentImage)

	logMsg := fmt.Sprintf(
		"[POST] Sending AnimeTosho Announcement embed to channel %s for torrent '%s'",
		channelID,
		torrent.Title,
	)
	return b.sendAnnouncementEmbed(channelID, logMsg, comment.Message, mentionsDisable, embed)
}

func (b *DiscordBot) BuildNekoBTEmbed(
	torrent db.Torrent,
	comment db.Comment,
	authorIconURL string,
	showCommentID bool,
	resolveUserContentImage bool,
	parentText string,
) *discordgo.MessageEmbed {
	jumpToCommentURL := fmt.Sprintf(
		"https://nekobt.to/torrents/%s?com=%s",
		torrent.TorrentID,
		comment.CommentID,
	)

	authorURL := fmt.Sprintf("https://nekobt.to/u/%s", urlPathEscape(comment.Username))
	description := resolveNekoBTMentions(comment.Message)

	embed := &discordgo.MessageEmbed{
		Title: trimField(comment.Username),
		URL:   authorURL,
		Author: &discordgo.MessageEmbedAuthor{
			Name:    trimField(torrent.Title),
			URL:     jumpToCommentURL,
			IconURL: authorIconURL,
		},
		Description: trimDescription(description),
		Color:       0x8c4fff, // nekoBT Purple
		Footer: &discordgo.MessageEmbedFooter{
			Text: "nekoBT Comments",
		},
	}

	if showCommentID {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  "Comment ID",
			Value: fmt.Sprintf("`%s`", comment.CommentID),
		})
	}

	if comment.ParentID != "" {
		appendReplyingToField(embed, resolveNekoBTMentions(parentText))
	}

	if resolveUserContentImage {
		b.resolveEmbedImage(embed, comment.Message, "nekobt.to")
	}

	b.setEmbedTimestamp(embed, comment.Timestamp)
	b.setEmbedThumbnail(embed, comment.AvatarURL, "")

	return embed
}

func (b *DiscordBot) AnnounceNekoBTComment(
	channelID string,
	torrent db.Torrent,
	comment db.Comment,
	authorIconURL string,
	showCommentID bool,
	resolveUserContentImage bool,
	mentionsDisable bool,
) error {
	if channelID == "" {
		return b.EnqueueAnnouncement(
			"nekobt",
			"",
			torrent.TorrentID,
			comment.CommentID,
			authorIconURL,
			showCommentID,
			resolveUserContentImage,
			mentionsDisable,
		)
	}

	parentText := comment.ParentMessage
	if parentText == "" && comment.ParentID != "" {
		if parent, ok := b.DB.GetComment("nekobt", torrent.TorrentID, comment.ParentID); ok {
			parentText = parent.Message
		}
	}

	embed := b.BuildNekoBTEmbed(
		torrent,
		comment,
		authorIconURL,
		showCommentID,
		resolveUserContentImage,
		parentText,
	)

	logMsg := fmt.Sprintf(
		"[POST] Announcing nekoBT comment on torrent '%s' to channel %s",
		torrent.Title,
		channelID,
	)
	return b.sendAnnouncementEmbed(channelID, logMsg, comment.Message, mentionsDisable, embed)
}

func (b *DiscordBot) BuildTsukihimeEmbed(
	torrent db.Torrent,
	comment db.Comment,
	authorIconURL string,
	showCommentID bool,
	resolveUserContentImage bool,
	parentText string,
) *discordgo.MessageEmbed {
	var torrentURL string
	if torrent.TorrentID == "feedback" {
		torrentURL = "https://tsukihime.org/feedback"
	} else {
		torrentURL = fmt.Sprintf("https://tsukihime.org/view/%s", torrent.TorrentID)
	}

	embed := &discordgo.MessageEmbed{
		Title: trimField(comment.Username),
		Author: &discordgo.MessageEmbedAuthor{
			Name: trimField(torrent.Title),
			URL:  torrentURL,
		},
		Description: trimDescription(comment.Message),
		Color:       0xf25aa6, // TsukiHime Pink
		Footer: &discordgo.MessageEmbedFooter{
			Text: "TsukiHime Comments",
		},
	}

	if showCommentID {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  "Comment ID",
			Value: fmt.Sprintf("`%s`", comment.CommentID),
		})
	}

	if resolveUserContentImage {
		b.resolveEmbedImage(embed, comment.Message, "tsukihime.org")
	}

	b.setEmbedTimestamp(embed, comment.Timestamp)
	b.setEmbedThumbnail(embed, comment.AvatarURL, "default.png")

	if comment.ParentID != "" {
		appendReplyingToField(embed, parentText)
	}

	return embed
}

func (b *DiscordBot) AnnounceTsukihimeComment(
	channelID string,
	torrent db.Torrent,
	comment db.Comment,
	authorIconURL string,
	showCommentID bool,
	resolveUserContentImage bool,
	mentionsDisable bool,
) error {
	if channelID == "" {
		return b.EnqueueAnnouncement(
			"tsukihime",
			"",
			torrent.TorrentID,
			comment.CommentID,
			authorIconURL,
			showCommentID,
			resolveUserContentImage,
			mentionsDisable,
		)
	}

	parentText := comment.ParentMessage
	if parentText == "" && comment.ParentID != "" {
		if parent, ok := b.DB.GetComment("tsukihime", torrent.TorrentID, comment.ParentID); ok {
			parentText = parent.Message
		}
	}

	embed := b.BuildTsukihimeEmbed(
		torrent,
		comment,
		authorIconURL,
		showCommentID,
		resolveUserContentImage,
		parentText,
	)

	logMsg := fmt.Sprintf(
		"[POST] Announcing TsukiHime comment on torrent '%s' to channel %s",
		torrent.Title,
		channelID,
	)
	return b.sendAnnouncementEmbed(channelID, logMsg, comment.Message, mentionsDisable, embed)
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
				req.Header.Set(
					"User-Agent",
					"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
				)
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
	return nekoBTMentionRegex.ReplaceAllString(text, "[@$1](https://nekobt.to/u/$1)")
}

func (b *DiscordBot) BuildAnirenaEmbed(
	torrent db.Torrent,
	comment db.Comment,
	authorIconURL string,
	showCommentID bool,
	resolveUserContentImage bool,
) *discordgo.MessageEmbed {
	siteBase := "www.anirena.com"
	commentURL := fmt.Sprintf(
		"https://%s/torrent/%s?tab=comments#tc-comment-%s",
		siteBase,
		torrent.TorrentID,
		comment.CommentID,
	)

	authorName := comment.Username
	if torrent.Uploader != "" && strings.EqualFold(comment.Username, torrent.Uploader) {
		authorName = fmt.Sprintf("%s (Uploader)", comment.Username)
	} else if comment.UserRole != "" && !strings.EqualFold(comment.UserRole, "user") {
		authorName = fmt.Sprintf("%s (%s)", comment.Username, comment.UserRole)
	}

	authorURL := fmt.Sprintf(
		"https://%s/?q=user%%253A%%22%s%%22",
		siteBase,
		urlPathEscape(comment.Username),
	)

	embedColor := 0xde3d20

	embed := &discordgo.MessageEmbed{
		Title:       trimField(authorName),
		URL:         authorURL,
		Color:       embedColor,
		Description: trimDescription(comment.Message),
		Author: &discordgo.MessageEmbedAuthor{
			Name: trimField(torrent.Title),
			URL:  commentURL,
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "AniRena Comments",
		},
	}

	if showCommentID {
		embed.Fields = []*discordgo.MessageEmbedField{
			{
				Name:   "Comment ID",
				Value:  fmt.Sprintf("[%s](%s)", comment.CommentID, commentURL),
				Inline: true,
			},
		}
	}

	if resolveUserContentImage {
		b.resolveEmbedImage(embed, comment.Message, "")
	}

	b.setEmbedTimestamp(embed, comment.Timestamp)

	return embed
}

func (b *DiscordBot) AnnounceAnirenaComment(
	channelID string,
	torrent db.Torrent,
	comment db.Comment,
	authorIconURL string,
	showCommentID bool,
	resolveUserContentImage bool,
	mentionsDisable bool,
) error {
	if channelID == "" {
		return b.EnqueueAnnouncement(
			"anirena",
			"",
			torrent.TorrentID,
			comment.CommentID,
			authorIconURL,
			showCommentID,
			resolveUserContentImage,
			mentionsDisable,
		)
	}

	embed := b.BuildAnirenaEmbed(
		torrent,
		comment,
		authorIconURL,
		showCommentID,
		resolveUserContentImage,
	)

	logMsg := fmt.Sprintf(
		"[POST] Sending AniRena Announcement embed to channel %s for torrent '%s'",
		channelID,
		torrent.Title,
	)
	return b.sendAnnouncementEmbed(channelID, logMsg, comment.Message, mentionsDisable, embed)
}

var (
	htmlTagRegex  = regexp.MustCompile(`<[^>]*>`)
	mdLinkRegex   = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	mdFormatRegex = regexp.MustCompile(`[*_~` + "`" + `]+`)
)

func sanitizeForLookup(text string) string {
	// 1. Markdown links: [text](url) -> text
	text = mdLinkRegex.ReplaceAllString(text, "$1")
	// 2. HTML tags: <... text ...> -> remove tags
	text = htmlTagRegex.ReplaceAllString(text, "")
	// 3. Remove common markdown formatting characters
	text = mdFormatRegex.ReplaceAllString(text, "")
	return text
}

func urlPathEscape(s string) string {
	return strings.ReplaceAll(url.PathEscape(s), "+", "%20")
}

func (b *DiscordBot) findMatchedMentions(text string, disabled bool) map[string]string {
	matched := make(map[string]string)
	if disabled || b.Config.Discord.Mentions == nil {
		return matched
	}
	cleanText := strings.ToLower(sanitizeForLookup(text))
	for find, snowflakeAny := range b.Config.Discord.Mentions {
		if strings.Contains(cleanText, "@"+strings.ToLower(find)) {
			matched[find] = fmt.Sprintf("%v", snowflakeAny)
		}
	}
	return matched
}

func (b *DiscordBot) GetMentionsForText(text string, disabled bool) string {
	matched := b.findMatchedMentions(text, disabled)
	if len(matched) == 0 {
		return ""
	}
	var mentions []string
	for _, snowflake := range matched {
		mentions = append(mentions, "<@"+snowflake+">")
	}
	return strings.Join(mentions, " ")
}

func (b *DiscordBot) ResolveMentionsPlain(text string, disabled bool) string {
	matched := b.findMatchedMentions(text, disabled)
	if len(matched) == 0 {
		return ""
	}
	var mentions []string
	for find := range matched {
		mentions = append(mentions, find)
	}
	return strings.Join(mentions, " ")
}

func trimField(s string) string {
	runes := []rune(s)
	if len(runes) > 253 {
		return string(runes[:253])
	}
	return s
}

func (b *DiscordBot) BuildNekoBTNotificationEmbed(
	service string,
	torrent db.Torrent,
	comment db.Comment,
) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       "NekoBT Notification",
		Description: comment.Message,
		Color:       0x8c4fff, // nekoBT Purple
		Footer: &discordgo.MessageEmbedFooter{
			Text: "nekoBT Notification",
		},
	}

	b.setEmbedTimestamp(embed, comment.Timestamp)

	return embed
}
