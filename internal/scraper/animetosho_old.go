package scraper

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type AnimeToshoOldScraper struct {
	baseURL string
	client  *http.Client
}

func NewAnimeToshoOldScraper() *AnimeToshoOldScraper {
	return &AnimeToshoOldScraper{
		baseURL: "https://animetosho.org",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *AnimeToshoOldScraper) ScrapeComments(page int) ([]ATComment, bool, error) {
	qVals := url.Values{}
	if page > 1 {
		qVals.Set("page", strconv.Itoa(page))
	}
	qVals.Add("filter_types[]", "0")

	u := fmt.Sprintf("%s/comments?%s", s.baseURL, qVals.Encode())
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("HTTP error %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, false, err
	}

	var comments []ATComment

	// Parse Old AnimeTosho Layout (ORG)
	doc.Find("div.comment, div.comment2").Each(func(i int, sel *goquery.Selection) {
		commentUser := sel.Find("div.comment_user")
		if commentUser.Length() == 0 {
			return
		}

		var torrentID string
		var torrentTitle string
		var commentID string

		commentUser.Find("a").Each(func(j int, a *goquery.Selection) {
			href, exists := a.Attr("href")
			if !exists {
				return
			}
			if strings.Contains(href, "/view/") {
				if strings.Contains(href, "#comment") {
					parts := strings.Split(href, "#comment")
					if len(parts) > 1 {
						commentID = parts[1]
					}
				}
				uParsed, err := url.Parse(href)
				if err == nil {
					p := uParsed.Path
					p = strings.TrimPrefix(p, "/view/")
					torrentID = p
				}
				txt := strings.TrimSpace(a.Text())
				if txt != "Comment" && txt != "Delete" && torrentTitle == "" {
					torrentTitle = txt
				}
			} else if strings.Contains(href, "/feedback") {
				torrentID = "feedback"
				torrentTitle = "Feedback"
				if strings.Contains(href, "#comment") {
					parts := strings.Split(href, "#comment")
					if len(parts) > 1 {
						commentID = parts[1]
					}
				}
			}
		})

		if torrentID == "" || commentID == "" {
			return
		}

		username := strings.TrimSpace(commentUser.Find("strong").Text())
		if strings.HasPrefix(username, "Anonymous") {
			if strings.Contains(username, ":") {
				parts := strings.SplitN(username, ":", 2)
				nick := strings.Trim(parts[1], " \t\r\n\"")
				if nick != "" {
					username = fmt.Sprintf("Anonymous (%s)", nick)
				} else {
					username = "Anonymous"
				}
			}
		}

		allText := commentUser.Text()
		var timeStr string
		if idx := strings.LastIndex(allText, "—"); idx != -1 {
			timeStr = allText[idx+1:]
		}

		timestamp := parseATTime(timeStr)

		// Parse Comment Type
		cType := "Torrent"
		typeSel := commentUser.Find("span.comment_type")
		if typeSel.Length() > 0 {
			cType = typeSel.AttrOr("title", "Torrent")
		} else if torrentID == "feedback" {
			cType = "Feedback"
		}

		messageDiv := sel.Find("div.user_message_c")
		message := strings.TrimSpace(messageDiv.Text())

		comments = append(comments, ATComment{
			ID:        commentID,
			TorrentID: torrentID,
			Title:     torrentTitle,
			Username:  username,
			Message:   message,
			Timestamp: timestamp,
			Type:      cType,
		})
	})

	hasNext := false
	nextPageStr := strconv.Itoa(page + 1)
	doc.Find("div.pagination a").Each(func(i int, sel *goquery.Selection) {
		title, _ := sel.Attr("title")
		if strings.Contains(title, "Go to page "+nextPageStr) || strings.TrimSpace(sel.Text()) == nextPageStr {
			hasNext = true
		}
	})

	return comments, hasNext, nil
}
