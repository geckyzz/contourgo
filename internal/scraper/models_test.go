package scraper

import (
	"encoding/json"
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
			got := parseATTime(tt.input, time.Time{})
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
	if !strings.Contains(
		text,
		"[feedback page](https://animetosho.xyz/feedback?page=44#comment946)",
	) {
		t.Errorf("expected text to contain the markdown link, but got:\n%s", text)
	}
}

func TestResolveATParent(t *testing.T) {
	t.Run("New Layout (XYZ)", func(t *testing.T) {
		const newLayoutHTML = `<div id="view_comments">
			<div id="comment837192" class="comment comment-depth-0">
				<div class="comment_user"><strong>Anonymous</strong> posted on 15/06/2026 04:27 UTC</div>
				<div id="comment_body_837192">
					<div class="comment_message">Lorem ipsum dolor sit amet, consectetur adipiscing elit.</div>
				</div>
			</div>
			<div id="comment928374" class="comment2 comment-depth-1">
				<div class="comment_user"><strong>Anonymous</strong> posted on 15/06/2026 11:42 UTC</div>
				<div id="comment_body_928374">
					<div class="comment_message">Sed do eiusmod tempor incididunt ut labore.</div>
				</div>
			</div>
		</div>`

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(newLayoutHTML))
		if err != nil {
			t.Fatalf("failed to parse: %v", err)
		}

		// Comment 928374 is a child of 837192
		pID, pText := ResolveATParent(doc, "928374")
		if pID != "837192" {
			t.Errorf("expected parent ID to be 837192, got %s", pID)
		}
		if pText != "Lorem ipsum dolor sit amet, consectetur adipiscing elit." {
			t.Errorf("expected parent text, got %q", pText)
		}

		// Comment 837192 is root (no parent)
		pID, pText = ResolveATParent(doc, "837192")
		if pID != "" || pText != "" {
			t.Errorf("expected no parent for 837192, got ID=%q, text=%q", pID, pText)
		}
	})

	t.Run("Old Layout (ORG)", func(t *testing.T) {
		const oldLayoutHTML = `<div id="view_comments_real">
			<div id="view_comments">
				<a id="comment748392"></a>
				<div class="comment">
					<div class="comment_user">18/02/2026 14:05 — <strong>Anonymous</strong></div>
					<div id="comment_body_748392">
						<div class="comment_message"><div class="user_message_c">Lorem ipsum dolor sit amet.</div></div>
						<a id="comment837492"></a>
						<div class="comment2">
							<div class="comment_user">18/02/2026 14:15 — <strong>Anonymous</strong></div>
							<div id="comment_body_837492">
								<div class="comment_message"><div class="user_message_c">Consectetur adipiscing elit.</div></div>
								<a id="comment928301"></a>
								<div class="comment">
									<div class="comment_user">18/02/2026 14:30 — <strong>Anonymous</strong></div>
									<div id="comment_body_928301">
										<div class="comment_message"><div class="user_message_c">Sed do eiusmod tempor.</div></div>
									</div>
								</div>
							</div>
						</div>
					</div>
				</div>
			</div>
		</div>`

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(oldLayoutHTML))
		if err != nil {
			t.Fatalf("failed to parse: %v", err)
		}

		// 837492 is child of 748392
		pID, pText := ResolveATParent(doc, "837492")
		if pID != "748392" {
			t.Errorf("expected parent ID to be 748392, got %s", pID)
		}
		if pText != "Lorem ipsum dolor sit amet." {
			t.Errorf("expected parent text, got %q", pText)
		}

		// 928301 is child of 837492
		pID, pText = ResolveATParent(doc, "928301")
		if pID != "837492" {
			t.Errorf("expected parent ID to be 837492, got %s", pID)
		}
		if pText != "Consectetur adipiscing elit." {
			t.Errorf("expected parent text, got %q", pText)
		}

		// 748392 is root
		pID, pText = ResolveATParent(doc, "748392")
		if pID != "" || pText != "" {
			t.Errorf("expected no parent for 748392, got ID=%q, text=%q", pID, pText)
		}
	})

	t.Run("New Nested Layout (XYZ)", func(t *testing.T) {
		const newNestedHTML = `<div id="view_comments">
			<div class="comment">
				<div class="comment_user">
					<span onclick="var e=document.getElementById('comment_body_1204').style; e.display=e.display?'':'none';" id="comment_mod_1204">
						Yesterday 22:18 —
						<strong><em>Anonymous</em></strong>
					</span>
				</div>
				<div id="comment_body_1204">
					<div id="comment_message_1204" class="comment_message">This is a parent comment.</div>
					<a id="comment1207"></a>
					<div class="comment2">
						<div class="comment_user">
							<span id="comment_mod_1207">
								Yesterday 22:25 —
								<strong><em>Anonymous</em></strong>
							</span>
						</div>
						<div id="comment_body_1207">
							<div id="comment_message_1207" class="comment_message">This is a nested reply.</div>
						</div>
					</div>
				</div>
			</div>
		</div>`

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(newNestedHTML))
		if err != nil {
			t.Fatalf("failed to parse: %v", err)
		}

		// 1207 is child of 1204
		pID, pText := ResolveATParent(doc, "1207")
		if pID != "1204" {
			t.Errorf("expected parent ID to be 1204, got %s", pID)
		}
		if pText != "This is a parent comment." {
			t.Errorf("expected parent text, got %q", pText)
		}

		// 1204 is root
		pID, pText = ResolveATParent(doc, "1204")
		if pID != "" || pText != "" {
			t.Errorf("expected no parent for 1204, got ID=%q, text=%q", pID, pText)
		}
	})
}

func TestFlexBool(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Int 1 is true",
			input:    `1`,
			expected: true,
		},
		{
			name:     "Int 0 is false",
			input:    `0`,
			expected: false,
		},
		{
			name:     "Bool true is true",
			input:    `true`,
			expected: true,
		},
		{
			name:     "Bool false is false",
			input:    `false`,
			expected: false,
		},
		{
			name:     "Float 1.0 is true",
			input:    `1.0`,
			expected: true,
		},
		{
			name:     "Float 0.0 is false",
			input:    `0.0`,
			expected: false,
		},
		{
			name:     "String 'true' is true",
			input:    `"true"`,
			expected: true,
		},
		{
			name:     "String '1' is true",
			input:    `"1"`,
			expected: true,
		},
		{
			name:     "String 'yes' is true",
			input:    `"yes"`,
			expected: true,
		},
		{
			name:     "String 'false' is false",
			input:    `"false"`,
			expected: false,
		},
		{
			name:     "Null is false",
			input:    `null`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tb FlexBool
			if err := json.Unmarshal([]byte(tt.input), &tb); err != nil {
				t.Fatalf("Failed to unmarshal %s: %v", tt.input, err)
			}
			if bool(tb) != tt.expected {
				t.Errorf("UnmarshalJSON(%s) = %v, want %v", tt.input, bool(tb), tt.expected)
			}
		})
	}
}
