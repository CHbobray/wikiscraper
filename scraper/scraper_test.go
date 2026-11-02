package scraper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// syntheticArticle builds a small Wikipedia-shaped page with a known number
// of paragraphs, so tests can predict the scraped word count.
func syntheticArticle(title string, paragraphs int) string {
	var body strings.Builder
	for i := range paragraphs {
		fmt.Fprintf(&body, "<p>Paragraph %d about %s.<sup class=\"reference\">[%d]</sup></p>\n", i+1, title, i+1)
	}
	return fmt.Sprintf(`<html><head><title>%[1]s - Wikipedia</title></head><body>
<h1 id="firstHeading">%[1]s</h1>
<div id="mw-content-text"><div class="mw-parser-output">%[2]s</div></div>
</body></html>`, title, body.String())
}

// mockWikipedia serves synthetic articles at /wiki/<title>, waiting latency
// before each response to imitate a real network round trip. It records the
// highest number of requests it saw in flight at the same time.
type mockWikipedia struct {
	server      *httptest.Server
	inFlight    atomic.Int32
	maxInFlight atomic.Int32
	requests    atomic.Int32
}

func newMockWikipedia(t testing.TB, latency time.Duration) *mockWikipedia {
	mock := &mockWikipedia{}
	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.requests.Add(1)
		current := mock.inFlight.Add(1)
		defer mock.inFlight.Add(-1)
		for {
			highest := mock.maxInFlight.Load()
			if current <= highest || mock.maxInFlight.CompareAndSwap(highest, current) {
				break
			}
		}
		time.Sleep(latency)

		title, ok := strings.CutPrefix(r.URL.Path, "/wiki/")
		if !ok || title == "Missing_page" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, syntheticArticle(title, 3))
	}))
	t.Cleanup(mock.server.Close)
	return mock
}

func (m *mockWikipedia) urls(titles ...string) []string {
	urls := make([]string, len(titles))
	for i, title := range titles {
		urls[i] = m.server.URL + "/wiki/" + title
	}
	return urls
}

func pageTitles(count int) []string {
	titles := make([]string, count)
	for i := range titles {
		titles[i] = fmt.Sprintf("Article_%02d", i)
	}
	return titles
}

func TestScrapeReturnsPagesInInputOrder(t *testing.T) {
	mock := newMockWikipedia(t, 5*time.Millisecond)
	titles := pageTitles(12)
	result, err := Scrape(mock.urls(titles...), DefaultConfig())
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("unexpected failures: %v", result.Failures)
	}
	if len(result.Pages) != len(titles) {
		t.Fatalf("got %d pages, want %d", len(result.Pages), len(titles))
	}
	for i, page := range result.Pages {
		if page.Title != titles[i] {
			t.Errorf("page %d title = %q, want %q", i, page.Title, titles[i])
		}
		if page.StatusCode != http.StatusOK {
			t.Errorf("page %d status = %d, want 200", i, page.StatusCode)
		}
		// 3 paragraphs × "Paragraph N about Title." (4 words each).
		if page.WordCount != 12 {
			t.Errorf("page %d word count = %d, want 12 (text %q)", i, page.WordCount, page.Text)
		}
		if strings.Contains(page.Text, "[") {
			t.Errorf("page %d text still has citation markers: %q", i, page.Text)
		}
	}
}

func TestScrapeReportsFailuresWithoutStopping(t *testing.T) {
	mock := newMockWikipedia(t, 0)
	urls := mock.urls("Robot", "Missing_page", "Chatbot")
	result, err := Scrape(urls, DefaultConfig())
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}
	if len(result.Pages) != 2 {
		t.Errorf("got %d pages, want 2", len(result.Pages))
	}
	if len(result.Failures) != 1 || result.Failures[0].URL != urls[1] {
		t.Fatalf("failures = %v, want one failure for %s", result.Failures, urls[1])
	}
	if !strings.Contains(result.Failures[0].Err.Error(), "404") {
		t.Errorf("failure should mention HTTP 404, got %v", result.Failures[0].Err)
	}
}

func TestScrapeRespectsParallelismLimit(t *testing.T) {
	mock := newMockWikipedia(t, 20*time.Millisecond)
	cfg := DefaultConfig()
	cfg.Parallelism = 3
	if _, err := Scrape(mock.urls(pageTitles(15)...), cfg); err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}
	if peak := mock.maxInFlight.Load(); peak > 3 {
		t.Errorf("peak concurrent requests = %d, want at most 3", peak)
	}
	if peak := mock.maxInFlight.Load(); peak < 2 {
		t.Errorf("peak concurrent requests = %d; requests did not run concurrently", peak)
	}
}

func TestScrapeRejectsBadInput(t *testing.T) {
	if _, err := Scrape(nil, DefaultConfig()); err == nil {
		t.Error("Scrape(nil) should fail")
	}
	if _, err := Scrape([]string{"not a url"}, DefaultConfig()); err == nil {
		t.Error("Scrape with an invalid URL should fail")
	}
	cfg := DefaultConfig()
	cfg.Parallelism = 0
	if _, err := Scrape([]string{"https://en.wikipedia.org/wiki/Robot"}, cfg); err == nil {
		t.Error("Scrape with parallelism 0 should fail")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Config)
		valid  bool
	}{
		{"defaults", func(*Config) {}, true},
		{"zero parallelism", func(c *Config) { c.Parallelism = 0 }, false},
		{"negative delay", func(c *Config) { c.Delay = -time.Second }, false},
		{"zero timeout", func(c *Config) { c.Timeout = 0 }, false},
		{"empty user agent", func(c *Config) { c.UserAgent = "" }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.modify(&cfg)
			if err := cfg.Validate(); (err == nil) != tc.valid {
				t.Errorf("Validate() error = %v, want valid=%v", err, tc.valid)
			}
		})
	}
}

// BenchmarkScrape compares a sequential crawl (parallelism 1, like a
// one-request-at-a-time crawler) with concurrent crawls. Each simulated page
// takes 50 ms to serve, roughly a real Wikipedia round trip.
func BenchmarkScrape(b *testing.B) {
	mock := newMockWikipedia(b, 50*time.Millisecond)
	urls := mock.urls(pageTitles(20)...)
	for _, parallelism := range []int{1, 4, 8, 16} {
		b.Run(fmt.Sprintf("parallel=%d", parallelism), func(b *testing.B) {
			cfg := DefaultConfig()
			cfg.Parallelism = parallelism
			b.ReportAllocs()
			for b.Loop() {
				result, err := Scrape(urls, cfg)
				if err != nil || len(result.Pages) != len(urls) {
					b.Fatalf("scrape failed: err=%v pages=%d", err, len(result.Pages))
				}
			}
		})
	}
}
