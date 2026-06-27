package discord

import (
	"strings"
	"testing"

	"github.com/geckyzz/contourgo/internal/config"
)

// ── RewriteTweetURL ────────────────────────────────────────────────────────────

func TestRewriteTweetURL(t *testing.T) {
	tests := []struct {
		name         string
		link         string
		embedService string
		want         string
	}{
		{
			name:         "empty embed_service — fallback to x.com",
			link:         "https://nitter.net/mofusand_anime/status/1234567890",
			embedService: "",
			want:         "https://x.com/mofusand_anime/status/1234567890",
		},
		{
			name:         "short name fixupx",
			link:         "https://nitter.net/mofusand_anime/status/1234567890",
			embedService: "fixupx",
			want:         "https://fixupx.com/mofusand_anime/status/1234567890",
		},
		{
			name:         "short name vxtwitter",
			link:         "https://twitter.com/mofusand_anime/status/1234567890",
			embedService: "vxtwitter",
			want:         "https://vxtwitter.com/mofusand_anime/status/1234567890",
		},
		{
			name:         "short name fxtwitter",
			link:         "https://x.com/mofusand_anime/status/9876543210",
			embedService: "fxtwitter",
			want:         "https://fxtwitter.com/mofusand_anime/status/9876543210",
		},
		{
			name:         "short name twittpr",
			link:         "https://twitter.com/mofusand_anime/status/1111111111",
			embedService: "twittpr",
			want:         "https://twittpr.com/mofusand_anime/status/1111111111",
		},
		{
			name:         "bare domain string",
			link:         "https://nitter.net/mofusand_anime/status/1234567890",
			embedService: "fixvx.com",
			want:         "https://fixvx.com/mofusand_anime/status/1234567890",
		},
		{
			name:         "case insensitive short name",
			link:         "https://nitter.net/mofusand_anime/status/1234567890",
			embedService: "FIXUPX",
			want:         "https://fixupx.com/mofusand_anime/status/1234567890",
		},
		{
			name:         "unknown service without dot — no change",
			link:         "https://nitter.net/mofusand_anime/status/1234567890",
			embedService: "notadomain",
			want:         "https://nitter.net/mofusand_anime/status/1234567890",
		},
		{
			name:         "x.com link rewrites correctly",
			link:         "https://x.com/someuser/status/4444444444",
			embedService: "vxtwitter",
			want:         "https://vxtwitter.com/someuser/status/4444444444",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RewriteTweetURL(tt.link, tt.embedService)
			if got != tt.want {
				t.Errorf(
					"RewriteTweetURL(%q, %q) = %q; want %q",
					tt.link,
					tt.embedService,
					got,
					tt.want,
				)
			}
		})
	}
}

// ── ExtractTweetIDFromURL ──────────────────────────────────────────────────────

func TestExtractTweetIDFromURL(t *testing.T) {
	tests := []struct {
		name   string
		link   string
		wantID string
	}{
		{"twitter.com link", "https://twitter.com/mofusand_anime/status/1234567890", "1234567890"},
		{"x.com link", "https://x.com/mofusand_anime/status/9876543210", "9876543210"},
		{
			"www.twitter.com",
			"https://www.twitter.com/user/status/1111222233334444",
			"1111222233334444",
		},
		{"nitter.net link", "https://nitter.net/mofusand_anime/status/5555555555", "5555555555"},
		{"nitter subdomain", "https://nitter.privacydev.net/user/status/6666666666", "6666666666"},
		{"no status segment", "https://twitter.com/mofusand_anime", ""},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTweetIDFromURL(tt.link)
			if got != tt.wantID {
				t.Errorf("ExtractTweetIDFromURL(%q) = %q; want %q", tt.link, got, tt.wantID)
			}
		})
	}
}

// ── ParseNitterRSSPubDate ──────────────────────────────────────────────────────

func TestParseNitterRSSPubDate(t *testing.T) {
	// Empty / garbage should fall back to time.Now() (non-zero).
	for _, s := range []string{"", "not-a-date"} {
		if ts := ParseNitterRSSPubDate(s); ts == 0 {
			t.Errorf("ParseNitterRSSPubDate(%q) = 0; want non-zero fallback", s)
		}
	}

	// Verify a specific RFC1123Z timestamp parses to the correct Unix value.
	ts := ParseNitterRSSPubDate("Sat, 01 Jan 2000 00:00:00 +0000")
	if ts != 946684800 {
		t.Errorf("ParseNitterRSSPubDate RFC1123Z = %d; want 946684800", ts)
	}

	// RFC3339 should also parse.
	ts2 := ParseNitterRSSPubDate("2000-01-01T00:00:00Z")
	if ts2 != 946684800 {
		t.Errorf("ParseNitterRSSPubDate RFC3339 = %d; want 946684800", ts2)
	}
}

