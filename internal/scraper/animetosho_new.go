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

type AnimeToshoNewScraper struct {
	baseURL string
	client  *http.Client
}

func NewAnimeToshoNewScraper() *AnimeToshoNewScraper {
	return &AnimeToshoNewScraper{
		baseURL: "https://animetosho.xyz",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *AnimeToshoNewScraper) ScrapeComments(
	page int,
	q string,
	feedback bool,
) ([]ATComment, bool, error) {
	qVals := url.Values{}
	if page > 1 {
		qVals.Set("page", strconv.Itoa(page))
	}
	if q != "" {
		qVals.Set("q", q)
	}
	if feedback {
		qVals.Set("feedback", "1")
	} else {
		qVals.Set("torrent", "1")
	}

	u := fmt.Sprintf("%s/comments?%s", s.baseURL, qVals.Encode())
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set(
		"User-Agent",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	)

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

	// Parse New AnimeTosho Layout (XYZ)
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
			if strings.Contains(href, "#comment") {
				parts := strings.Split(href, "#comment")
				if len(parts) > 1 {
					commentID = parts[1]
				}
			}

			if strings.Contains(href, "/view/") {
				uParsed, err := url.Parse(href)
				if err == nil {
					p := uParsed.Path
					p = strings.TrimPrefix(p, "/view/")
					torrentID = p
				}
				txt := strings.TrimSpace(a.Text())
				if txt != "Comment" && txt != "Delete" && txt != "DDL" && torrentTitle == "" {
					torrentTitle = txt
				}
			} else if strings.Contains(href, "/feedback") {
				if strings.Contains(href, "#comment") {
					torrentID = "feedback"
					torrentTitle = "Feedback"
					uParsed, err := url.Parse(href)
					if err == nil && uParsed.RawQuery != "" {
						torrentID = "feedback?" + uParsed.RawQuery
					}
				}
			}
		})

		// If no commentID was found (e.g. it was a DDL entry and we removed the DDL detection), skip it
		if commentID == "" || torrentID == "" {
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

		userText := commentUser.Text()
		var timestamp int64 = time.Now().Unix()
		if _, after, ok := strings.Cut(userText, "posted on "); ok {
			datePart := strings.TrimSpace(after)
			datePart = strings.TrimSuffix(datePart, " UTC")
			datePart = strings.TrimSpace(datePart)
			timestamp = parseATTime(datePart)
		}

		// Parse Comment Type
		cType := "Torrent"
		typeSel := commentUser.Find("span").First()
		if typeSel.Length() > 0 && typeSel.Parent().Is("div") {
			cType = strings.Trim(typeSel.Text(), "()")
		}

		messageDiv := sel.Find("div.comment_message")
		processMessageLinks(messageDiv, s.baseURL)
		messageDiv.Find("br").ReplaceWithHtml("\n")
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
	doc.Find("div.home_list_pagination a").Each(func(i int, sel *goquery.Selection) {
		if strings.Contains(sel.Text(), "Next Page") {
			hasNext = true
		}
	})

	return comments, hasNext, nil
}
