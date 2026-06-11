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
