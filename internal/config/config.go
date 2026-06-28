package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type StringOrInt string

func (s *StringOrInt) UnmarshalText(text []byte) error {
	*s = StringOrInt(text)
	return nil
}

type Config struct {
	Discord  DiscordConfig                       `toml:"discord"`
	Config   MainConfig                          `toml:"config"`
	Monitors map[string]map[string]MonitorConfig `toml:"monitors"`
	Donation DonationConfig                      `toml:"donation"`
}

type DiscordConfig struct {
	Token           string               `toml:"token"`
	Server          StringOrInt          `toml:"server"`
	Mentions        map[string]any       `toml:"mentions"`
	Announce        AnnounceConfig       `toml:"announce"`
	Members         MembersConfig        `toml:"members"`
	Embed           MonitorEmbedConfig   `toml:"embed"`
	Fields          MonitorFieldsConfig  `toml:"fields"`
	Display         MonitorDisplayConfig `toml:"display"`
	MentionsDisable *bool                `toml:"mentions_disable"`
}

type DonationConfig struct {
	Currency       string             `toml:"currency"`         // e.g. USD, EUR, etc.
	PerkMultiplier float64            `toml:"perk_multiplier"`  // e.g. 9.99 USD
	MaxStacks      int                `toml:"max_stacks"`       // e.g. 12 months maximum stack limit
	Tiers          map[string]float64 `toml:"tiers"`            // Tier RoleID -> Minimum Cumulative USD required
	NotifyWarnDays int                `toml:"notify_warn_days"` // Days before warning threshold (e.g. 3)
	Silent         SilentConfig       `toml:"silent"`           // Nested dot-notation configuration for silences
	Format         DonationFormat     `toml:"format"`           // Configurable DM templates
}

type SilentConfig struct {
	Globally  bool `toml:"globally"`   // Mute all DMs
	OnWarning bool `toml:"on_warning"` // Mute only warning pings
	OnExpiry  bool `toml:"on_expiry"`  // Mute only expiry DMs
}

type DonationFormat struct {
	Add    ActionTemplates `toml:"add"`
	Renew  ActionTemplates `toml:"renew"`
	Warn   ActionTemplates `toml:"warn"`
	Expiry ActionTemplates `toml:"expiry"`
}

type ActionTemplates struct {
	Title string `toml:"title"`
	Desc  string `toml:"desc"`
}

type AnnounceConfig struct {
	Channel StringOrInt `toml:"channel"`
}

type MembersConfig struct {
	Admins     AllowConfig     `toml:"admins"`
	Moderators AllowConfig     `toml:"moderators"`
	Others     AllowListConfig `toml:"others"`
	Roles      AllowListConfig `toml:"roles"`
}

type AllowConfig struct {
	Allow bool `toml:"allow"`
}

type AllowListConfig struct {
	Allow []string `toml:"allow"`
}

func (a *AllowListConfig) UnmarshalTOML(fn func(any) error) error {
	var raw struct {
		Allow []any `toml:"allow"`
	}
	if err := fn(&raw); err != nil {
		return err
	}
	a.Allow = make([]string, 0, len(raw.Allow))
	for _, item := range raw.Allow {
		switch v := item.(type) {
		case string:
			a.Allow = append(a.Allow, v)
		case int64:
			a.Allow = append(a.Allow, fmt.Sprintf("%d", v))
		case float64:
			a.Allow = append(a.Allow, fmt.Sprintf("%.0f", v))
		default:
			a.Allow = append(a.Allow, fmt.Sprintf("%v", v))
		}
	}
	return nil
}

type MainConfig struct {
	Time       TimeConfig        `toml:"time"`
	Monitor    MonitorTimeConfig `toml:"monitor"`
	Nyaa       NyaaConfig        `toml:"nyaa"`
	Animetosho AnimetoshoConfig  `toml:"animetosho"`
	Nekobt     NekobtConfig      `toml:"nekobt"`
	Anirena    AnirenaConfig     `toml:"anirena"`
	Twitter    TwitterConfig     `toml:"twitter"`
}

