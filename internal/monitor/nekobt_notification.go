package monitor

import (
	"fmt"
	"log"

	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/scraper"
)

func (m *Monitor) checkNekoBTNotification(
	service, key string,
	monitorCfg config.MonitorConfig,
	force bool,
) {
	prefix := fmt.Sprintf("[%s][%s]", service, key)
	log.Printf("%s Processing monitor", prefix)

	apiKey := m.Config().Config.Nekobt.API.Key
	scr := scraper.NewNekoBTScraper(apiKey)

	notifications, err := scr.FetchNotifications()
	if err != nil {
		log.Printf("%s Error fetching notifications: %v", prefix, err)
		return
	}

	for _, notif := range notifications {
		if notif.Seen {
			continue
		}

		if m.db.IsCommentStored(service, key, notif.ID) {
			continue
		}

		log.Printf("%s Found new notification: %s - %s", prefix, notif.ID, notif.Data)

		var ts int64 = scraper.DecodeNekoBTSnowflake(notif.ID) / 1000

		m.db.StoreComment(
			service,
			key,
			notif.ID,
			"NekoBT Notification",
			scraper.ParseNekoBTNotificationText(notif.Data),
			ts,
			0,
			"",
			"",
			"",
			"",
		)

		m.db.UpdateTorrent(service, key, "NekoBT Notification", 1, ts*1000, "")

		m.enqueueAnnouncement(prefix, service, key, notif.ID, monitorCfg)
	}
}
