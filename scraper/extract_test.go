package scraper

import (
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// loadFixture parses an HTML file from testdata into a goquery document.
func loadFixture(t *testing.T, name string) *goquery.Selection {
	t.Helper()
	file, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer file.Close()
	doc, err := goquery.NewDocumentFromReader(file)
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	return doc.Selection
}

func TestExtractTitle(t *testing.T) {
	doc := loadFixture(t, "sample_article.html")
	if got, want := ExtractTitle(doc), "Test Robot"; got != want {
		t.Errorf("ExtractTitle() = %q, want %q", got, want)
	}
}

func TestExtractTitleFallsBackToTitleTag(t *testing.T) {
	doc := parseHTML(t, `<html><head><title>Chatbot - Wikipedia</title></head><body></body></html>`)
	if got, want := ExtractTitle(doc), "Chatbot"; got != want {
		t.Errorf("ExtractTitle() = %q, want %q", got, want)
	}
}

func TestExtractTextKeepsArticleProse(t *testing.T) {
	text := ExtractText(loadFixture(t, "sample_article.html"))
	wantLines := []string{
		"A test robot is a machine that carries out tasks automatically. It is used in many industries.",
		"History",
		"Early robots appeared in the 1950s.",
		"Uses",
		"Manufacturing Welding",
		"Exploration",
	}
	if got, want := text, strings.Join(wantLines, "\n"); got != want {
		t.Errorf("ExtractText() mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestExtractTextRemovesMarkupAndNoise(t *testing.T) {
	text := ExtractText(loadFixture(t, "sample_article.html"))
	unwanted := []string{
		"<", ">", // no HTML tags
		"tracking", "color: black", // script and style bodies
		"Navigation menu", "Footer text", // page chrome outside the article
		"Infobox value", "Navbox links", "Figure caption", "Contents 1", // non-prose blocks
		"For other uses", "Machine used in tests", // hatnote and short description
		"[1]", "[2]", "[edit]", "citation needed", // citation and edit markers
		"Reference text", "Android (robot)", // sections after "See also"/"References"
	}
	for _, fragment := range unwanted {
		if strings.Contains(text, fragment) {
			t.Errorf("ExtractText() output contains %q:\n%s", fragment, text)
		}
	}
}

func TestExtractTextDoesNotModifyDocument(t *testing.T) {
	doc := loadFixture(t, "sample_article.html")
	ExtractText(doc)
	if doc.Find(".infobox").Length() == 0 {
		t.Error("ExtractText removed elements from the caller's document")
	}
}

func TestExtractTextFallsBackToBody(t *testing.T) {
	doc := parseHTML(t, `<html><body><p>Plain page.</p><script>x()</script></body></html>`)
	if got, want := ExtractText(doc), "Plain page."; got != want {
		t.Errorf("ExtractText() = %q, want %q", got, want)
	}
}

func TestCleanText(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{"collapses whitespace", "  a \n\t b  ", "a b"},
		{"numeric citation", "robots.[12] Next", "robots. Next"},
		{"letter footnote", "note[a] here", "note here"},
		{"citation needed", "claim[citation needed].", "claim."},
		{"keeps real brackets", "array [1, 2]", "array [1, 2]"},
		{"empty", "   ", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanText(tc.input); got != tc.want {
				t.Errorf("CleanText(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCountWords(t *testing.T) {
	if got := CountWords("one two\nthree  four"); got != 4 {
		t.Errorf("CountWords() = %d, want 4", got)
	}
	if got := CountWords(""); got != 0 {
		t.Errorf("CountWords(\"\") = %d, want 0", got)
	}
}

func parseHTML(t *testing.T, html string) *goquery.Selection {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parsing HTML: %v", err)
	}
	return doc.Selection
}

// BenchmarkExtractText measures parsing cost per page, independent of the network.
func BenchmarkExtractText(b *testing.B) {
	raw, err := os.ReadFile("testdata/sample_article.html")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(string(raw)))
		ExtractText(doc.Selection)
	}
}