// ── renderCustomFormat ─────────────────────────────────────────────────────────

func TestRenderCustomFormat(t *testing.T) {
	data := TwitterPostData{
		Account:      "mofusand_anime",
		DisplayName:  "もふさんど",
		TweetID:      "1234567890",
		Link:         "https://fixupx.com/mofusand_anime/status/1234567890",
		OriginalLink: "https://twitter.com/mofusand_anime/status/1234567890",
		Title:        "New artwork dropped!",
		PublishedAt:  946684800,
	}

	tests := []struct {
		name    string
		format  string
		want    string
		wantErr bool
	}{
		{
			name:   "simple link",
			format: "{{.Link}}",
			want:   "https://fixupx.com/mofusand_anime/status/1234567890",
		},
		{
			name:   "full custom message",
			format: "📢 **@{{.Account}}** tweeted: {{.Link}}",
			want:   "📢 **@mofusand_anime** tweeted: https://fixupx.com/mofusand_anime/status/1234567890",
		},
		{
			name:   "all placeholders",
			format: "{{.Account}} {{.DisplayName}} {{.TweetID}} {{.OriginalLink}} {{.Title}}",
			want:   "mofusand_anime もふさんど 1234567890 https://twitter.com/mofusand_anime/status/1234567890 New artwork dropped!",
		},
		{
			name:   "published at as int",
			format: "ts={{.PublishedAt}}",
			want:   "ts=946684800",
		},
		{
			name:    "invalid template syntax",
			format:  "{{.Unclosed",
			wantErr: true,
		},
		{
			name:    "non-existent field",
			format:  "{{.NonExistent}}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderCustomFormat(tt.format, data)
			if tt.wantErr {
				if err == nil {
					t.Errorf("renderCustomFormat(%q) expected error, got %q", tt.format, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("renderCustomFormat(%q) unexpected error: %v", tt.format, err)
			}
			if got != tt.want {
				t.Errorf("renderCustomFormat(%q) = %q; want %q", tt.format, got, tt.want)
			}
		})
	}
}

// ── Content-building logic (no Discord session required) ───────────────────────

func TestRewriteTweetURL_EmbedServiceProducesCorrectDomain(t *testing.T) {
	rawLink := "https://nitter.net/mofusand_anime/status/1234567890"
	cases := []struct {
		svc    string
		prefix string
	}{
		{"fixupx", "https://fixupx.com/"},
		{"vxtwitter", "https://vxtwitter.com/"},
		{"fxtwitter", "https://fxtwitter.com/"},
		{"twittpr", "https://twittpr.com/"},
		{"", "https://x.com/"}, // fall back to x.com
	}
	for _, c := range cases {
		t.Run("embed_service="+c.svc, func(t *testing.T) {
			result := RewriteTweetURL(rawLink, c.svc)
			if !strings.HasPrefix(result, c.prefix) {
				t.Errorf("RewriteTweetURL(%q) = %q; want prefix %q", c.svc, result, c.prefix)
			}
		})
	}
}

func TestRenderCustomFormat_ContainsRewrittenLink(t *testing.T) {
	monitorCfg := config.MonitorConfig{
		EmbedService: "fixupx",
		CustomFormat: "🐱 **@{{.Account}}** — {{.Link}}",
	}

	rawLink := "https://nitter.net/mofusand_anime/status/1234567890"
	data := TwitterPostData{
		Account: "mofusand_anime",
		TweetID: "1234567890",
		Link:    RewriteTweetURL(rawLink, monitorCfg.EmbedService),
		Title:   "cute art",
	}

	rendered, err := renderCustomFormat(monitorCfg.CustomFormat, data)
	if err != nil {
		t.Fatalf("renderCustomFormat() error: %v", err)
	}
	if !strings.Contains(rendered, "https://fixupx.com/mofusand_anime/status/1234567890") {
		t.Errorf("rendered %q missing fixupx link", rendered)
	}
}

func TestNoEmbedService_FallsBackToX(t *testing.T) {
	rawLink := "https://nitter.net/mofusand_anime/status/1234567890"
	got := RewriteTweetURL(rawLink, "")
	want := "https://x.com/mofusand_anime/status/1234567890"
	if got != want {
		t.Errorf("expected link rewritten to x.com when no embed_service; got %q", got)
	}
}
