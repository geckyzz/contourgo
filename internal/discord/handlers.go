package discord

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

func (b *DiscordBot) sendFollowupEmbed(
	s *discordgo.Session,
	i *discordgo.Interaction,
	embed *discordgo.MessageEmbed,
) {
	_, err := s.FollowupMessageCreate(i, true, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
	})
	if err != nil {
		log.Printf("❌ Failed to send followup embed: %v", err)
	}
}
