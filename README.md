# WikiScraper: a concurrent Wikipedia crawler/scraper in Go

WikiScraper replaces the firm's Python/Scrapy spider (`articles-spider.py`) with a Go program built on the [Colly](https://github.com/gocolly/colly) framework. It downloads a list of Wikipedia pages about intelligent systems and robotics **concurrently**, strips the HTML markup, and writes the article text to a **JSON Lines** file, one JSON object per page, ready to load into the firm's knowledge base.

```
wikiscraper  ──►  10 Wikipedia pages fetched in parallel (goroutines via Colly)
             ──►  HTML → plain text (headings, paragraphs, list items)
             ──►  wikipedia_pages.jl   (one JSON object per line)
```

## Repository layout

| Path | Role |
|---|---|
| `cmd/wikiscraper/main.go` | Command-line application: flags, input validation, logging, profiling, output |
| `scraper/scraper.go` | Concurrent crawler built on Colly (`Scrape`, `Config`, `Page`) |
| `scraper/extract.go` | Turns a Wikipedia HTML page into clean text (removes citations, infoboxes, navboxes, tables, references…) |
| `scraper/urls.go` | Default URL list (same as the Scrapy spider), URL-file parsing and validation |
| `jsonlines/jsonlines.go` | Generic, reusable JSON Lines reader/writer (`Write[T]`, `Read[T]`) |
| `*_test.go`, `scraper/testdata/` | Unit tests, a synthetic Wikipedia page, and benchmarks |
| `urls.txt` | Example URL file (the 10 target pages) |
| `wikipedia_pages.jl` | **Deliverable:** text extracted from the crawl |
| `scripts/build.sh` | Builds the Windows `.exe`, macOS `.app`, and Linux binaries into `dist/` |
| `dist/` | Prebuilt executables |
| `docs/GENAI_LOG.md` | Record of how generative AI was used |

The `jsonlines` package and the `scraper` package have no dependency on the command-line program, so either can be imported by other projects.

## Quick start (no Go required)

**Windows:** download `dist/windows/wikiscraper.exe` and double-click it. A console window opens, the crawl runs, and `wikipedia_pages.jl` appears next to the `.exe`. The window waits for Enter before closing. You can also run it from PowerShell: `.\wikiscraper.exe -h`.

**macOS:** download `dist/macos/WikiScraper.app` and double-click it. It opens Terminal, runs the crawl, and opens `~/Documents/WikiScraper`, which holds `wikipedia_pages.jl` and `wikiscraper.log`. The app is not signed, so the first time right-click it and choose **Open**, or run `xattr -dr com.apple.quarantine WikiScraper.app`. The command-line binaries are inside the bundle at `WikiScraper.app/Contents/MacOS/wikiscraper-arm64` (Apple silicon) and `wikiscraper-x86_64` (Intel).

## Building from source

Requires Go 1.24 or newer.

```bash
git clone <this repository>.git
cd wikiscraper
go mod tidy                 # downloads Colly and its dependencies, completes go.sum
go run ./cmd/wikiscraper    # crawl the 10 default pages
```

To build executables: `go build -o wikiscraper ./cmd/wikiscraper` (add `.exe` on Windows), or `./scripts/build.sh` to produce every platform's build in `dist/`.

## Usage

```
wikiscraper [flags]

  -urls file        text file with one Wikipedia URL per line (default: the built-in list of 10 pages)
  -out file         JSON Lines output file (default "wikipedia_pages.jl")
  -parallel n       maximum simultaneous requests, 1–64 (default 8; 1 = sequential)
  -delay d          pause between requests to the same host, e.g. 250ms (default 0)
  -timeout d        per-request timeout (default 30s)
  -log file         also write the log to this file
  -cpuprofile file  write a CPU profile
  -memprofile file  write a heap profile
  -ua string        User-Agent header sent to Wikipedia
```

Examples:

```bash
wikiscraper                                   # default 10 pages → wikipedia_pages.jl
wikiscraper -urls urls.txt -out robotics.jl   # your own list
wikiscraper -parallel 1                       # sequential, for timing comparisons
wikiscraper -log run.log -cpuprofile cpu.prof -memprofile mem.prof
```

**Input checking.** Bad input is rejected before any request is made, with a message saying what to fix: unknown flags, stray arguments, a parallelism outside 1–64, negative delays, unparseable durations, a missing URL file, and URLs that are not `https://en.wikipedia.org/wiki/<Title>` (all bad URLs are listed at once). Duplicate URLs are removed. In URL files, blank lines and `#` comments are ignored.

**Exit codes.** `0` success · `1` some pages failed (the rest were written) · `2` invalid input · `3` nothing could be scraped or written.

## Output format

