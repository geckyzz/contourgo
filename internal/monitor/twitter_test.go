package monitor

import (
	"regexp"
	"testing"
)

func TestNormaliseToXLink(t *testing.T) {
	got := normaliseToXLink("mofusand_anime", "1234567890")
	want := "https://x.com/mofusand_anime/status/1234567890"
	if got != want {
		t.Errorf("normaliseToXLink() = %q; want %q", got, want)
	}
}

func TestTwitterRegexFilters(t *testing.T) {
	title := "🐾今日の #mofusand🐾 にゃー！！ #ちびにゃん"

	// Match case
	kw := "mofusand"
	re, err := regexp.Compile("(?i)" + kw)
	if err != nil {
		t.Fatalf("regex compile error: %v", err)
	}
	if !re.MatchString(title) {
		t.Errorf("expected keyword %q to match title %q", kw, title)
	}

	// Case-insensitive match case
	kw2 := "ちびにゃん"
	re2, _ := regexp.Compile("(?i)" + kw2)
	if !re2.MatchString(title) {
		t.Errorf("expected keyword %q to match title %q (case insensitive)", kw2, title)
	}

	// Non-match case
	kw3 := "dog"
	re3, _ := regexp.Compile("(?i)" + kw3)
	if re3.MatchString(title) {
		t.Errorf("expected keyword %q NOT to match title %q", kw3, title)
	}

	// Exclude match case
	ex := "🐾今日の.*"
	reEx, _ := regexp.Compile("(?i)" + ex)
	if !reEx.MatchString(title) {
		t.Errorf("expected exclude pattern %q to match title %q", ex, title)
	}
}
