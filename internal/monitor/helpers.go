package monitor

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/geckyzz/contourgo/internal/config"
)

func (m *Monitor) parseGenericDate(dateStr string) int64 {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		time.RFC3339,
	}
	for _, f := range formats {
		t, err := time.Parse(f, dateStr)
		if err == nil {
			return t.Unix()
		}
	}
	return 0
}

func (m *Monitor) isExcluded(title string, excludes []string) bool {
	if len(excludes) == 0 {
		return false
	}

	titleLower := strings.ToLower(title)
	for _, pattern := range excludes {
		patternLower := strings.ToLower(pattern)
		matched, err := filepath.Match(patternLower, titleLower)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func (m *Monitor) enqueueAnnouncement(
	prefix, service, torrentID, commentID string,
	monitorCfg config.MonitorConfig,
) {
	if !m.DumpComments {
		log.Printf("%s Queuing announcement for comment %s on %s", prefix, commentID, torrentID)
		cfg := m.Config()
		err := m.bot.EnqueueAnnouncement(
			service,
			string(monitorCfg.Discord.Channel),
			torrentID,
			commentID,
			cfg.ResolveAuthorURL(monitorCfg),
			cfg.ResolveCommentID(monitorCfg),
			cfg.ResolveUserContentImage(monitorCfg),
			cfg.ResolveMentionsDisable(monitorCfg),
		)
		if err != nil {
			log.Printf("%s Error queuing announcement: %v", prefix, err)
		}
	} else {
		log.Printf("%s [DUMP] Storing comment %s on %s", prefix, commentID, torrentID)
	}
}

func (m *Monitor) logFetch(prefix string, page int, params map[string]any) {
	var parts []string

	// Define a preferred order for common keys
	order := []string{
		"keyword",
		"q",
		"user",
		"group",
		"group_id",
		"uploader_id",
		"media_id",
		"query",
		"sort",
		"order",
		"offset",
	}

	// First, add keys in preferred order
	seen := make(map[string]bool)
	for _, k := range order {
		if v, ok := params[k]; ok {
			seen[k] = true
			val := fmt.Sprintf("%v", v)
			if val == "" || val == "0" || val == "<nil>" {
				continue
			}
			if s, ok := v.(string); ok {
				parts = append(parts, fmt.Sprintf("%s: %q", k, s))
			} else {
				parts = append(parts, fmt.Sprintf("%s: %v", k, v))
			}
		}
	}

	// Add any remaining keys
	for k, v := range params {
		if !seen[k] {
			val := fmt.Sprintf("%v", v)
			if val == "" || val == "0" || val == "<nil>" {
				continue
			}
			if s, ok := v.(string); ok {
				parts = append(parts, fmt.Sprintf("%s: %q", k, s))
			} else {
				parts = append(parts, fmt.Sprintf("%s: %v", k, v))
			}
		}
	}

	paramStr := ""
	if len(parts) > 0 {
		paramStr = " (" + strings.Join(parts, ", ") + ")"
	}
	log.Printf("%s Fetching page %d%s", prefix, page, paramStr)
}