type TimeConfig struct {
	Uniform bool `toml:"uniform"`
}

type MonitorTimeConfig struct {
	By string `toml:"by"`
}

type NyaaConfig struct {
	Proxy ProxyConfig `toml:"proxy"`
	Sort  string      `toml:"sort"`
	Order string      `toml:"order"`
	Page  PageConfig  `toml:"page"`
}

type ProxyConfig struct {
	URL string `toml:"url"`
}

type AnimetoshoConfig struct {
	Old AnimetoshoPageConfig `toml:"old"`
	New AnimetoshoPageConfig `toml:"new"`
}

type AnimetoshoPageConfig struct {
	Page PageConfig `toml:"page"`
}

type NekobtConfig struct {
	API  NekobtAPIConfig `toml:"api"`
	Sort string          `toml:"sort"`
	Page PageConfig      `toml:"page"`
}

type NekobtAPIConfig struct {
	Key string `toml:"key"`
}

type AnirenaConfig struct {
	API   AnirenaAPIConfig `toml:"api"`
	Sort  string           `toml:"sort"`
	Order string           `toml:"order"`
	Page  PageConfig       `toml:"page"`
}

type AnirenaAPIConfig struct {
	Key string `toml:"key"`
}

// TwitterConfig holds global settings for Twitter/Nitter monitoring.
type TwitterConfig struct {
	// NitterURL is the base URL of the Nitter instance to use (e.g. "https://nitter.net").
	// Individual monitors may override this.
	NitterURL string `toml:"nitter_url"`
	// EmbedService is the global default fix-embed domain/service to use.
	EmbedService string `toml:"embed_service"`
	// ExcludeReposts is the global default for ignoring retweet/repost items.
	ExcludeReposts bool `toml:"exclude_reposts"`
}

type MonitorConfig struct {
	Keywords  []string             `toml:"keywords"`
	Excludes  []string             `toml:"excludes"`
	Uploaders []string             `toml:"uploaders"` // Unified for Nyaa/Sukebei/NekoBT
	Groups    []string             `toml:"groups"`    // NekoBT
	Media     []string             `toml:"media"`     // NekoBT
	Sort      string               `toml:"sort"`      // Optional: sorting method
	Order     string               `toml:"order"`     // Optional: asc/desc
	Page      PageConfig           `toml:"page"`
	Discord   MonitorDiscordConfig `toml:"discord"`
	Monitor   MonitorTimeConfig    `toml:"monitor"`

	// Twitter/Nitter RSS-specific fields (only used by [monitor.twitter.*]).
	// Account is the Twitter/X username (without @). Defaults to the monitor key.
	Account string `toml:"account"`
	// NitterURL overrides config.twitter.nitter_url for this specific account.
	NitterURL string `toml:"nitter_url"`
	// EmbedService rewrites tweet links to a fix-embed front-end.
	// Short names: "fixupx", "vxtwitter", "fxtwitter", "twittpr".
	// Or pass any bare domain string (e.g. "fixvx.com").
	EmbedService string `toml:"embed_service"`
	// CustomFormat is a Go text/template string sent as plain content instead of an embed.
	// Placeholders: {{.Account}}, {{.DisplayName}}, {{.TweetID}}, {{.Link}},
	//               {{.OriginalLink}}, {{.Title}}, {{.PublishedAt}}
	CustomFormat string `toml:"custom_format"`
	// ExcludeReposts ignores Nitter RSS items that are retweets/reposts (starting with "RT by @").
	ExcludeReposts *bool `toml:"exclude_reposts"`
}

type PageConfig struct {
	Max int `toml:"max"`
}

type MonitorDiscordConfig struct {
	Channel  StringOrInt           `toml:"channel"` // Optional: override global announce channel
	Mentions MonitorMentionsConfig `toml:"mentions"`
	Embed    MonitorEmbedConfig    `toml:"embed"`
	Fields   MonitorFieldsConfig   `toml:"fields"`
	Display  MonitorDisplayConfig  `toml:"display"`
}

