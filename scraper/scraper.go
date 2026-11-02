// Package scraper crawls a fixed list of web pages concurrently with the
// Colly framework and turns each page into plain text.
//
// Concurrency model: Colly runs in asynchronous mode, so every Visit call
// starts a goroutine. A LimitRule caps how many requests are in flight at
// once, which keeps the crawler fast without overloading the target site.
package scraper

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/gocolly/colly/v2"
)

// Page is one scraped web page; it becomes one line of the JSON Lines output.
type Page struct {
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	Text       string    `json:"text"`
	WordCount  int       `json:"word_count"`
	StatusCode int       `json:"status_code"`
	FetchedAt  time.Time `json:"fetched_at"`
}

// Failure records a URL that could not be scraped.
type Failure struct {
	URL string
	Err error
}

// Result is everything a crawl produced.
type Result struct {
	Pages    []Page        // successful pages, in the same order as the input URLs
	Failures []Failure     // pages that failed, in input order
	Elapsed  time.Duration // wall-clock time of the whole crawl
}

// Config controls crawler behaviour. The zero value is not valid; start from
// DefaultConfig.
type Config struct {
	Parallelism int           // maximum simultaneous requests
	Delay       time.Duration // pause between requests on the same host
	Timeout     time.Duration // per-request timeout
	UserAgent   string        // sent with every request
	Logger      *slog.Logger  // receives per-page progress; nil disables logging
}

// DefaultUserAgent identifies the crawler, as Wikipedia's robot policy asks.
const DefaultUserAgent = "WikiScraperGo/1.0 (educational web-scraping assignment; Go Colly)"

// DefaultConfig returns settings that are polite to Wikipedia yet fast.
func DefaultConfig() Config {
	return Config{
		Parallelism: 8,
		Delay:       0,
		Timeout:     30 * time.Second,
		UserAgent:   DefaultUserAgent,
	}
}

// Validate reports the first invalid setting, if any.
func (cfg Config) Validate() error {
	switch {
	case cfg.Parallelism < 1:
		return errors.New("parallelism must be at least 1")
	case cfg.Delay < 0:
		return errors.New("delay cannot be negative")
	case cfg.Timeout <= 0:
		return errors.New("timeout must be positive")
	case cfg.UserAgent == "":
		return errors.New("user agent cannot be empty")
	}
	return nil
}

// Scrape fetches every URL concurrently and extracts its title and text.
// A failed page does not stop the crawl; it is reported in Result.Failures.
func Scrape(urls []string, cfg Config) (Result, error) {
	if err := cfg.Validate(); err != nil {
		return Result{}, err
	}
	if len(urls) == 0 {
		return Result{}, errors.New("no URLs to scrape")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	collector, err := newCollector(urls, cfg)
	if err != nil {
		return Result{}, err
	}

	// Callbacks run on many goroutines at once, so shared maps need a mutex.
	var mu sync.Mutex
	pagesByURL := make(map[string]Page, len(urls))
	errorsByURL := make(map[string]error)

	collector.OnHTML("html", func(element *colly.HTMLElement) {
		pageURL := element.Request.Ctx.Get("seedURL")
		document := element.DOM
		text := ExtractText(document)
		page := Page{
			URL:        pageURL,
			Title:      ExtractTitle(document),
			Text:       text,
			WordCount:  CountWords(text),
			StatusCode: element.Response.StatusCode,
			FetchedAt:  time.Now().UTC(),
		}
		mu.Lock()
		pagesByURL[pageURL] = page
		mu.Unlock()
		logger.Info("scraped page", "url", pageURL, "title", page.Title, "words", page.WordCount)
	})

	collector.OnError(func(response *colly.Response, requestErr error) {
		pageURL := response.Request.Ctx.Get("seedURL")
		if response.StatusCode != 0 {
			requestErr = fmt.Errorf("HTTP %d: %w", response.StatusCode, requestErr)
		}
		mu.Lock()
		errorsByURL[pageURL] = requestErr
		mu.Unlock()
		logger.Warn("page failed", "url", pageURL, "error", requestErr)
	})

	start := time.Now()
	for _, pageURL := range urls {
		// The seed URL travels in the request context so results can be
		// matched to inputs even when Wikipedia redirects the request.
		ctx := colly.NewContext()
		ctx.Put("seedURL", pageURL)
		if visitErr := collector.Request("GET", pageURL, nil, ctx, nil); visitErr != nil {
			mu.Lock()
			errorsByURL[pageURL] = visitErr
			mu.Unlock()
		}
	}
	collector.Wait() // block until every asynchronous request has finished

	return collectResults(urls, pagesByURL, errorsByURL, time.Since(start)), nil
}

// newCollector builds an asynchronous Colly collector restricted to the
// hosts of the input URLs, with the configured rate limits.
func newCollector(urls []string, cfg Config) (*colly.Collector, error) {
	hosts, err := hostsOf(urls)
	if err != nil {
		return nil, err
	}
	collector := colly.NewCollector(
		colly.Async(true),
		colly.AllowedDomains(hosts...),
		colly.UserAgent(cfg.UserAgent),
		colly.MaxDepth(1), // fetch only the listed pages; do not follow links
	)
	collector.SetRequestTimeout(cfg.Timeout)
	limitErr := collector.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: cfg.Parallelism,
		Delay:       cfg.Delay,
	})
	if limitErr != nil {
		return nil, fmt.Errorf("configuring rate limit: %w", limitErr)
	}
	return collector, nil
}

// collectResults orders pages and failures to match the input list, so output
// is deterministic even though pages finish in random order.
func collectResults(urls []string, pagesByURL map[string]Page, errorsByURL map[string]error, elapsed time.Duration) Result {
	result := Result{Elapsed: elapsed}
	for _, pageURL := range urls {
		if page, ok := pagesByURL[pageURL]; ok {
			result.Pages = append(result.Pages, page)
			continue
		}
		failure := errorsByURL[pageURL]
		if failure == nil {
			failure = errors.New("no HTML content received")
		}
		result.Failures = append(result.Failures, Failure{URL: pageURL, Err: failure})
	}
	return result
}

// hostsOf returns the distinct host names of urls.
func hostsOf(urls []string) ([]string, error) {
	seen := make(map[string]bool)
	var hosts []string
	for _, raw := range urls {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			return nil, fmt.Errorf("invalid URL %q", raw)
		}
		// Colly compares against the host name without a port.
		host := parsed.Hostname()
		if !seen[host] {
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	return hosts, nil
}
