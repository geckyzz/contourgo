package discord

import "testing"

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
