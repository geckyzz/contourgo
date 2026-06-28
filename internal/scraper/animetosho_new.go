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
		client:  NewHTTPClient(30 * time.Second),
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
	doc, err := fetchGoqueryDocument(s.client, u)
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

		username := parseATUsername(commentUser.Find("strong").Text())

		userText := commentUser.Text()
		// If userText has multiple lines, find the one containing Today, Yesterday, or a DD/MM/YY date.
		var datePart string
		lines := strings.Split(userText, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "Today") || strings.Contains(line, "Yesterday") ||
				strings.Contains(line, "/") {
				// Cut off anything after a dash if present
				if idx := strings.Index(line, "—"); idx != -1 {
					line = line[:idx]
				}
				if idx := strings.Index(line, "\u2014"); idx != -1 {
					line = line[:idx]
				}
				datePart = strings.TrimSpace(line)
				break
			}
		}

		if datePart == "" {
			if _, after, ok := strings.Cut(userText, "posted on "); ok {
				datePart = after
			} else if idx := strings.Index(userText, "—"); idx != -1 {
				datePart = userText[:idx]
			} else if idx := strings.Index(userText, "\u2014"); idx != -1 {
				datePart = userText[:idx]
			} else {
				datePart = userText
			}
		}

		// Clean up the date string
		datePart = strings.TrimSpace(datePart)
		datePart = strings.TrimSuffix(datePart, " UTC")
		datePart = strings.TrimSpace(datePart)

		var timestamp int64 = time.Now().Unix()
		if datePart != "" {
			timestamp = parseATTime(datePart, time.Time{})
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

	for i := range comments {
		comments[i].Unescape()
	}
	return comments, hasNext, nil
}
