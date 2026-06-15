package scraper

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func TestParseATTime(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{
			name:     "Today format",
			input:    "Today 18:38",
			expected: time.Date(now.Year(), now.Month(), now.Day(), 18, 38, 0, 0, time.UTC).Unix(),
		},
		{
			name:  "Yesterday format",
			input: "Yesterday 16:35",
			expected: time.Date(now.Year(), now.Month(), now.Day(), 16, 35, 0, 0, time.UTC).
				AddDate(0, 0, -1).
				Unix(),
		},
		{
			name:     "Absolute date format",
			input:    "24/05/2026 12:48",
			expected: time.Date(2026, time.May, 24, 12, 48, 0, 0, time.UTC).Unix(),
		},
		{
			name:     "Absolute date format with short year",
			input:    "24/05/26 12:48",
			expected: time.Date(2026, time.May, 24, 12, 48, 0, 0, time.UTC).Unix(),
		},
		{
			name:     "Absolute date format with UTC suffix",
			input:    "12/06/2026 18:46 UTC",
			expected: time.Date(2026, time.June, 12, 18, 46, 0, 0, time.UTC).Unix(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseATTime(tt.input)
			if got != tt.expected {
				t.Errorf(
					"parseATTime(%q) = %v, want %v",
					tt.input,
					time.Unix(got, 0).UTC(),
					time.Unix(tt.expected, 0).UTC(),
				)
			}
		})
	}
}

func TestProcessMessageLinks(t *testing.T) {
	const htmlInput = `<div class="comment_message">
		<a href="https://example.com/project/abcdef1234567890abcdef1234567890">https://example.com/project/abcdef12...34567890</a><br/>
		^^ DDLs here ^^<br/><br/>
		If the torrent is on NekoBT or AnimeTosho and under 10G you can post its link at <a href="https://example.test/notify">https://example.test/notify</a> to auto parse it...
		Read more at <a href="/feedback?page=44#comment946">feedback page</a>
	</div>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlInput))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	sel := doc.Find("div.comment_message")
	sel.Find("br").ReplaceWithHtml("\n")

	// Apply processing
	sel.Find("a").Each(func(i int, aSel *goquery.Selection) {
		href, exists := aSel.Attr("href")
		if !exists {
			return
		}
		if strings.HasPrefix(href, "/") {
			href = "https://animetosho.xyz" + href
		}
		text := strings.TrimSpace(aSel.Text())
		if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
			aSel.ReplaceWithHtml(href)
		} else {
			aSel.ReplaceWithHtml("[" + text + "](" + href + ")")
		}
	})

	text := strings.TrimSpace(sel.Text())

	// Standardize whitespace for comparison (replace tabs/multiple spaces or check contains)
	if !strings.Contains(text, "https://example.com/project/abcdef1234567890abcdef1234567890") {
		t.Errorf("expected text to contain the full restored URL, but got:\n%s", text)
	}
	if !strings.Contains(text, "[feedback page](https://animetosho.xyz/feedback?page=44#comment946)") {
		t.Errorf("expected text to contain the markdown link, but got:\n%s", text)
	}
}
