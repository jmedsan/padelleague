package search

import (
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/sahilm/fuzzy"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Scope controls entry visibility: public, admin-only, or per-competition.
type Scope struct {
	Public bool
	Admin  bool
	CompID string
}

// Entry is a single searchable item in the index.
type Entry struct {
	Label      string
	folded     string
	searchText string
	Secondary  string
	Type       string
	URL        string
	Keywords   []string
	Scope      Scope
}

// Index holds the search entries behind a read-write lock.
type Index struct {
	mu      sync.RWMutex
	entries []Entry
}

// Viewer represents the requesting user's access context.
type Viewer struct {
	IsAdmin bool
	CompIDs map[string]struct{}
}

// Result is a ranked search match returned to the caller.
type Result struct {
	Label     string
	Secondary string
	Type      string
	URL       string
	Score     float64
}

// NewEntry computes the folded label and searchable text for matching.
func NewEntry(e Entry) Entry {
	e.folded = fold(e.Label)
	parts := []string{e.folded}
	if e.Secondary != "" {
		parts = append(parts, fold(e.Secondary))
	}
	if e.Type != "" {
		parts = append(parts, fold(e.Type))
	}
	for _, kw := range e.Keywords {
		parts = append(parts, fold(kw))
	}
	e.searchText = strings.Join(parts, " ")
	return e
}

// Replace atomically swaps the index entries.
func (ix *Index) Replace(entries []Entry) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.entries = entries
}

// Search returns ranked results filtered by the viewer's access scope.
func (ix *Index) Search(query string, v Viewer, limit int) []Result {
	if query == "" {
		return nil
	}

	fq := fold(query)

	ix.mu.RLock()
	visible := make([]Entry, 0, len(ix.entries))
	for _, e := range ix.entries {
		if e.visibleTo(v) {
			visible = append(visible, e)
		}
	}
	ix.mu.RUnlock()

	hits := rankEntries(fq, visible)

	if len(hits) > limit {
		hits = hits[:limit]
	}

	results := make([]Result, len(hits))
	for i, h := range hits {
		results[i] = Result{
			Label:     h.entry.Label,
			Secondary: h.entry.Secondary,
			Type:      h.entry.Type,
			URL:       h.entry.URL,
			Score:     h.score,
		}
	}
	return results
}

type scored struct {
	entry Entry
	score float64
}

func rankEntries(fq string, visible []Entry) []scored {
	labelTexts := make([]string, len(visible))
	fullTexts := make([]string, len(visible))
	for i, e := range visible {
		labelTexts[i] = e.folded
		fullTexts[i] = e.searchText
	}

	labelHit := fuzzyScoreMap(fq, labelTexts, math.Inf(-1))
	fullHit := fuzzyScoreMap(fq, fullTexts, -0.2)

	var hits []scored
	for i, e := range visible {
		_, inLabel := labelHit[i]
		_, inFull := fullHit[i]
		if inLabel || inFull {
			best := math.Inf(-1)
			if inLabel {
				best = labelHit[i] * 2.0
			}
			if inFull && fullHit[i] > best {
				best = fullHit[i]
			}
			hits = append(hits, scored{e, best})
			continue
		}
		if s := wordSimilarity(fq, e.searchText); s >= 0.6 {
			hits = append(hits, scored{e, s})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].score > hits[j].score
	})
	return hits
}

func fuzzyScoreMap(query string, targets []string, minScore float64) map[int]float64 {
	matches := fuzzy.Find(query, targets)
	result := make(map[int]float64, len(matches))
	for _, m := range matches {
		s := float64(m.Score)
		if len(m.Str) > 0 {
			s /= float64(len(m.Str))
		}
		if s < minScore {
			continue
		}
		result[m.Index] = s
	}
	return result
}

func (e Entry) visibleTo(v Viewer) bool {
	if e.Scope.Admin {
		return v.IsAdmin
	}
	if e.Scope.CompID != "" {
		if v.IsAdmin {
			return true
		}
		_, ok := v.CompIDs[e.Scope.CompID]
		return ok
	}
	return true
}

func fold(s string) string {
	s = strings.ToLower(s)
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, s)
	return result
}

func wordSimilarity(query, target string) float64 {
	qWords := strings.Fields(query)
	tWords := strings.Fields(target)
	if len(qWords) == 0 || len(tWords) == 0 {
		return 0
	}

	var total float64
	for _, qw := range qWords {
		best := 0.0
		for _, tw := range tWords {
			if s := levenshteinSimilarity(qw, tw); s > best {
				best = s
			}
		}
		total += best
	}
	return total / float64(len(qWords))
}

func levenshteinSimilarity(a, b string) float64 {
	ra := []rune(a)
	rb := []rune(b)
	la, lb := len(ra), len(rb)
	maxLen := la
	if lb > maxLen {
		maxLen = lb
	}
	if maxLen == 0 {
		return 1.0
	}
	d := levenshteinDistance(ra, rb)
	return 1.0 - float64(d)/float64(maxLen)
}

func levenshteinDistance(a, b []rune) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	return int(math.Min(float64(a), math.Min(float64(b), float64(c))))
}
