package discord

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"text/template"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/db"
)

func renderNekoBTNotificationFormat(format string, data any) (string, error) {
	tmpl, err := template.New("nekobt_notification").Parse(format)
	if err != nil {
		return "", fmt.Errorf("invalid template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing template: %w", err)
	}
	return buf.String(), nil
}

func (b *DiscordBot) queueWorker() {
	log.Println("[QUEUE] Starting announcement queue worker...")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			log.Println("[QUEUE] Queue worker stopped.")
			return
		case <-ticker.C:
			pending, err := b.DB.GetPendingAnnouncements()
			if err != nil {
				log.Printf("[QUEUE] Error fetching pending announcements: %v", err)
				continue
			}

			if len(pending) == 0 {
				continue
			}

			a := pending[0]

			isNotification := a.Service == "nekobt" &&
				(a.TorrentID == "notification" || a.TorrentID == "notifications")
			if !isNotification && (a.Torrent.Title == "" || a.Comment.Username == "") {
				log.Printf(
					"[QUEUE] Skipping announcement %d for %s: missing critical data (title or username)",
					a.ID,
					a.Service,
				)
				b.DB.MarkAnnouncementPosted(a.ID)
				continue
			}

			embed := b.buildEmbedForService(a)
			if embed == nil {
				log.Printf("[QUEUE] Unknown service %s for announcement %d", a.Service, a.ID)
				b.DB.MarkAnnouncementPosted(a.ID)
				continue
			}

			channelID := a.ChannelID
			if channelID == "" {
				channelID = b.AnnounceChannel
			}

			log.Printf(
				"[QUEUE] Posting announcement %d for %s (torrent: %s, channel: %s, retry: %d)",
				a.ID,
				a.Service,
				a.Torrent.Title,
				channelID,
				a.RetryCount,
			)

			var content string
			var monitorCfg config.MonitorConfig
			if inner, ok := b.Config.Monitors[a.Service]; ok {
				monitorCfg = inner[a.TorrentID]
			}

			if isNotification && monitorCfg.CustomFormat != "" {
				tplData := map[string]any{
					"Message": a.Comment.Message,
				}
				if rendered, err := renderNekoBTNotificationFormat(monitorCfg.CustomFormat, tplData); err == nil {
					content = rendered
				} else {
					log.Printf("[QUEUE] Error rendering custom format for notification: %v", err)
					content = b.GetMentionsForText(a.Comment.Message, a.MentionsDisable)
				}
			} else {
				content = b.GetMentionsForText(a.Comment.Message, a.MentionsDisable)
			}

			msgSend := &discordgo.MessageSend{
				Content: content,
				Embeds: []*discordgo.MessageEmbed{
					embed,
				},
			}
			if isNotification {
				customID := fmt.Sprintf(
					"nekobt_read:%s:%s:%s",
					a.Comment.CommentID,
					a.Service,
					a.TorrentID,
				)
				msgSend.Components = []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.Button{
								Label:    "Mark as Read",
								Style:    discordgo.SuccessButton,
								CustomID: customID,
							},
						},
					},
				}
			}

			_, err = b.Session.ChannelMessageSendComplex(channelID, msgSend)
			if err != nil {
				log.Printf("[QUEUE] Error sending embed for ID %d: %v", a.ID, err)
				b.DB.FailAnnouncement(a.ID, err.Error())
				continue
			}

			if err := b.DB.MarkAnnouncementPosted(a.ID); err != nil {
				log.Printf("[QUEUE] Error marking announcement %d as posted: %v", a.ID, err)
			}
		}
	}
}

func (b *DiscordBot) resolveParentText(a db.QueuedAnnouncement) string {
	parentText := a.Comment.ParentMessage
	if parentText == "" && a.Comment.ParentID != "" {
		if parent, ok := b.DB.GetComment(a.Service, a.TorrentID, a.Comment.ParentID); ok {
			parentText = parent.Message
		}
	}
	return parentText
}

func (b *DiscordBot) buildEmbedForService(a db.QueuedAnnouncement) *discordgo.MessageEmbed {
	switch {
	case a.Service == "nyaa" || a.Service == "sukebei":
		return b.BuildNyaaEmbed(
			a.Service,
			a.AuthorIconURL,
			a.Torrent,
			a.Comment,
			a.ShowCommentID,
			a.ResolveImage,
		)
	case strings.HasPrefix(a.Service, "animetosho"):
		return b.BuildATEmbed(a.Service, a.Torrent, a.Comment, a.ShowCommentID, a.ResolveImage)
	case a.Service == "nekobt":
		if a.TorrentID == "notification" || a.TorrentID == "notifications" {
			return b.BuildNekoBTNotificationEmbed(
				a.Service,
				a.Torrent,
				a.Comment,
			)
		}
		return b.BuildNekoBTEmbed(
			a.Torrent,
			a.Comment,
			a.AuthorIconURL,
			a.ShowCommentID,
			a.ResolveImage,
			b.resolveParentText(a),
		)
	case a.Service == "anirena":
		return b.BuildAnirenaEmbed(
			a.Torrent,
			a.Comment,
			a.AuthorIconURL,
			a.ShowCommentID,
			a.ResolveImage,
		)
	case a.Service == "tsukihime":
		return b.BuildTsukihimeEmbed(
			a.Torrent,
			a.Comment,
			a.AuthorIconURL,
			a.ShowCommentID,
			a.ResolveImage,
			b.resolveParentText(a),
		)
	default:
		return nil
	}
}
