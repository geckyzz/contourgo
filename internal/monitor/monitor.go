package monitor

import (
	"log"
	"sync"
	"time"

	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/db"
	"github.com/geckyzz/contourgo/internal/discord"
)

type Monitor struct {
	config         *config.Config
	db             *db.DB
	bot            *discord.DiscordBot
	forceCheckChan chan bool
	lastCheckTime  time.Time
	DumpComments   bool
	lastCheckMap   map[string]map[string]time.Time
	mu             sync.Mutex
}

func NewMonitor(
	cfg *config.Config,
	database *db.DB,
	bot *discord.DiscordBot,
	forceCheckChan chan bool,
) *Monitor {
	return &Monitor{
		config:         cfg,
		db:             database,
		bot:            bot,
		forceCheckChan: forceCheckChan,
		lastCheckMap:   make(map[string]map[string]time.Time),
	}
}

func (m *Monitor) isDue(service, key string, monitorCfg config.MonitorConfig, force bool) bool {
	if force {
		return true
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	lastCheck, ok := m.lastCheckMap[service][key]
	if !ok {
		return true
	}

	interval := config.ParseISO8601Duration(m.config.Config.Monitor.By)
	if monitorCfg.Monitor.By != "" {
		interval = config.ParseISO8601Duration(monitorCfg.Monitor.By)
	}

	return time.Since(lastCheck) >= interval
}

func alignToInterval(t time.Time, d time.Duration) time.Time {
	if d <= 0 {
		return t
	}
	totalPast := t.UnixNano() % int64(d)
	return t.Add(-time.Duration(totalPast))
}

func (m *Monitor) updateLastCheck(service, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.lastCheckMap == nil {
		m.lastCheckMap = make(map[string]map[string]time.Time)
	}
	if m.lastCheckMap[service] == nil {
		m.lastCheckMap[service] = make(map[string]time.Time)
	}

	now := time.Now()
	if m.config.Config.Time.Uniform {
		interval := config.ParseISO8601Duration(m.config.Config.Monitor.By)
		inner, ok := m.config.Monitors[service]
		if ok {
			if monitorCfg, exists := inner[key]; exists && monitorCfg.Monitor.By != "" {
				interval = config.ParseISO8601Duration(monitorCfg.Monitor.By)
			}
		}
		now = alignToInterval(now, interval)
	}
	m.lastCheckMap[service][key] = now
}

func (m *Monitor) hasActiveMonitorsDue(service string, force bool) bool {
	inner, ok := m.config.Monitors[service]
	if !ok || len(inner) == 0 {
		return false
	}
	for key, monitorCfg := range inner {
		if m.isDue(service, key, monitorCfg, force) {
			return true
		}
	}
	return false
}

func (m *Monitor) Start() {
	log.Println("Performing initial check on startup...")
	m.CheckAll(true)

	// Tick every 10 seconds to process due monitors
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.CheckAll(false)
		case <-m.forceCheckChan:
			log.Println("Manual check triggered.")
			m.CheckAll(true)
		}
	}
}

func (m *Monitor) CheckAll(force bool) {
	m.lastCheckTime = time.Now()

	var wg sync.WaitGroup
	anyChecked := false

	// 1. Nyaa
	if m.hasActiveMonitorsDue("nyaa", force) {
		anyChecked = true
		wg.Go(func() {
			m.checkNyaa(force)
		})
	}

	// 2. Sukebei
	if m.hasActiveMonitorsDue("sukebei", force) {
		anyChecked = true
		wg.Go(func() {
			m.checkSukebei(force)
		})
	}

	// 3. AnimeTosho Old
	if m.hasActiveMonitorsDue("animetosho_old", force) {
		anyChecked = true
		wg.Go(func() {
			m.checkAnimeToshoOld(force)
		})
	}

	// 4. AnimeTosho New
	if m.hasActiveMonitorsDue("animetosho_new", force) {
		anyChecked = true
		wg.Go(func() {
			m.checkAnimeToshoNew(force)
		})
	}

	// 5. NekoBT
	if m.hasActiveMonitorsDue("nekobt", force) {
		anyChecked = true
		wg.Go(func() {
			m.checkNekoBT(force)
		})
	}

	// 6. AniRena
	if m.hasActiveMonitorsDue("anirena", force) {
		anyChecked = true
		wg.Go(func() {
			m.checkAnirena(force)
		})
	}

	// 7. Tsukihime
	if m.hasActiveMonitorsDue("tsukihime", force) {
		anyChecked = true
		wg.Go(func() {
			m.checkTsukihime(force)
		})
	}

	wg.Wait()

	if anyChecked {
		log.Println("All monitor checks completed.")
	}
}