`wikipedia_pages.jl` follows the [JSON Lines](https://jsonlines.org/) format: each line is one complete JSON object, separated by line feeds. Pages appear in the same order as the input list, regardless of which download finished first.

```json
{"url":"https://en.wikipedia.org/wiki/Robot","title":"Robot","text":"A robot is a machine…\nTypes of robots\n…","word_count":7412,"status_code":200,"fetched_at":"2026-11-03T15:04:05Z"}
```

| Field | Meaning |
|---|---|
| `url` | The page requested |
| `title` | Article title from the page heading |
| `text` | Article text with all HTML removed; one heading, paragraph, or list item per `\n`-separated line |
| `word_count` | Number of words in `text` |
| `status_code` | HTTP status of the response |
| `fetched_at` | UTC time the page was scraped |

What the extractor keeps and drops: it reads only the article body (`#mw-content-text .mw-parser-output`), keeps headings, paragraphs and list items, and removes scripts, styles, citation markers such as `[12]` and `[citation needed]`, edit links, infoboxes, tables, navigation boxes, figures and captions, hatnotes, and the table of contents. It stops at the *See also*, *References*, *Notes*, *External links* and *Further reading* sections, as those are bibliography rather than prose. Images are ignored, as the assignment allows.

## How the concurrency works

Colly runs in asynchronous mode (`colly.Async(true)`): each `Request` call returns immediately and the download runs on its own goroutine. A `LimitRule` caps the number of requests in flight (`-parallel`, default 8), so the crawler is fast without hammering Wikipedia. Callbacks run on many goroutines at once, so results are stored in a mutex-protected map and re-ordered to match the input list after `collector.Wait()` returns. A failed page is logged and reported but does not stop the crawl. The crawler sends a descriptive User-Agent, as Wikipedia's bot policy asks, and does not follow links (`MaxDepth(1)`), since the assignment is a fixed list of pages.

## Testing

```bash
go test ./...                 # all unit tests
go test -race -cover ./...    # with the race detector and coverage
go test -run=^$ -bench=. -benchtime=5x ./scraper   # benchmarks
```

The tests never touch the real Wikipedia, so they run offline and give the same result every time:

- **Text extraction** (`scraper/extract_test.go`) runs against `scraper/testdata/sample_article.html`, a synthetic page with the same structure as a Wikipedia article (navigation, infobox, citations, edit links, nested lists, navbox, references). Tests check that the prose is kept exactly and that every kind of noise is removed.
- **Crawling** (`scraper/scraper_test.go`) starts a local mock Wikipedia server (`httptest`) that generates synthetic articles, adds network latency, and serves a 404 for one page. Tests check output order, word counts, that one failure does not stop the crawl, and that the server never sees more simultaneous requests than `Parallelism` allows.
- **URL handling, JSON Lines, and the command line** are covered with table-driven tests, including CRLF line endings, blank lines, malformed JSON, and every invalid flag.

Coverage: `scraper` 96%, `jsonlines` 91%, `cmd/wikiscraper` 40% (the untested part of `main` is the live crawl, which needs the network). All tests pass with the race detector on.

## Performance

### Concurrency benchmark (measured)

`BenchmarkScrape` crawls 20 synthetic pages from the local mock server, each taking 50 ms to serve (roughly a Wikipedia round trip), at different parallelism settings. Measured on a 2-CPU Linux machine (Intel Xeon 2.8 GHz), Go 1.24:

| Parallelism | Time for 20 pages | Speed-up | Memory allocated |
|---|---|---|---|
| 1 (sequential) | 1.021 s | 1.0× | 0.65 MB |
| 4 | 0.257 s | 4.0× | 0.67 MB |
| 8 (default) | 0.155 s | 6.6× | 0.72 MB |
| 16 | 0.104 s | 9.8× | 0.82 MB |

Parsing and extracting one page (`BenchmarkExtractText`) takes about 0.2 ms and 48 KB. The work is almost entirely waiting on the network, so the speed-up comes from overlapping requests, and it scales well beyond the number of CPU cores (2 here). Memory grows only slightly with parallelism.

### Go vs. Python/Scrapy on the real pages

Fill this table by running both programs against Wikipedia on the same machine and network, three runs each:

```bash
# Go
time ./wikiscraper -parallel 1     # sequential
time ./wikiscraper                 # concurrent (default 8)

# Python/Scrapy, from the WebFocusedCrawl directory
time scrapy crawl articles-spider -o items.jl
```

| Program | Run 1 | Run 2 | Run 3 | Mean |
|---|---|---|---|---|
| Python/Scrapy | | | | |
| Go, `-parallel 1` | | | | |
| Go, `-parallel 8` | | | | |

Observations (update these with your numbers):

- *Go is expected to beat a sequential crawl by roughly the parallelism factor, as the benchmark above shows.*
- *Scrapy is also asynchronous (Twisted, 16 concurrent requests by default), so for only 10 pages the gap with Scrapy may be smaller than expected; much of Scrapy's time is interpreter and framework start-up. Go's advantage grows with hundreds or thousands of pages, where its lower per-page CPU cost and lighter goroutines matter.*
- *For a 10-page crawl, network latency and Wikipedia's response time dominate; CPU cores matter less than the number of requests in flight.*

### Logs and profiles

Every run logs one line per page (title and word count, or the error) and, at the end, the total time and memory use (heap, total allocated, GC cycles, goroutines). `-log file` saves the log. `-cpuprofile` and `-memprofile` write profiles to inspect with `go tool pprof -top cpu.prof` or `go tool pprof -http=:8080 mem.prof`.

## GenAI Tools

I used **Claude** (Anthropic, Claude Opus 5.5, in the claude.ai app) as a coding assistant for this assignment. I gave it the assignment description and grading guidelines; it proposed the package structure and wrote the first draft of the code, unit tests, synthetic test page, build script and this README. It ran the tests and benchmarks in its own sandbox, using a local mock Wikipedia server because its sandbox could not reach Wikipedia.

I then *[describe your own part: reviewing the code, running the real crawl, filling in the timing table, changes you made]*.

The conversation log is in [`docs/GENAI_LOG.md`](docs/GENAI_LOG.md), with a link to the full conversation.

## References

- Colly documentation and examples: <https://github.com/gocolly/colly>, <https://go-colly.org/docs/>
- JSON Lines format: <https://jsonlines.org/>
- McConnell, S. (2004). *Code Complete* (2nd ed.). Microsoft Press.
- Mitchell, R. (2018). *Web Scraping with Python* (2nd ed.). O'Reilly.
- Wikipedia User-Agent policy: <https://meta.wikimedia.org/wiki/User-Agent_policy>