type MonitorMentionsConfig struct {
	Disable bool `toml:"disable"`
}

type MonitorDisplayConfig struct {
	UserContentImage *bool `toml:"user_content_image"`
}

type MonitorFieldsConfig struct {
	CommentID *bool `toml:"comment_id"`
}

type MonitorEmbedConfig struct {
	Author    MonitorAuthorConfig `toml:"author"`
	Thumbnail *string             `toml:"thumbnail"` // Deprecated: use Author.URL
}

type MonitorAuthorConfig struct {
	URL *string `toml:"url"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	// Try parsing everything normally first
	err = toml.Unmarshal(data, &cfg)

	// Migrate Thumbnail to Author.URL if needed
	for svc := range cfg.Monitors {
		for key := range cfg.Monitors[svc] {
			m := cfg.Monitors[svc][key]
			if (m.Discord.Embed.Author.URL == nil || *m.Discord.Embed.Author.URL == "") &&
				m.Discord.Embed.Thumbnail != nil &&
				*m.Discord.Embed.Thumbnail != "" {
				m.Discord.Embed.Author.URL = m.Discord.Embed.Thumbnail
				cfg.Monitors[svc][key] = m
			}
		}
	}

	// Fallback to raw map for robust recovery if monitors are empty or error occurred
	if err != nil || len(cfg.Monitors) == 0 {
		var raw map[string]any
		if err2 := toml.Unmarshal(data, &raw); err2 == nil {
			// 1. Recover Discord if needed
			if cfg.Discord.Token == "" {
				if d, ok := raw["discord"]; ok {
					dBytes, _ := toml.Marshal(d)
					toml.Unmarshal(dBytes, &cfg.Discord)
				}
			}

			// 2. Recover Monitors (check both "monitor" and "monitors")
			if len(cfg.Monitors) == 0 {
				mSource := raw["monitors"]
				if mSource == nil {
					mSource = raw["monitor"]
				}
				if mSource != nil {
					if mData, ok := mSource.(map[string]any); ok {
						// Handle Underscore Aliases (backward compatibility)
						if org, ok := mData["animetosho_org"]; ok &&
							mData["animetosho_old"] == nil {
							mData["animetosho_old"] = org
						}
						if xyz, ok := mData["animetosho_xyz"]; ok &&
							mData["animetosho_new"] == nil {
							mData["animetosho_new"] = xyz
						}

						mBytes, _ := toml.Marshal(mData)
						toml.Unmarshal(mBytes, &cfg.Monitors)
					}
				}
			}

			// 3. Recover Config section if the initial parse failed
			if err != nil && raw["config"] != nil {
				if configData, ok := raw["config"].(map[string]any); ok {
					// monitor.by
					if mon, ok := configData["monitor"].(map[string]any); ok {
						if by, ok := mon["by"].(string); ok {
							cfg.Config.Monitor.By = by
						}
					}
					// time.uniform
					if t, ok := configData["time"].(map[string]any); ok {
						if u, ok := t["uniform"].(bool); ok {
							cfg.Config.Time.Uniform = u
						}
					}
					// nyaa.proxy.url
					if nyaa, ok := configData["nyaa"].(map[string]any); ok {
						if proxy, ok := nyaa["proxy"].(map[string]any); ok {
							if u, ok := proxy["url"].(string); ok {
								cfg.Config.Nyaa.Proxy.URL = u
							}
						}
					}
					// animetosho
					if at, ok := configData["animetosho"].(map[string]any); ok {
						// Alias .org -> .old, .xyz -> .new
						if org, ok := at["org"].(map[string]any); ok && at["old"] == nil {
							at["old"] = org
						}
						if xyz, ok := at["xyz"].(map[string]any); ok && at["new"] == nil {
							at["new"] = xyz
						}

						// Handle page.max legacy
						legacyMax := 5
						if p, ok := at["page"].(map[string]any); ok {
							if m, ok := p["max"].(int64); ok {
								legacyMax = int(m)
							}
						}

						// Handle Old
						if oldVal, ok := at["old"].(map[string]any); ok {
							if p, ok := oldVal["page"].(map[string]any); ok {
								if m, ok := p["max"].(int64); ok {
									cfg.Config.Animetosho.Old.Page.Max = int(m)
								}
							}
						} else {
							cfg.Config.Animetosho.Old.Page.Max = legacyMax
						}

						// Handle New
						if newVal, ok := at["new"].(map[string]any); ok {
							if p, ok := newVal["page"].(map[string]any); ok {
								if m, ok := p["max"].(int64); ok {
									cfg.Config.Animetosho.New.Page.Max = int(m)
								}
							}
						} else {
							// If "new" was a bool (legacy), we just use legacyMax
							cfg.Config.Animetosho.New.Page.Max = legacyMax
						}
					}
				}
			}
		}
	}

	// Clean fields
	cfg.Config.Nyaa.Proxy.URL = strings.TrimSuffix(cfg.Config.Nyaa.Proxy.URL, "/")
	cfg.Config.Twitter.NitterURL = strings.TrimSuffix(cfg.Config.Twitter.NitterURL, "/")

	// Trim trailing slashes from per-monitor NitterURL fields.
	if twitterMonitors, ok := cfg.Monitors["twitter"]; ok {
		for key, m := range twitterMonitors {
			if m.NitterURL != "" {
				m.NitterURL = strings.TrimSuffix(m.NitterURL, "/")
				twitterMonitors[key] = m
			}
		}
	}

	return &cfg, nil
}

func ParseISO8601Duration(s string) time.Duration {
	if s == "" {
		return 30 * time.Minute
	}

	if d, err := time.ParseDuration(s); err == nil {
		return d
	}

	s = strings.ToUpper(s)
	if !strings.HasPrefix(s, "P") {
		return 30 * time.Minute
	}

	var duration time.Duration
	_, after, ok := strings.Cut(s, "T")
	if ok {
		timePart := after
		var currentNum string
		for i := 0; i < len(timePart); i++ {
			char := timePart[i]
			if char >= '0' && char <= '9' {
				currentNum += string(char)
			} else {
				if currentNum != "" {
					var num int
					fmt.Sscanf(currentNum, "%d", &num)
					currentNum = ""
					switch char {
					case 'H':
						duration += time.Duration(num) * time.Hour
					case 'M':
						duration += time.Duration(num) * time.Minute
					case 'S':
						duration += time.Duration(num) * time.Second
					}
				}
			}
		}
	}

	if duration == 0 {
		return 30 * time.Minute
	}
	return duration
}

func (cfg *Config) LogConfigSummary() {
	log.Println("--- Configuration Summary ---")
	log.Printf("Discord Server ID: %s", cfg.Discord.Server)
	log.Printf("Discord Announce Channel: %s", cfg.Discord.Announce.Channel)
	log.Printf("Discord Comment ID Visible: %t", func() bool {
		if cfg.Discord.Fields.CommentID != nil {
			return *cfg.Discord.Fields.CommentID
		}
		return false
	}())
	log.Printf("Discord User Content Image: %t", func() bool {
		if cfg.Discord.Display.UserContentImage != nil {
			return *cfg.Discord.Display.UserContentImage
		}
		return false
	}())
	log.Printf(
		"Donation Settings: Perk Multiplier=%.2f, Max Stacks=%d, Warn Days=%d, Silent Globally=%t",
		cfg.Donation.PerkMultiplier,
		cfg.Donation.MaxStacks,
		cfg.Donation.NotifyWarnDays,
		cfg.Donation.Silent.Globally,
	)
	log.Printf(
		"Monitor Interval: %s (%v)",
		cfg.Config.Monitor.By,
		ParseISO8601Duration(cfg.Config.Monitor.By),
	)
	log.Printf("Nyaa Proxy URL: %q", cfg.Config.Nyaa.Proxy.URL)
	log.Printf("nekoBT API Key: %s", func() string {
		if cfg.Config.Nekobt.API.Key != "" {
			return "PRESENT"
		}
		return "MISSING"
	}())
	log.Printf("AniRena API Key: %s", func() string {
		if cfg.Config.Anirena.API.Key != "" {
			return "PRESENT"
		}
		return "MISSING"
	}())
	log.Printf(
		"AnimeTosho Max Pages: Old=%d, New=%d",
		cfg.Config.Animetosho.Old.Page.Max,
		cfg.Config.Animetosho.New.Page.Max,
	)

	totalMonitors := 0
	for service, innerMap := range cfg.Monitors {
		log.Printf("Service %q has %d monitor(s):", service, len(innerMap))
		for key, m := range innerMap {
			totalMonitors++
			log.Printf("  - [%s]:", key)

			if len(m.Keywords) > 0 {
				log.Printf("      keywords: %v", m.Keywords)
			}
			if len(m.Uploaders) > 0 {
				log.Printf("      uploaders: %v", m.Uploaders)
			}
			if len(m.Groups) > 0 {
				log.Printf("      groups: %v", m.Groups)
			}
			if len(m.Media) > 0 {
				log.Printf("      media: %v", m.Media)
			}
			if m.Sort != "" {
				log.Printf("      sort: %q", m.Sort)
			}
			if m.Order != "" {
				log.Printf("      order: %q", m.Order)
			}
			if m.Page.Max > 0 {
				log.Printf("      page_max: %d", m.Page.Max)
			}

			// If no specific filters were logged
			if len(m.Keywords) == 0 && len(m.Uploaders) == 0 && len(m.Groups) == 0 &&
				len(m.Media) == 0 && m.Sort == "" && m.Order == "" && m.Page.Max == 0 {
				log.Printf("      (global/default filters)")
			}
		}
	}

	log.Printf("Total Monitors Configured: %d", totalMonitors)
	log.Println("-----------------------------")
}

func (c *Config) ResolveAuthorURL(monitor MonitorConfig) string {
	if monitor.Discord.Embed.Author.URL != nil && *monitor.Discord.Embed.Author.URL != "" {
		return *monitor.Discord.Embed.Author.URL
	}
	if c.Discord.Embed.Author.URL != nil && *c.Discord.Embed.Author.URL != "" {
		return *c.Discord.Embed.Author.URL
	}
	return ""
}

func (c *Config) ResolveCommentID(monitor MonitorConfig) bool {
	if monitor.Discord.Fields.CommentID != nil {
		return *monitor.Discord.Fields.CommentID
	}
	if c.Discord.Fields.CommentID != nil {
		return *c.Discord.Fields.CommentID
	}
	return false
}

func (c *Config) ResolveUserContentImage(monitor MonitorConfig) bool {
	if monitor.Discord.Display.UserContentImage != nil {
		return *monitor.Discord.Display.UserContentImage
	}
	if c.Discord.Display.UserContentImage != nil {
		return *c.Discord.Display.UserContentImage
	}
	return false
}

// ResolveNitterURL returns the effective Nitter base URL for a MonitorConfig entry.
// Per-monitor nitter_url takes precedence over the global config.twitter.nitter_url.
// Falls back to "https://nitter.net" if neither is set.
func (c *Config) ResolveNitterURL(monitor MonitorConfig) string {
	if monitor.NitterURL != "" {
		return monitor.NitterURL
	}
	if c.Config.Twitter.NitterURL != "" {
		return c.Config.Twitter.NitterURL
	}
	return "https://nitter.net"
}

// ResolveEmbedService returns the effective EmbedService for a MonitorConfig entry.
// Per-monitor embed_service takes precedence over the global config.twitter.embed_service.
func (c *Config) ResolveEmbedService(monitor MonitorConfig) string {
	if monitor.EmbedService != "" {
		return monitor.EmbedService
	}
	return c.Config.Twitter.EmbedService
}

// ResolveExcludeReposts returns the effective ExcludeReposts setting for a MonitorConfig entry.
// Per-monitor exclude_reposts takes precedence over the global config.twitter.exclude_reposts.
func (c *Config) ResolveExcludeReposts(monitor MonitorConfig) bool {
	if monitor.ExcludeReposts != nil {
		return *monitor.ExcludeReposts
	}
	return c.Config.Twitter.ExcludeReposts
}

// ResolveMentionsDisable returns true if mentions are disabled globally or on the monitor level.
func (c *Config) ResolveMentionsDisable(monitor MonitorConfig) bool {
	if monitor.Discord.Mentions.Disable {
		return true
	}
	if c.Discord.MentionsDisable != nil {
		return *c.Discord.MentionsDisable
	}
	return false
}

// ResolveNyaaSort returns the sort method for a Nyaa/Sukebei monitor, falling back to config-level, then "comments".
func (c *Config) ResolveNyaaSort(monitor MonitorConfig) string {
	if monitor.Sort != "" {
		return monitor.Sort
	}
	if c.Config.Nyaa.Sort != "" {
		return c.Config.Nyaa.Sort
	}
	return "comments"
}

// ResolveNyaaOrder returns the order for a Nyaa/Sukebei monitor, falling back to config-level, then "desc".
func (c *Config) ResolveNyaaOrder(monitor MonitorConfig) string {
	if monitor.Order != "" {
		return monitor.Order
	}
	if c.Config.Nyaa.Order != "" {
		return c.Config.Nyaa.Order
	}
	return "desc"
}

// ResolveNyaaPageMax returns the page limit for a Nyaa/Sukebei monitor, falling back to config-level, then 0.
func (c *Config) ResolveNyaaPageMax(monitor MonitorConfig) int {
	if monitor.Page.Max > 0 {
		return monitor.Page.Max
	}
	return c.Config.Nyaa.Page.Max
}

// ResolveNekobtSort returns the sort method for a Nekobt monitor, falling back to config-level, then "date".
func (c *Config) ResolveNekobtSort(monitor MonitorConfig) string {
	if monitor.Sort != "" {
		return monitor.Sort
	}
	if c.Config.Nekobt.Sort != "" {
		return c.Config.Nekobt.Sort
	}
	return "date"
}

// ResolveNekobtPageMax returns the page limit for a Nekobt monitor, falling back to config-level, then 0.
func (c *Config) ResolveNekobtPageMax(monitor MonitorConfig) int {
	if monitor.Page.Max > 0 {
		return monitor.Page.Max
	}
	return c.Config.Nekobt.Page.Max
}

// ResolveAnirenaSort returns the sort method for an Anirena monitor, falling back to config-level, then "date".
func (c *Config) ResolveAnirenaSort(monitor MonitorConfig) string {
	if monitor.Sort != "" {
		return monitor.Sort
	}
	if c.Config.Anirena.Sort != "" {
		return c.Config.Anirena.Sort
	}
	return "date"
}

// ResolveAnirenaOrder returns the order for an Anirena monitor, falling back to config-level, then "desc".
func (c *Config) ResolveAnirenaOrder(monitor MonitorConfig) string {
	if monitor.Order != "" {
		return monitor.Order
	}
	if c.Config.Anirena.Order != "" {
		return c.Config.Anirena.Order
	}
	return "desc"
}

// ResolveAnirenaPageMax returns the page limit for an Anirena monitor, falling back to config-level, then 0.
func (c *Config) ResolveAnirenaPageMax(monitor MonitorConfig) int {
	if monitor.Page.Max > 0 {
		return monitor.Page.Max
	}
	return c.Config.Anirena.Page.Max
}
