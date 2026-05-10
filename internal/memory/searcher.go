package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SearchResult represents a single match from session memory search.
type SearchResult struct {
	SessionID    string    `json:"session_id"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	CreatedAt    time.Time `json:"created_at"`
	Snippet      string    `json:"snippet"`
	Relevance    float64   `json:"relevance"`
	MessageCount int       `json:"message_count"`
}

// Searcher performs keyword-based searches across historical sessions.
type Searcher struct {
	sessionsDir string
}

// NewSearcher creates a Searcher that scans the given directory.
func NewSearcher(sessionsDir string) *Searcher {
	return &Searcher{sessionsDir: sessionsDir}
}

// Search scans all session JSON files for the given query and returns ranked results.
func (s *Searcher) Search(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	files, err := os.ReadDir(s.sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	query = strings.ToLower(query)
	queryTerms := strings.Fields(query)

	var results []SearchResult

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.sessionsDir, f.Name()))
		if err != nil {
			continue
		}

		var session struct {
			ID        string    `json:"id"`
			Provider  string    `json:"provider"`
			Model     string    `json:"model"`
			CreatedAt time.Time `json:"created_at"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}

		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}

		var content strings.Builder
		for _, msg := range session.Messages {
			content.WriteString(msg.Content)
			content.WriteByte(' ')
		}

		lower := strings.ToLower(content.String())
		matchCount := 0
		firstMatch := ""

		for _, term := range queryTerms {
			count := strings.Count(lower, term)
			if count > 0 {
				matchCount += count
				if firstMatch == "" {
					idx := strings.Index(lower, term)
					start := idx - 50
					if start < 0 {
						start = 0
					}
					end := idx + len(term) + 50
					if end > len(content.String()) {
						end = len(content.String())
					}
					snippet := content.String()[start:end]
					if start > 0 {
						snippet = "..." + snippet
					}
					if end < len(content.String()) {
						snippet = snippet + "..."
					}
					if len(snippet) > 200 {
						snippet = snippet[:200] + "..."
					}
					firstMatch = strings.TrimSpace(snippet)
				}
			}
		}

		if matchCount > 0 {
			msgCount := len(session.Messages)
			denom := float64(msgCount + 1)
			if denom < 1 {
				denom = 1
			}
			relevance := float64(matchCount) / denom
			if relevance > 1.0 {
				relevance = 1.0
			}

			results = append(results, SearchResult{
				SessionID:    session.ID,
				Provider:     session.Provider,
				Model:        session.Model,
				CreatedAt:    session.CreatedAt,
				Snippet:      firstMatch,
				Relevance:    relevance,
				MessageCount: msgCount,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// FormatResults formats search results for display.
func FormatResults(results []SearchResult) string {
	if len(results) == 0 {
		return "No matching sessions found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matching session(s):\n\n", len(results)))

	for i, r := range results {
		shortID := r.SessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s | %s | %s | %d msgs | %.0f%% match\n",
			i+1, shortID, r.CreatedAt.Format("2006-01-02 15:04"),
			r.Provider, r.Model, r.MessageCount, r.Relevance*100))
		sb.WriteString(fmt.Sprintf("   \"%s\"\n\n", r.Snippet))
	}

	return sb.String()
}
