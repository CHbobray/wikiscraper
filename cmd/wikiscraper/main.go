// Command wikiscraper crawls a list of Wikipedia pages concurrently, strips
// the HTML markup, and writes one JSON object per page to a JSON Lines file.
//
// Usage:
//
//	wikiscraper [flags]
//
// Run "wikiscraper -h" for the list of flags.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"wikiscraper/jsonlines"
	"wikiscraper/scraper"
)

// Exit codes.
const (
	exitOK          = 0
	exitPartialFail = 1 // some pages could not be scraped
	exitUsage       = 2 // invalid flags or URL list
	exitFatal       = 3 // nothing could be written
)

// options holds the parsed command-line flags.
type options struct {
	urlFile     string
	outputPath  string
	logPath     string
	cpuProfile  string
	memProfile  string
	parallelism int
	delay       time.Duration
	timeout     time.Duration
	userAgent   string
}

func main() {
	exitCode := run(os.Args[1:], os.Stdout, os.Stderr)
	// When the .exe is double-clicked in Windows Explorer the console window
	// would close before the user can read the summary, so wait for Enter.
	if ownsConsoleWindow() {
		fmt.Print("\nPress Enter to close this window...")
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
	os.Exit(exitCode)
}

// run is main without the process exit, which keeps it testable.
func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\nRun with -h to see the available flags.\n", err)
		return exitUsage
	}

	logger, closeLog, err := newLogger(opts.logPath, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}
	defer closeLog()

	urls, err := loadURLs(opts.urlFile)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}

	stopProfile, err := startCPUProfile(opts.cpuProfile)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}

	cfg := scraper.Config{
		Parallelism: opts.parallelism,
		Delay:       opts.delay,
		Timeout:     opts.timeout,
		UserAgent:   opts.userAgent,
		Logger:      logger,
	}
	logger.Info("starting crawl", "pages", len(urls), "parallelism", cfg.Parallelism, "cpus", runtime.NumCPU())
	result, err := scraper.Scrape(urls, cfg)
	stopProfile()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitUsage
	}

	if len(result.Pages) == 0 {
		fmt.Fprintln(stderr, "error: no pages were scraped; nothing written")
		reportFailures(stderr, result.Failures)
		return exitFatal
	}
	if err := jsonlines.WriteFile(opts.outputPath, result.Pages); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitFatal
	}
	if err := writeMemProfile(opts.memProfile); err != nil {
		logger.Warn("could not write memory profile", "error", err)
	}

	printSummary(stdout, result, opts.outputPath)
	logMemoryUse(logger)
	if len(result.Failures) > 0 {
		reportFailures(stderr, result.Failures)
		return exitPartialFail
	}
	return exitOK
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	defaults := scraper.DefaultConfig()
	var opts options
	flags := flag.NewFlagSet("wikiscraper", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.urlFile, "urls", "", "text file with one Wikipedia URL per line (default: the built-in list of 10 pages)")
	flags.StringVar(&opts.outputPath, "out", "wikipedia_pages.jl", "JSON Lines output file")
	flags.StringVar(&opts.logPath, "log", "", "also write the log to this file")
	flags.StringVar(&opts.cpuProfile, "cpuprofile", "", "write a CPU profile to this file")
	flags.StringVar(&opts.memProfile, "memprofile", "", "write a heap profile to this file")
	flags.IntVar(&opts.parallelism, "parallel", defaults.Parallelism, "maximum simultaneous requests (1 = sequential, max 64)")
	flags.DurationVar(&opts.delay, "delay", defaults.Delay, "pause between requests to the same host, e.g. 250ms")
	flags.DurationVar(&opts.timeout, "timeout", defaults.Timeout, "per-request timeout, e.g. 30s")
	flags.StringVar(&opts.userAgent, "ua", defaults.UserAgent, "User-Agent header sent to Wikipedia")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "wikiscraper crawls Wikipedia pages concurrently and writes their text as JSON Lines.")
		fmt.Fprintln(stderr, "\nUsage: wikiscraper [flags]\n\nFlags:")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() > 0 {
		return opts, fmt.Errorf("unexpected argument %q (flags start with '-')", flags.Arg(0))
	}
	if opts.parallelism > 64 {
		return opts, errors.New("-parallel cannot exceed 64; be polite to Wikipedia")
	}
	if opts.outputPath == "" {
		return opts, errors.New("-out cannot be empty")
	}
	cfg := scraper.Config{Parallelism: opts.parallelism, Delay: opts.delay, Timeout: opts.timeout, UserAgent: opts.userAgent}
	return opts, cfg.Validate()
}

// loadURLs returns the built-in list or, when a file is given, its validated contents.
func loadURLs(path string) ([]string, error) {
	if path == "" {
		return scraper.DefaultURLs, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening URL file: %w", err)
	}
	defer file.Close()
	urls, err := scraper.ParseURLList(file)
	if err != nil {
		return nil, fmt.Errorf("reading URL file: %w", err)
	}
	return scraper.ValidateWikipediaURLs(urls)
}

// newLogger logs to stderr and, if logPath is set, to that file as well.
func newLogger(logPath string, stderr io.Writer) (*slog.Logger, func(), error) {
	if logPath == "" {
		return slog.New(slog.NewTextHandler(stderr, nil)), func() {}, nil
	}
	file, err := os.Create(logPath)
	if err != nil {
		return nil, nil, fmt.Errorf("creating log file: %w", err)
	}
	handler := slog.NewTextHandler(io.MultiWriter(stderr, file), nil)
	return slog.New(handler), func() { file.Close() }, nil
}

func startCPUProfile(path string) (stop func(), err error) {
	if path == "" {
		return func() {}, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("creating CPU profile: %w", err)
	}
	if err := pprof.StartCPUProfile(file); err != nil {
		file.Close()
		return nil, fmt.Errorf("starting CPU profile: %w", err)
	}
	return func() {
		pprof.StopCPUProfile()
		file.Close()
	}, nil
}

func writeMemProfile(path string) error {
	if path == "" {
		return nil
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	runtime.GC() // record up-to-date allocation statistics
	return pprof.WriteHeapProfile(file)
}

func printSummary(stdout io.Writer, result scraper.Result, outputPath string) {
	totalWords := 0
	for _, page := range result.Pages {
		totalWords += page.WordCount
	}
	fmt.Fprintf(stdout, "\nScraped %d page(s), %d word(s), in %s\n",
		len(result.Pages), totalWords, result.Elapsed.Round(time.Millisecond))
	fmt.Fprintf(stdout, "Output written to %s\n", outputPath)
}

func reportFailures(stderr io.Writer, failures []scraper.Failure) {
	if len(failures) == 0 {
		return
	}
	fmt.Fprintf(stderr, "%d page(s) failed:\n", len(failures))
	for _, failure := range failures {
		fmt.Fprintf(stderr, "  %s: %v\n", failure.URL, failure.Err)
	}
}

func logMemoryUse(logger *slog.Logger) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	logger.Info("memory use",
		"heap_alloc_mb", fmt.Sprintf("%.1f", float64(stats.HeapAlloc)/1e6),
		"total_alloc_mb", fmt.Sprintf("%.1f", float64(stats.TotalAlloc)/1e6),
		"sys_mb", fmt.Sprintf("%.1f", float64(stats.Sys)/1e6),
		"gc_cycles", stats.NumGC,
		"goroutines", runtime.NumGoroutine())
}
