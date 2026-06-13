package discord

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/bwmarrin/discordgo"
	"github.com/geckyzz/contourgo/internal/crypto"
)

type OldComment struct {
	ID        int   `json:"id"`
	Pos       int   `json:"pos"`
	Timestamp int64 `json:"timestamp"`
	User      struct {
		Username string `json:"username"`
		Image    string `json:"image"`
	} `json:"user"`
	Message string `json:"message"`
}

func (b *DiscordBot) handleSlashImport(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	service := optionMap["service"].StringValue()
	attachmentID := optionMap["file"].Value.(string)
	key := ""
	if opt, ok := optionMap["key"]; ok {
		key = opt.StringValue()
	}

	var attachment *discordgo.MessageAttachment
	for _, a := range i.ApplicationCommandData().Resolved.Attachments {
		if a.ID == attachmentID {
			attachment = a
			break
		}
	}

	if attachment == nil {
		b.sendFollowupMessage(s, i.Interaction, "❌ Attachment not found in resolved data.")
		return
	}

	b.sendFollowupMessage(
		s,
		i.Interaction,
		fmt.Sprintf("📥 Downloading legacy database dump `%s`...", attachment.Filename),
	)

	resp, err := http.Get(attachment.URL)
	if err != nil {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Failed to download attachment: %v", err),
		)
		return
	}
	defer resp.Body.Close()

	fileData, err := io.ReadAll(resp.Body)
	if err != nil {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Failed to read attachment content: %v", err),
		)
		return
	}

	var jsonBytes []byte
	if key != "" {
		b.sendFollowupMessage(s, i.Interaction, "🔑 Decrypting data...")
		jsonBytes, err = crypto.DecryptAndDecompress(fileData, key)
		if err != nil {
			b.sendFollowupMessage(
				s,
				i.Interaction,
				fmt.Sprintf("❌ Decryption/Decompression failed: %v. Check key.", err),
			)
			return
		}
	} else {
		jsonBytes = fileData
	}

	var oldData map[string][]OldComment
	if err := json.Unmarshal(jsonBytes, &oldData); err != nil {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Failed to parse JSON database structure: %v", err),
		)
		return
	}

	b.sendFollowupMessage(
		s,
		i.Interaction,
		fmt.Sprintf("⚙️ Importing data for service `%s`...", service),
	)

	importedTorrents := 0
	importedComments := 0

	for torrentID, comments := range oldData {
		count := len(comments)
		title := fmt.Sprintf("Imported Torrent %s", torrentID)

		err = b.DB.UpdateTorrent(service, torrentID, title, count, 0, "")
		if err != nil {
			continue
		}
		importedTorrents++

		for _, c := range comments {
			commentIDStr := strconv.Itoa(c.ID)
			b.DB.StoreComment(
				service,
				torrentID,
				commentIDStr,
				c.User.Username,
				c.Message,
				c.Timestamp,
				c.Pos,
				"",
				c.User.Image,
				"",
				"",
			)
			importedComments++
		}
	}

	b.sendFollowupMessage(
		s,
		i.Interaction,
		fmt.Sprintf(
			"✅ **Import Completed!** Imported **%d** torrents and **%d** comments for service `%s`.",
			importedTorrents,
			importedComments,
			service,
		),
	)
}
