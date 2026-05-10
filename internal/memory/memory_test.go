package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helper to write a session JSON file for testing
func writeSessionFile(t *testing.T, dir, filename string, session interface{}) {
	t.Helper()
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestNewSearcher(t *testing.T) {
	t.Parallel()

	s := NewSearcher("/tmp/test")
	if s == nil {
		t.Fatal("expected searcher, got nil")
	}
	if s.sessionsDir != "/tmp/test" {
		t.Errorf("expected sessionsDir, got %q", s.sessionsDir)
	}
}

func TestSearchNoSessionFiles(t *testing.T) {
	dir := t.TempDir()
	s := NewSearcher(dir)

	results, err := s.Search("test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchNonexistentDir(t *testing.T) {
	s := NewSearcher("/nonexistent/path/xyz")
	_, err := s.Search("test", 5)
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestSearchMatchingContent(t *testing.T) {
	dir := t.TempDir()
	s := NewSearcher(dir)

	session1 := struct {
		ID        string    `json:"id"`
		Provider  string    `json:"provider"`
		Model     string    `json:"model"`
		CreatedAt time.Time `json:"created_at"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		ID:        "session-001",
		Provider:  "openai",
		Model:     "gpt-4o",
		CreatedAt: time.Now(),
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "user", Content: "Hello, how do I use Go interfaces?"},
			{Role: "assistant", Content: "Go interfaces are implemented implicitly..."},
		},
	}

	session2 := struct {
		ID        string    `json:"id"`
		Provider  string    `json:"provider"`
		Model     string    `json:"model"`
		CreatedAt time.Time `json:"created_at"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		ID:        "session-002",
		Provider:  "anthropic",
		Model:     "claude-sonnet-4",
		CreatedAt: time.Now(),
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "user", Content: "Write a Python script to parse JSON"},
			{Role: "assistant", Content: "Here's a Python script using the json module..."},
		},
	}

	writeSessionFile(t, dir, "session-001.json", session1)
	writeSessionFile(t, dir, "session-002.json", session2)

	t.Run("search for Go", func(t *testing.T) {
		results, err := s.Search("Go", 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].SessionID != "session-001" {
			t.Errorf("expected session-001, got %q", results[0].SessionID)
		}
		if results[0].Relevance <= 0 {
			t.Error("expected positive relevance")
		}
	})

	t.Run("search for Python", func(t *testing.T) {
		results, err := s.Search("Python", 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].SessionID != "session-002" {
			t.Errorf("expected session-002, got %q", results[0].SessionID)
		}
	})

	t.Run("search with default limit", func(t *testing.T) {
		results, err := s.Search("Go interfaces json", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) > 5 {
			t.Errorf("expected at most 5 results, got %d", len(results))
		}
	})

	t.Run("search with limit", func(t *testing.T) {
		results, err := s.Search("json", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) > 1 {
			t.Errorf("expected at most 1 result, got %d", len(results))
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		results, err := s.Search("go", 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) < 1 {
			t.Error("expected results for case-insensitive search")
		}
	})
}

func TestSearchNoMatchingContent(t *testing.T) {
	dir := t.TempDir()
	s := NewSearcher(dir)

	session := struct {
		ID        string    `json:"id"`
		Provider  string    `json:"provider"`
		Model     string    `json:"model"`
		CreatedAt time.Time `json:"created_at"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		ID:        "session-001",
		Provider:  "openai",
		Model:     "gpt-4o",
		CreatedAt: time.Now(),
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "user", Content: "Hello world"},
		},
	}

	writeSessionFile(t, dir, "session-001.json", session)

	results, err := s.Search("zzzzz_nonexistent_query", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchSkipsNonJSON(t *testing.T) {
	dir := t.TempDir()
	s := NewSearcher(dir)

	// Write a non-JSON file
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("Go interfaces"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	results, err := s.Search("Go", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-JSON files, got %d", len(results))
	}
}

func TestSearchSkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	s := NewSearcher(dir)

	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	results, err := s.Search("test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchSkipsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	s := NewSearcher(dir)

	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not json"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	results, err := s.Search("test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for invalid JSON, got %d", len(results))
	}
}

func TestFormatResults(t *testing.T) {
	t.Parallel()

	t.Run("empty results", func(t *testing.T) {
		output := FormatResults(nil)
		if !strings.Contains(output, "No matching sessions found") {
			t.Errorf("expected 'No matching', got %q", output)
		}
	})

	t.Run("with results", func(t *testing.T) {
		results := []SearchResult{
			{
				SessionID:    "abcdef1234567890",
				Provider:     "openai",
				Model:        "gpt-4o",
				CreatedAt:    time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC),
				Snippet:      "here is a snippet of matched text",
				Relevance:    0.85,
				MessageCount: 5,
			},
		}
		output := FormatResults(results)
		if !strings.Contains(output, "Found 1 matching") {
			t.Errorf("expected 'Found 1 matching', got %q", output)
		}
		if !strings.Contains(output, "abcdef12") {
			t.Errorf("expected truncated session ID, got %q", output)
		}
		if !strings.Contains(output, "openai") {
			t.Errorf("expected provider, got %q", output)
		}
		if !strings.Contains(output, "85%") {
			t.Errorf("expected 85%% relevance, got %q", output)
		}
		if !strings.Contains(output, "snippet of matched text") {
			t.Errorf("expected snippet, got %q", output)
		}
	})

	t.Run("short session ID", func(t *testing.T) {
		results := []SearchResult{
			{
				SessionID:    "abc",
				Provider:     "test",
				Model:        "test",
				CreatedAt:    time.Now(),
				Snippet:      "snippet",
				Relevance:    1.0,
				MessageCount: 1,
			},
		}
		output := FormatResults(results)
		if !strings.Contains(output, "abc") {
			t.Errorf("expected full short ID, got %q", output)
		}
	})

	t.Run("multiple results are numbered", func(t *testing.T) {
		results := []SearchResult{
			{SessionID: "id1", Provider: "p1", Model: "m1", CreatedAt: time.Now(), Snippet: "a", Relevance: 1.0, MessageCount: 1},
			{SessionID: "id2", Provider: "p2", Model: "m2", CreatedAt: time.Now(), Snippet: "b", Relevance: 0.5, MessageCount: 2},
		}
		output := FormatResults(results)
		if !strings.Contains(output, "1.") {
			t.Error("expected first numbered entry")
		}
		if !strings.Contains(output, "2.") {
			t.Error("expected second numbered entry")
		}
	})
}

func TestSearchResultStruct(t *testing.T) {
	t.Parallel()

	r := SearchResult{
		SessionID:    "test-id",
		Provider:     "openai",
		Model:        "gpt-4o",
		Snippet:      "matched text",
		Relevance:    0.75,
		MessageCount: 10,
	}

	if r.SessionID != "test-id" {
		t.Error("expected SessionID")
	}
	if r.Relevance != 0.75 {
		t.Error("expected Relevance 0.75")
	}
}
