package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Discord  DiscordConfig                       `toml:"discord"`
	Config   MainConfig                          `toml:"config"`
	Monitors map[string]map[string]MonitorConfig `toml:"monitors"`
}

type DiscordConfig struct {
	Token    string         `toml:"token"`
	Server   string         `toml:"server"`
	Announce AnnounceConfig `toml:"announce"`
	Members  MembersConfig  `toml:"members"`
}

type AnnounceConfig struct {
	Channel string `toml:"channel"`
}

type MembersConfig struct {
	Admins     AllowConfig     `toml:"admins"`
	Moderators AllowConfig     `toml:"moderators"`
	Others     AllowListConfig `toml:"others"`
}

type AllowConfig struct {
	Allow bool `toml:"allow"`
}

type AllowListConfig struct {
	Allow []string `toml:"allow"`
}

type MainConfig struct {
	Monitor    MonitorTimeConfig `toml:"monitor"`
	Nyaa       NyaaConfig        `toml:"nyaa"`
	Animetosho AnimetoshoConfig  `toml:"animetosho"`
	Nekobt     NekobtConfig      `toml:"nekobt"`
	Anirena    AnirenaConfig     `toml:"anirena"`
}

type MonitorTimeConfig struct {
	By string `toml:"by"`
}

type NyaaConfig struct {
	Proxy ProxyConfig `toml:"proxy"`
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
	API NekobtAPIConfig `toml:"api"`
}

type NekobtAPIConfig struct {
	Key string `toml:"key"`
}

type AnirenaConfig struct {
	API      AnirenaAPIConfig `toml:"api"`
	Username string           `toml:"username"`
	Password string           `toml:"password"`
}

type AnirenaAPIConfig struct {
	Key string `toml:"key"`
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
}

type PageConfig struct {
	Max int `toml:"max"`
}

type MonitorDiscordConfig struct {
	Embed MonitorEmbedConfig `toml:"embed"`
}

type MonitorEmbedConfig struct {
	Thumbnail string `toml:"thumbnail"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	// Try parsing everything normally first
	err = toml.Unmarshal(data, &cfg)

	// Fallback to raw map for robust recovery if monitors are empty or error occurred
	if err != nil || len(cfg.Monitors) == 0 {
		var raw map[string]interface{}
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
					if mData, ok := mSource.(map[string]interface{}); ok {
						// Handle Underscore Aliases (backward compatibility)
						if org, ok := mData["animetosho_org"]; ok && mData["animetosho_old"] == nil {
							mData["animetosho_old"] = org
						}
						if xyz, ok := mData["animetosho_xyz"]; ok && mData["animetosho_new"] == nil {
							mData["animetosho_new"] = xyz
						}

						// Handle Dotted/Nested Aliases [monitor.animetosho.old], etc.
						if at, ok := mData["animetosho"].(map[string]interface{}); ok {
							if org, ok := at["org"].(map[string]interface{}); ok && mData["animetosho_old"] == nil {
								mData["animetosho_old"] = org
							}
							if old, ok := at["old"].(map[string]interface{}); ok && mData["animetosho_old"] == nil {
								mData["animetosho_old"] = old
							}

							if xyz, ok := at["xyz"].(map[string]interface{}); ok && mData["animetosho_new"] == nil {
								mData["animetosho_new"] = xyz
							}
							if new, ok := at["new"].(map[string]interface{}); ok && mData["animetosho_new"] == nil {
								mData["animetosho_new"] = new
							}
						}

						mBytes, _ := toml.Marshal(mData)
						toml.Unmarshal(mBytes, &cfg.Monitors)
					}
				}
			}

			// 3. Recover Config section if the initial parse failed
			if err != nil && raw["config"] != nil {
				if configData, ok := raw["config"].(map[string]interface{}); ok {
					// monitor.by
					if mon, ok := configData["monitor"].(map[string]interface{}); ok {
						if by, ok := mon["by"].(string); ok {
							cfg.Config.Monitor.By = by
						}
					}
					// nyaa.proxy.url
					if nyaa, ok := configData["nyaa"].(map[string]interface{}); ok {
						if proxy, ok := nyaa["proxy"].(map[string]interface{}); ok {
							if u, ok := proxy["url"].(string); ok {
								cfg.Config.Nyaa.Proxy.URL = u
							}
						}
					}
					// animetosho
					if at, ok := configData["animetosho"].(map[string]interface{}); ok {
						// Alias .org -> .old, .xyz -> .new
						if org, ok := at["org"].(map[string]interface{}); ok && at["old"] == nil {
							at["old"] = org
						}
						if xyz, ok := at["xyz"].(map[string]interface{}); ok && at["new"] == nil {
							at["new"] = xyz
						}

						// Handle page.max legacy
						legacyMax := 5
						if p, ok := at["page"].(map[string]interface{}); ok {
							if m, ok := p["max"].(int64); ok {
								legacyMax = int(m)
							}
						}

						// Handle Old
						if oldVal, ok := at["old"].(map[string]interface{}); ok {
							if p, ok := oldVal["page"].(map[string]interface{}); ok {
								if m, ok := p["max"].(int64); ok {
									cfg.Config.Animetosho.Old.Page.Max = int(m)
								}
							}
						} else {
							cfg.Config.Animetosho.Old.Page.Max = legacyMax
						}

						// Handle New
						if newVal, ok := at["new"].(map[string]interface{}); ok {
							if p, ok := newVal["page"].(map[string]interface{}); ok {
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
	tIdx := strings.Index(s, "T")
	if tIdx != -1 {
		timePart := s[tIdx+1:]
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
	log.Printf("Monitor Interval: %s (%v)", cfg.Config.Monitor.By, ParseISO8601Duration(cfg.Config.Monitor.By))
	log.Printf("Nyaa Proxy URL: %q", cfg.Config.Nyaa.Proxy.URL)
	log.Printf("NekoBT API Key: %s", func() string {
		if cfg.Config.Nekobt.API.Key != "" {
			return "PRESENT"
		}
		return "MISSING"
	}())
	log.Printf("AnimeTosho Max Pages: Old=%d, New=%d", cfg.Config.Animetosho.Old.Page.Max, cfg.Config.Animetosho.New.Page.Max)

	totalMonitors := 0
	for service, innerMap := range cfg.Monitors {
		log.Printf("Service %q has %d monitor(s):", service, len(innerMap))
		for key, m := range innerMap {
			totalMonitors++
			details := fmt.Sprintf("keywords=%v, uploaders=%v, sort=%q, order=%q, page_max=%d",
				m.Keywords, m.Uploaders, m.Sort, m.Order, m.Page.Max)
			if service == "nekobt" {
				details = fmt.Sprintf("groups=%v, uploaders=%v, media=%v, keywords=%v, sort=%q, page_max=%d",
					m.Groups, m.Uploaders, m.Media, m.Keywords, m.Sort, m.Page.Max)
			}
			log.Printf("  - [%s]: %s", key, details)
		}
	}
	log.Printf("Total Monitors Configured: %d", totalMonitors)
	log.Println("-----------------------------")
}
