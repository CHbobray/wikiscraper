package scraper

import (
	"strings"
	"testing"
)

func TestParseURLListSkipsCommentsAndBlankLines(t *testing.T) {
	input := "# robotics pages\nhttps://en.wikipedia.org/wiki/Robot\n\n   \n  https://en.wikipedia.org/wiki/Chatbot  \r\n"
	urls, err := ParseURLList(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseURLList() error: %v", err)
	}
	want := []string{"https://en.wikipedia.org/wiki/Robot", "https://en.wikipedia.org/wiki/Chatbot"}
	if strings.Join(urls, ",") != strings.Join(want, ",") {
		t.Errorf("ParseURLList() = %v, want %v", urls, want)
	}
}

func TestValidateWikipediaURLs(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		valid bool
	}{
		{"article", []string{"https://en.wikipedia.org/wiki/Robot"}, true},
		{"parentheses in title", []string{"https://en.wikipedia.org/wiki/Android_(robot)"}, true},
		{"http scheme", []string{"http://en.wikipedia.org/wiki/Robot"}, false},
		{"other language", []string{"https://de.wikipedia.org/wiki/Roboter"}, false},
		{"other site", []string{"https://example.com/wiki/Robot"}, false},
		{"no article", []string{"https://en.wikipedia.org/wiki/"}, false},
		{"not a wiki path", []string{"https://en.wikipedia.org/w/index.php"}, false},
		{"garbage", []string{"::not a url::"}, false},
		{"empty list", []string{}, false},
		{"one bad among good", []string{"https://en.wikipedia.org/wiki/Robot", "ftp://x"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateWikipediaURLs(tc.input)
			if (err == nil) != tc.valid {
				t.Errorf("ValidateWikipediaURLs(%v) error = %v, want valid=%v", tc.input, err, tc.valid)
			}
		})
	}
}

func TestValidateWikipediaURLsRemovesDuplicates(t *testing.T) {
	robot := "https://en.wikipedia.org/wiki/Robot"
	chatbot := "https://en.wikipedia.org/wiki/Chatbot"
	got, err := ValidateWikipediaURLs([]string{robot, chatbot, robot})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != robot || got[1] != chatbot {
		t.Errorf("got %v, want [%s %s]", got, robot, chatbot)
	}
}

func TestDefaultURLsAreValid(t *testing.T) {
	valid, err := ValidateWikipediaURLs(DefaultURLs)
	if err != nil {
		t.Fatal(err)
	}
	if len(valid) != 10 {
		t.Errorf("default list has %d unique URLs, want 10", len(valid))
	}
}
