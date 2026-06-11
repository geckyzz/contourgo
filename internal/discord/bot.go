package discord

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/db"
)

type DiscordBot struct {
	Session         *discordgo.Session
	Config          *config.Config
	ConfigPath      string
	Commands        []*discordgo.ApplicationCommand
	OwnerID         string
	AnnounceChannel string
	DB              *db.DB
	ForceCheckChan  chan bool
	StartTime       time.Time
}

func NewDiscordBot(cfg *config.Config, configPath string, database *db.DB, forceCheckChan chan bool) (*DiscordBot, error) {
	dg, err := discordgo.New("Bot " + cfg.Discord.Token)
	if err != nil {
		return nil, err
	}

	bot := &DiscordBot{
		Session:         dg,
		Config:          cfg,
		ConfigPath:      configPath,
		AnnounceChannel: cfg.Discord.Announce.Channel,
		DB:              database,
		ForceCheckChan:  forceCheckChan,
		StartTime:       time.Now(),
	}

	dg.AddHandler(bot.onReady)
	dg.AddHandler(bot.onInteractionCreate)

	return bot, nil
}

func (b *DiscordBot) Start() error {
	return b.Session.Open()
}

func (b *DiscordBot) Stop() error {
	// Clean up guild commands on shutdown
	if b.Session.State.User != nil {
		cmds, err := b.Session.ApplicationCommands(b.Session.State.User.ID, b.Config.Discord.Server)
		if err == nil {
			for _, cmd := range cmds {
				b.Session.ApplicationCommandDelete(b.Session.State.User.ID, b.Config.Discord.Server, cmd.ID)
			}
		}
	}
	return b.Session.Close()
}

func (b *DiscordBot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("Bot logged in as %s#%s", r.User.Username, r.User.Discriminator)

	app, err := s.Application("@me")
	if err == nil && app.Owner != nil {
		b.OwnerID = app.Owner.ID
		log.Printf("Application owner identified as: %s", b.OwnerID)
	} else {
		log.Printf("Warning: could not fetch application owner: %v", err)
	}

	s.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name: "comments on trackers",
				Type: discordgo.ActivityTypeWatching,
			},
		},
	})

	// Print invite link on startup
	inviteURL := fmt.Sprintf("https://discord.com/oauth2/authorize?client_id=%s&permissions=8&integration_type=0&scope=bot+applications.commands", s.State.User.ID)
	log.Printf("🔗 Bot Invite URL: %s", inviteURL)

	// Check if bot has joined the target server
	guildJoined := false
	for _, g := range r.Guilds {
		if g.ID == b.Config.Discord.Server {
			guildJoined = true
			break
		}
	}

	if !guildJoined {
		log.Printf("⚠️ WARNING: The bot has not joined the target server (Guild ID: %s) yet. Please use the invite link above to add the bot with both 'bot' and 'applications.commands' scopes.", b.Config.Discord.Server)
	}

	// Register Guild Slash Commands
	b.registerSlashCommands(s)
}
