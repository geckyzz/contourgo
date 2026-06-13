package discord

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/geckyzz/contourgo/internal/config"
)

func (b *DiscordBot) handleSlashReload(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	target := "monitors"
	if opt, ok := optionMap["target"]; ok {
		target = opt.StringValue()
	}

	if target == "config" {
		newCfg, err := config.LoadConfig(b.ConfigPath)
		if err != nil {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("❌ Error loading config: %v", err),
				},
			})
			return
		}
		b.Config = newCfg
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "✅ Main config reloaded (monitors not affected).",
			},
		})
	} else {
		b.ForceCheckChan <- true
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "🔄 Monitor check triggered manually.",
			},
		})
	}
}

func (b *DiscordBot) handleSlashPing(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🏓 Pong! Latency: %v", s.HeartbeatLatency()),
		},
	})
}

func (b *DiscordBot) handleSlashLogs(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	optionMap map[string]*discordgo.ApplicationCommandInteractionDataOption,
) {
	lines := 20
	if opt, ok := optionMap["lines"]; ok {
		lines = int(opt.IntValue())
	}

	file, err := os.Open("bot.log")
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Error opening log file: %v", err),
			},
		})
		return
	}
	defer file.Close()

	var logLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		logLines = append(logLines, scanner.Text())
		if len(logLines) > lines {
			logLines = logLines[1:]
		}
	}

	content := strings.Join(logLines, "\n")
	if len(content) > 1900 {
		content = content[len(content)-1900:]
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf(
				"📋 **Latest %d Log Lines:**\n```\n%s\n```",
				len(logLines),
				content,
			),
		},
	})
}

func (b *DiscordBot) handleSlashHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var sb strings.Builder
	for _, cmd := range b.Commands {
		if cmd.ID != "" {
			sb.WriteString(fmt.Sprintf("- </%s:%s> - %s\n", cmd.Name, cmd.ID, cmd.Description))
		} else {
			sb.WriteString(fmt.Sprintf("- `/%s` - %s\n", cmd.Name, cmd.Description))
		}
	}

	helpMsg := sb.String()
	if helpMsg == "" {
		helpMsg = "No commands available."
	}

	log.Printf("[POST] Responding to /help command with adaptive embed help menu")
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       "📜 Available Slash Commands",
					Description: helpMsg,
					Color:       0xf1c40f,
				},
			},
		},
	})
}

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (b *DiscordBot) handleSlashUpdate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	owner, repo := GetRepoInfo()
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Error creating update request: %v", err),
		)
		return
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Error checking for updates: %v", err),
		)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Failed to fetch release info (HTTP %d)", resp.StatusCode),
		)
		return
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Error decoding release info: %v", err),
		)
		return
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion, _ := GetVersionInfo()

	if latestVersion == currentVersion {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("✅ Bot is already up to date (v%s).", currentVersion),
		)
		return
	}

	// Find the correct asset
	assetName := fmt.Sprintf("contourgo-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf(
				"❌ Could not find a matching binary for your platform (%s) in release %s",
				assetName,
				release.TagName,
			),
		)
		return
	}

	b.sendFollowupMessage(
		s,
		i.Interaction,
		fmt.Sprintf("🚀 Update found: v%s. Downloading and applying update...", latestVersion),
	)

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Error getting executable path: %v", err),
		)
		return
	}

	// Download new binary
	resp, err = http.Get(downloadURL)
	if err != nil {
		b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error downloading update: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Failed to download binary (HTTP %d)", resp.StatusCode),
		)
		return
	}

	// Create a temporary file
	tmpPath := execPath + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		b.sendFollowupMessage(
			s,
			i.Interaction,
			fmt.Sprintf("❌ Error creating temporary file: %v", err),
		)
		return
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		b.sendFollowupMessage(s, i.Interaction, fmt.Sprintf("❌ Error saving new binary: %v", err))
		return
	}
	tmpFile.Close()

	if err := os.Rename(tmpPath, execPath); err != nil {
		_ = os.Remove(execPath)
		if err := os.Rename(tmpPath, execPath); err != nil {
			os.Remove(tmpPath)
			b.sendFollowupMessage(
				s,
				i.Interaction,
				fmt.Sprintf("❌ Error replacing binary: %v. Please update manually.", err),
			)
			return
		}
	}

	b.sendFollowupMessage(s, i.Interaction, "✅ Update applied successfully. Restarting bot...")

	log.Printf("Update applied. Exiting with code 301 for restart.")
	os.Exit(301)
}
