package scraper

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// contentSelector is the container that holds an article's body on
// Wikipedia (MediaWiki) pages. Everything outside it is navigation chrome.
const contentSelector = "#mw-content-text .mw-parser-output"

// noiseSelectors lists elements inside the article body that are not prose:
// citation markers, edit links, infoboxes, navigation boxes, tables, figures,
// reference lists, hidden metadata and embedded styles or scripts.
var noiseSelectors = strings.Join([]string{
	"script", "style", "noscript",
	"sup.reference", ".mw-editsection",
	"table", ".infobox", ".navbox", ".vertical-navbox", ".sidebar",
	".thumb", "figure", ".gallery",
	".reflist", ".references", ".refbegin",
	".hatnote", ".shortdescription", ".mw-empty-elt",
	".metadata", ".ambox", ".noprint", "#toc", ".toc",
}, ", ")

// textBlockSelectors are the elements whose text forms the readable article,
// visited in document order.
const textBlockSelectors = "h2, h3, h4, p, li, dd"

// stopSections end extraction: what follows them is bibliography, not prose.
var stopSections = map[string]bool{
	"references":      true,
	"see also":        true,
	"external links":  true,
	"further reading": true,
	"notes":           true,
	"bibliography":    true,
	"sources":         true,
	"citations":       true,
}

var (
	// bracketedNoteRE matches leftover markers such as [12], [a] or [citation needed].
	bracketedNoteRE = regexp.MustCompile(`\[(?:\d+|[a-z]|citation needed|clarification needed|edit)\]`)
	whitespaceRE    = regexp.MustCompile(`\s+`)
)

// ExtractTitle returns the article title, preferring the visible heading
// over the <title> tag (which carries a " - Wikipedia" suffix).
func ExtractTitle(doc *goquery.Selection) string {
	if heading := CleanText(doc.Find("#firstHeading").First().Text()); heading != "" {
		return heading
	}
	title := CleanText(doc.Find("title").First().Text())
	return strings.TrimSuffix(title, " - Wikipedia")
}

// ExtractText returns the article's readable text with all HTML markup removed.
// Headings, paragraphs and list items each become one line, so the result
// keeps the article's structure while being plain text.
func ExtractText(doc *goquery.Selection) string {
	content := doc.Find(contentSelector).First()
	if content.Length() == 0 {
		// Not a MediaWiki page: fall back to the whole <body>.
		content = doc.Find("body").First()
	}
	// Work on a copy so callers keep the original document intact.
	content = content.Clone()
	content.Find(noiseSelectors).Remove()

	var lines []string
	content.Find(textBlockSelectors).EachWithBreak(func(_ int, block *goquery.Selection) bool {
		if isHeading(block) && stopSections[strings.ToLower(CleanText(block.Text()))] {
			return false // stop: the rest of the page is reference material
		}
		if hasTextBlockAncestor(block) {
			return true // already captured as part of its parent block
		}
		if text := CleanText(block.Text()); text != "" {
			lines = append(lines, text)
		}
		return true
	})
	return strings.Join(lines, "\n")
}

// CleanText removes citation markers and collapses runs of whitespace.
func CleanText(raw string) string {
	text := bracketedNoteRE.ReplaceAllString(raw, "")
	text = whitespaceRE.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// CountWords counts whitespace-separated words.
func CountWords(text string) int {
	return len(strings.Fields(text))
}

func isHeading(block *goquery.Selection) bool {
	switch goquery.NodeName(block) {
	case "h2", "h3", "h4":
		return true
	default:
		return false
	}
}

// hasTextBlockAncestor reports whether block sits inside another list item or
// definition; the outer element's text already includes it, so including it
// again would duplicate content.
func hasTextBlockAncestor(block *goquery.Selection) bool {
	return block.ParentsFiltered("li, dd").Length() > 0
}
