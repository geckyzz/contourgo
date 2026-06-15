package scraper

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type NyaaTorrent struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
	Comments    int    `json:"comments"`
	Downloads   int    `json:"downloads"`
	Seeders     int    `json:"seeders"`
	Leechers    int    `json:"leechers"`
	Size        string `json:"size"`
	UploadDate  string `json:"uploadDate"`
	Magnet      string `json:"magnet"`
	Download    string `json:"download"`
	InfoHash    string `json:"infoHash"`
	Trusted     bool   `json:"trusted"`
	Remake      bool   `json:"remake"`
}

type NyaaSearchResult struct {
	Torrents   []NyaaTorrent `json:"torrents"`
	Pagination struct {
		CurrentPage  int `json:"currentPage"`
		TotalPages   int `json:"totalPages"`
		TotalResults int `json:"totalResults"`
	} `json:"pagination"`
}

type NyaaComment struct {
	ID        int    `json:"id"`
	Pos       int    `json:"pos"`
	Username  string `json:"username"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"` // ISO 8601 string
	Role      string `json:"role"`
	Avatar    string `json:"avatar"`
}

type ATComment struct {
	ID        string
	TorrentID string
	Title     string
	Username  string
	Message   string
	Timestamp int64
	Type      string // Torrent, Feedback, DDL, Question, etc.
}

func parseATTime(timeStr string) int64 {
	timeStr = strings.TrimSpace(timeStr)
	timeStr = strings.TrimPrefix(timeStr, "—")
	timeStr = strings.TrimSpace(timeStr)

	now := time.Now().UTC()
	if strings.Contains(timeStr, "Today") {
		parts := strings.Fields(timeStr)
		if len(parts) >= 2 {
			timePart := parts[1]
			var hour, min int
			fmt.Sscanf(timePart, "%d:%d", &hour, &min)
			dt := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)
			return dt.Unix()
		}
	} else if strings.Contains(timeStr, "Yesterday") {
		parts := strings.Fields(timeStr)
		if len(parts) >= 2 {
			timePart := parts[1]
			var hour, min int
			fmt.Sscanf(timePart, "%d:%d", &hour, &min)
			yest := now.AddDate(0, 0, -1)
			dt := time.Date(yest.Year(), yest.Month(), yest.Day(), hour, min, 0, 0, time.UTC)
			return dt.Unix()
		}
	} else {
		var day, month, year, hour, min int
		_, err := fmt.Sscanf(timeStr, "%d/%d/%d %d:%d", &day, &month, &year, &hour, &min)
		if err == nil {
			if year < 100 {
				year += 2000
			}
			dt := time.Date(year, time.Month(month), day, hour, min, 0, 0, time.UTC)
			return dt.Unix()
		}
	}
	return now.Unix()
}

func DecodeNekoBTSnowflake(idStr string) int64 {
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return time.Now().UnixMilli()
	}

	const epoch int64 = 1735689600000 // 2025-01-01T00:00:00.000Z
	// Last 8 bits are Type (4 bits) and Increment (4 bits)
	timestampMs := int64(id>>8) + epoch
	return timestampMs
}

type NekoBTComment struct {
	ID          string          `json:"id"`
	Text        string          `json:"text"`
	ReplyingTo  *string         `json:"replying_to"`
	DisplayName string          `json:"display_name"`
	UserID      string          `json:"user_id"`
	PfpHash     *string         `json:"pfp_hash"`
	CreatedAt   int64           `json:"created_at"` // Assuming Unix milliseconds
	LastEdit    *int64          `json:"last_edit"`  // Unix timestamp in milliseconds
	Deleted     bool            `json:"deleted"`
	Children    []NekoBTComment `json:"children"`

	// Derived fields
	ParentText     string   `json:"-"`
	TorrentID      string   `json:"-"`
	Title          string   `json:"-"`
	UploaderID     string   `json:"-"`
	ContributorIDs []string `json:"-"`
}

