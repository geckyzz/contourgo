package discord

import (
	"testing"

	"github.com/geckyzz/contourgo/internal/db"
)

func TestExtractImageURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Markdown Image",
			input:    "![alt](https://example.com/image.png)",
			expected: "https://example.com/image.png",
		},
		{
			name:     "Markdown Image with whitespace",
			input:    "  ![alt](https://example.com/image.png)  ",
			expected: "https://example.com/image.png",
		},
		{
			name:     "BBCode Image",
			input:    "[img]https://example.com/image.png[/img]",
			expected: "https://example.com/image.png",
		},
		{
			name:     "BBCode Image Case Insensitive",
			input:    "[IMG]https://example.com/image.png[/IMG]",
			expected: "https://example.com/image.png",
		},
		{
			name:     "Multiple lines Markdown",
			input:    "\n![alt](https://example.com/image.png)\n",
			expected: "https://example.com/image.png",
		},
		{
			name:     "Not a single image (text before)",
			input:    "Hello ![alt](https://example.com/image.png)",
			expected: "",
		},
		{
			name:     "Not a single image (text after)",
			input:    "![alt](https://example.com/image.png) World",
			expected: "",
		},
		{
			name:     "Plain URL (not handled by extractImageURL)",
			input:    "https://example.com/image.png",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractImageURL(tt.input); got != tt.expected {
				t.Errorf("extractImageURL() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestResolveNyaaMentions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		siteBase string
		expected string
	}{
		{
			name:     "Simple mention at start",
			input:    "@someone hello",
			siteBase: "nyaa.si",
			expected: "[@someone](https://nyaa.si/user/someone) hello",
		},
		{
			name:     "Simple mention in middle",
			input:    "hello @someone world",
			siteBase: "nyaa.si",
			expected: "hello [@someone](https://nyaa.si/user/someone) world",
		},
		{
			name:     "Multiple mentions",
			input:    "@user-one and @user_two",
			siteBase: "sukebei.nyaa.si",
			expected: "[@user-one](https://sukebei.nyaa.si/user/user-one) and [@user_two](https://sukebei.nyaa.si/user/user_two)",
		},
		{
			name:     "Email address (should not match)",
			input:    "test@example.com",
			siteBase: "nyaa.si",
			expected: "test@example.com",
		},
		{
			name:     "Mention with punctuation",
			input:    "@someone, hello!",
			siteBase: "nyaa.si",
			expected: "[@someone](https://nyaa.si/user/someone), hello!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveNyaaMentions(tt.input, tt.siteBase); got != tt.expected {
				t.Errorf("resolveNyaaMentions() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildNyaaEmbed_DefaultAvatar(t *testing.T) {
	bot := &DiscordBot{}

	torrent := db.Torrent{
		TorrentID: "2083384",
		Title:     "Test Torrent",
	}
	comment := db.Comment{
		Username:  "someuser",
		Position:  1,
		AvatarURL: "/static/img/avatar/default.png",
		Message:   "please seed",
		Timestamp: 1777144089,
	}

	embed := bot.BuildNyaaEmbed("nyaa", "https://author-icon.png", torrent, comment, true, false)

	if embed.Thumbnail == nil {
		t.Fatalf("Expected embed Thumbnail to be set, but got nil")
	}

	expectedThumbnailURL := "https://nyaa.si/static/img/avatar/default.png"
	if embed.Thumbnail.URL != expectedThumbnailURL {
		t.Errorf(
			"Expected thumbnail URL to be %q, got %q",
			expectedThumbnailURL,
			embed.Thumbnail.URL,
		)
	}
}

func TestBuildNyaaEmbed_CustomAvatar(t *testing.T) {
	bot := &DiscordBot{}

	torrent := db.Torrent{
		TorrentID: "2085720",
		Title:     "Test Torrent",
	}
	comment := db.Comment{
		Username:  "anotheruser",
		Position:  1,
		AvatarURL: "https://i1.wp.com/nyaa.si/user/anotheruser/avatar-WP25-A?ssl=1",
		Message:   "Please seed, I missed this one...",
		Timestamp: 1777144089,
	}

	embed := bot.BuildNyaaEmbed("nyaa", "https://author-icon.png", torrent, comment, true, false)

	if embed.Thumbnail == nil {
		t.Fatalf("Expected embed Thumbnail to be set, but got nil")
	}

	expectedThumbnailURL := "https://i1.wp.com/nyaa.si/user/anotheruser/avatar-WP25-A?ssl=1"
	if embed.Thumbnail.URL != expectedThumbnailURL {
		t.Errorf(
			"Expected thumbnail URL to be %q, got %q",
			expectedThumbnailURL,
			embed.Thumbnail.URL,
		)
	}
}

func TestBuildNyaaEmbed_DynamicReply(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	bot := &DiscordBot{
		DB: database,
	}

	torrent := db.Torrent{
		TorrentID: "2085681",
		Title:     "Everyday HOST CLUB",
	}

	// Store Moses35i comment
	err = database.StoreComment(
		"nyaa",
		"2085681",
		"1518581367766782033",
		"Moses35i",
		"Any news on the last episode?",
		1777144089,
		3,
		"",
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("Failed to store Moses35i comment: %v", err)
	}

	// Build embed for Subcaln't comment that mentions Moses35i
	comment := db.Comment{
		Username:  "Subcaln't",
		Position:  4,
		Message:   "@Moses35i on it. I decided to go solo once again",
		Timestamp: 1777144200,
	}

	embed := bot.BuildNyaaEmbed("nyaa", "https://author-icon.png", torrent, comment, true, false)

	var hasReplyingTo bool
	for _, field := range embed.Fields {
		if field.Name == "↪️ Replying to" {
			hasReplyingTo = true
			if field.Value != "Any news on the last episode?" {
				t.Errorf(
					"Expected Replying to value to be %q, got %q",
					"Any news on the last episode?",
					field.Value,
				)
			}
		}
	}

	if !hasReplyingTo {
		t.Errorf("Expected embed to have '↪️ Replying to' field, but it did not")
	}
}
