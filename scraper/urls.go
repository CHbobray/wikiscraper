package scraper

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// DefaultURLs is the firm's test list, the same pages the Python/Scrapy
// spider (articles-spider.py) crawls.
var DefaultURLs = []string{
	"https://en.wikipedia.org/wiki/Robotics",
	"https://en.wikipedia.org/wiki/Robot",
	"https://en.wikipedia.org/wiki/Reinforcement_learning",
	"https://en.wikipedia.org/wiki/Robot_Operating_System",
	"https://en.wikipedia.org/wiki/Intelligent_agent",
	"https://en.wikipedia.org/wiki/Software_agent",
	"https://en.wikipedia.org/wiki/Robotic_process_automation",
	"https://en.wikipedia.org/wiki/Chatbot",
	"https://en.wikipedia.org/wiki/Applications_of_artificial_intelligence",
	"https://en.wikipedia.org/wiki/Android_(robot)",
}

// ParseURLList reads one URL per line. Blank lines and lines starting with
// '#' are ignored, so URL files can carry comments.
func ParseURLList(r io.Reader) ([]string, error) {
	var urls []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
	}
	return urls, scanner.Err()
}

// ValidateWikipediaURLs checks that every entry is an English Wikipedia
// article URL and removes duplicates while keeping the original order.
// All problems are reported together so the user can fix them in one pass.
func ValidateWikipediaURLs(urls []string) ([]string, error) {
	var problems []string
	seen := make(map[string]bool, len(urls))
	valid := make([]string, 0, len(urls))
	for _, raw := range urls {
		if err := checkWikipediaURL(raw); err != nil {
			problems = append(problems, fmt.Sprintf("  %q: %v", raw, err))
			continue
		}
		if seen[raw] {
			continue
		}
		seen[raw] = true
		valid = append(valid, raw)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%d invalid URL(s):\n%s", len(problems), strings.Join(problems, "\n"))
	}
	if len(valid) == 0 {
		return nil, errors.New("the URL list is empty")
	}
	return valid, nil
}

func checkWikipediaURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("not a valid URL")
	}
	switch {
	case parsed.Scheme != "https":
		return errors.New("must start with https://")
	case parsed.Host != "en.wikipedia.org":
		return errors.New("host must be en.wikipedia.org")
	case !strings.HasPrefix(parsed.Path, "/wiki/") || len(parsed.Path) == len("/wiki/"):
		return errors.New("path must be /wiki/<Article_title>")
	}
	return nil
}
