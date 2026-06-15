package discord

import (
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/geckyzz/contourgo/internal/db"
)

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

			if a.Torrent.Title == "" || a.Comment.Username == "" {
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

			content := b.GetMentionsForText(a.Comment.Message, a.MentionsDisable)
			_, err = b.Session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Content: content,
				Embeds: []*discordgo.MessageEmbed{
					embed,
				},
			})
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