type NekoBTTorrent struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CommentCount string `json:"comment_count"` // API returns string for some reason?
	UploadedAt   int64  `json:"uploaded_at"`   // Unix ms
}

type NekoBTResponse struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type NekoBTSearchResult struct {
	Results []NekoBTTorrent `json:"results"`
	More    bool            `json:"more"`
}

func processMessageLinks(messageDiv *goquery.Selection, baseURL string) {
	messageDiv.Find("a").Each(func(i int, aSel *goquery.Selection) {
		href, exists := aSel.Attr("href")
		if !exists {
			return
		}
		// Resolve relative URL
		if strings.HasPrefix(href, "/") {
			href = baseURL + href
		}

		text := strings.TrimSpace(aSel.Text())

		// If text looks like a URL (starts with http:// or https://), replace with the full href URL.
		if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
			aSel.ReplaceWithHtml(html.EscapeString(href))
		} else {
			// Otherwise format as a markdown link
			markdownLink := fmt.Sprintf("[%s](%s)", text, href)
			aSel.ReplaceWithHtml(html.EscapeString(markdownLink))
		}
	})
}

func ResolveATParent(doc *goquery.Document, commentID string) (parentID string, parentText string) {
	// 1. Try New Layout (XYZ)
	var targetCommentIndex = -1
	type commentItem struct {
		id    string
		depth int
		text  string
	}
	var items []commentItem
	doc.Find("#view_comments div.comment, #view_comments div.comment2").
		Each(func(i int, sel *goquery.Selection) {
			idAttr, _ := sel.Attr("id")
			if !strings.HasPrefix(idAttr, "comment") {
				return
			}
			id := strings.TrimPrefix(idAttr, "comment")

			var d int
			classes := sel.AttrOr("class", "")
			for _, class := range strings.Fields(classes) {
				if strings.HasPrefix(class, "comment-depth-") {
					if val, err := strconv.Atoi(strings.TrimPrefix(class, "comment-depth-")); err == nil {
						d = val
						break
					}
				}
			}

			msgSel := sel.Find("div.comment_message")
			msgSelCopy := msgSel.Clone()
			msgSelCopy.Find("br").ReplaceWithHtml("\n")
			msg := strings.TrimSpace(msgSelCopy.Text())

			items = append(items, commentItem{
				id:    id,
				depth: d,
				text:  msg,
			})
			if id == commentID {
				targetCommentIndex = len(items) - 1
			}
		})

	if targetCommentIndex != -1 {
		targetDepth := items[targetCommentIndex].depth
		if targetDepth > 0 {
			for i := targetCommentIndex - 1; i >= 0; i-- {
				if items[i].depth == targetDepth-1 {
					return items[i].id, items[i].text
				}
			}
		}
		return "", ""
	}

	// 2. Try Old Layout (ORG)
	bodySel := doc.Find(fmt.Sprintf("#comment_body_%s", commentID))
	if bodySel.Length() > 0 {
		curr := bodySel.Parent()
		for {
			curr = curr.Parent()
			if curr.Length() == 0 {
				break
			}
			var foundID string
			curr.Children().Each(func(i int, child *goquery.Selection) {
				idAttr, _ := child.Attr("id")
				if strings.HasPrefix(idAttr, "comment_body_") {
					foundID = strings.TrimPrefix(idAttr, "comment_body_")
				}
			})
			if foundID != "" {
				parentID = foundID
				parentBodySel := curr.Find(fmt.Sprintf("#comment_body_%s", parentID))
				msgSel := parentBodySel.Find("div.user_message_c").First()
				msgSelCopy := msgSel.Clone()
				msgSelCopy.Find("br").ReplaceWithHtml("\n")
				parentText = strings.TrimSpace(msgSelCopy.Text())
				return parentID, parentText
			}
		}
	}

	return "", ""
}

func ResolveParentInfo(client *http.Client, targetURL string, commentID string) (string, string) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set(
		"User-Agent",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	)
	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ""
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", ""
	}
	return ResolveATParent(doc, commentID)
}
