package discord

import (
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/db"
)

var (
	Version      = "0.10.1"
	CommitSHA    = "unknown"
	RepoOverride = "" // Can be set via ldflags: -X github.com/geckyzz/contourgo/internal/discord.RepoOverride=owner/repo
)

func GetRepoInfo() (owner, repo string) {
	// 1. Check ldflags override
	if RepoOverride != "" {
		parts := strings.Split(RepoOverride, "/")
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	}

	// 2. Check VCS URL from build info
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.url" {
				// Example: https://github.com/owner/repo.git or git@github.com:owner/repo.git
				url := setting.Value
				url = strings.TrimSuffix(url, ".git")
				url = strings.TrimPrefix(url, "https://")
				url = strings.TrimPrefix(url, "git@")
				url = strings.Replace(url, ":", "/", 1)

				parts := strings.Split(url, "/")
				// Expected parts: [github.com, owner, repo]
				if len(parts) >= 3 && parts[0] == "github.com" {
					return parts[1], parts[2]
				}
			}
		}

		// 3. Fallback to module path
		parts := strings.Split(info.Main.Path, "/")
		if len(parts) >= 3 && parts[0] == "github.com" {
			return parts[1], parts[2]
		}
	}

	// Fallback for development
	return "geckyzz", "contourgo"
}

func GetVersionInfo() (string, string) {
	v := Version
	sha := CommitSHA

	if sha == "unknown" || sha == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					sha = setting.Value
					if len(sha) > 7 {
						sha = sha[:7]
					}
					break
				}
			}
		}
	}
	return v, sha
}

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
	stopCh          chan struct{}
}

func NewDiscordBot(
	cfg *config.Config,
	configPath string,
	database *db.DB,
	forceCheckChan chan bool,
) (*DiscordBot, error) {
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
		stopCh:          make(chan struct{}),
	}

	dg.AddHandler(bot.onReady)
	dg.AddHandler(bot.onInteractionCreate)

	return bot, nil
}

func (b *DiscordBot) Start() error {
	go b.queueWorker()
	return b.Session.Open()
}

func (b *DiscordBot) Stop() error {
	close(b.stopCh)
	// Clean up guild commands on shutdown
	if b.Session.State.User != nil {
		cmds, err := b.Session.ApplicationCommands(b.Session.State.User.ID, b.Config.Discord.Server)
		if err == nil {
			for _, cmd := range cmds {
				b.Session.ApplicationCommandDelete(
					b.Session.State.User.ID,
					b.Config.Discord.Server,
					cmd.ID,
				)
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
	inviteURL := fmt.Sprintf(
		"https://discord.com/oauth2/authorize?client_id=%s&permissions=8&integration_type=0&scope=bot+applications.commands",
		s.State.User.ID,
	)
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
		log.Printf(
			"⚠️ WARNING: The bot has not joined the target server (Guild ID: %s) yet. Please use the invite link above to add the bot with both 'bot' and 'applications.commands' scopes.",
			b.Config.Discord.Server,
		)
	}

	// Register Guild Slash Commands
	b.registerSlashCommands(s)
}
