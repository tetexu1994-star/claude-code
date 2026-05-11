package memory

import (
	"encoding/json"
	"fmt"
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

// --- MemDir system tests ---

func TestParseMemoryType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw      string
		expected MemoryType
		ok       bool
	}{
		{"user", MemoryUser, true},
		{"USER", MemoryUser, true},
		{"User", MemoryUser, true},
		{"feedback", MemoryFeedback, true},
		{"project", MemoryProject, true},
		{"reference", MemoryReference, true},
		{"invalid", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		got, ok := ParseMemoryType(tt.raw)
		if ok != tt.ok || got != tt.expected {
			t.Errorf("ParseMemoryType(%q) = (%q, %v), want (%q, %v)", tt.raw, got, ok, tt.expected, tt.ok)
		}
	}
}

func TestValidMemoryTypes(t *testing.T) {
	types := ValidMemoryTypes()
	if len(types) != 4 {
		t.Errorf("expected 4 types, got %d", len(types))
	}
	seen := make(map[MemoryType]bool)
	for _, mt := range types {
		seen[mt] = true
	}
	for _, want := range []MemoryType{MemoryUser, MemoryFeedback, MemoryProject, MemoryReference} {
		if !seen[want] {
			t.Errorf("missing type %q", want)
		}
	}
}

func TestParseFrontmatter(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		content := `---
name: user role
description: Senior Go engineer
type: user
---

Memory content here.`

		fm, body, err := ParseFrontmatter(content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fm.Name != "user role" {
			t.Errorf("expected name 'user role', got %q", fm.Name)
		}
		if fm.Description != "Senior Go engineer" {
			t.Errorf("expected description, got %q", fm.Description)
		}
		if fm.Type != "user" {
			t.Errorf("expected type 'user', got %q", fm.Type)
		}
		if body != "Memory content here." {
			t.Errorf("expected body 'Memory content here.', got %q", body)
		}
	})

	t.Run("no frontmatter", func(t *testing.T) {
		_, _, err := ParseFrontmatter("just content, no frontmatter")
		if err == nil {
			t.Error("expected error for missing frontmatter")
		}
	})

	t.Run("no closing delimiter", func(t *testing.T) {
		content := "---\nname: test\ntype: user\n"
		_, _, err := ParseFrontmatter(content)
		if err == nil {
			t.Error("expected error for missing closing ---")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		content := "---\n{{invalid yaml!!!\n---\nbody"
		_, _, err := ParseFrontmatter(content)
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})
}

func TestMemoryFrontmatter_IsValid(t *testing.T) {
	t.Parallel()

	valid := &MemoryFrontmatter{Name: "test", Type: "user"}
	if !valid.IsValid() {
		t.Error("expected valid frontmatter")
	}

	noName := &MemoryFrontmatter{Name: "", Type: "user"}
	if noName.IsValid() {
		t.Error("expected invalid (no name)")
	}

	badType := &MemoryFrontmatter{Name: "test", Type: "bad"}
	if badType.IsValid() {
		t.Error("expected invalid (bad type)")
	}
}

func TestNewStore(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if s == nil {
		t.Fatal("expected store, got nil")
	}
	if s.dir != dir {
		t.Errorf("expected dir %q, got %q", dir, s.dir)
	}
}

func TestStore_EnsureDir(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	s := NewStore(memDir)

	if err := s.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}

	info, err := os.Stat(memDir)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}

	// Idempotent.
	if err := s.EnsureDir(); err != nil {
		t.Fatalf("second EnsureDir failed: %v", err)
	}
}

func TestStore_WriteAndRead(t *testing.T) {
	s := NewStore(t.TempDir())

	content := `---
name: user role
description: Test memory
type: user
---

Test content.`

	filename, err := s.Write("user role", "Test memory", "user", content)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if filename == "" {
		t.Fatal("expected non-empty filename")
	}
	if !strings.HasSuffix(filename, ".md") {
		t.Errorf("expected .md file, got %q", filename)
	}

	// Read it back.
	read, err := s.Read(filename)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if read != content {
		t.Errorf("content mismatch:\nwant: %q\ngot:  %q", content, read)
	}
}

func TestStore_WriteInvalidType(t *testing.T) {
	s := NewStore(t.TempDir())

	content := `---
name: test
description: bad type
type: invalid
---

Content.`

	_, err := s.Write("test", "bad type", "invalid", content)
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestStore_WriteNoFrontmatter(t *testing.T) {
	s := NewStore(t.TempDir())

	_, err := s.Write("test", "desc", "user", "no frontmatter content")
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}
}

func TestStore_ReadNotFound(t *testing.T) {
	s := NewStore(t.TempDir())
	_, err := s.Read("nonexistent.md")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestStore_Delete(t *testing.T) {
	s := NewStore(t.TempDir())

	content := `---
name: test
description: to delete
type: user
---

Delete me.`

	filename, err := s.Write("test", "to delete", "user", content)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if err := s.Delete(filename); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = s.Read(filename)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestStore_DeleteNotFound(t *testing.T) {
	s := NewStore(t.TempDir())
	err := s.Delete("nonexistent.md")
	if err == nil {
		t.Error("expected error for deleting nonexistent file")
	}
}

func TestStore_DeleteMEMORYMD(t *testing.T) {
	s := NewStore(t.TempDir())
	err := s.Delete(ENTRYPOINT_NAME)
	if err == nil {
		t.Error("expected error when deleting MEMORY.md")
	}
}

func TestStore_List(t *testing.T) {
	s := NewStore(t.TempDir())

	// Empty list.
	headers, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(headers) != 0 {
		t.Errorf("expected 0 headers, got %d", len(headers))
	}

	// Write a few memories.
	memories := []struct {
		name, desc, mtype, content string
	}{
		{
			"first memory", "First test memory", "user",
			"---\nname: first memory\ndescription: First test memory\ntype: user\n---\nContent 1.",
		},
		{
			"second feedback", "Second test memory", "feedback",
			"---\nname: second feedback\ndescription: Second test memory\ntype: feedback\n---\nContent 2.",
		},
		{
			"third project", "Third test memory", "project",
			"---\nname: third project\ndescription: Third test memory\ntype: project\n---\nContent 3.",
		},
	}

	for _, m := range memories {
		if _, err := s.Write(m.name, m.desc, m.mtype, m.content); err != nil {
			t.Fatalf("Write %q failed: %v", m.name, err)
		}
	}

	headers, err = s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(headers) != 3 {
		t.Errorf("expected 3 headers, got %d", len(headers))
	}

	// Check sorted by mod time (newest first).
	for i := 1; i < len(headers); i++ {
		if headers[i].ModTime.After(headers[i-1].ModTime) {
			t.Error("headers not sorted newest-first")
		}
	}
}

func TestStore_Search(t *testing.T) {
	s := NewStore(t.TempDir())

	content1 := "---\nname: golang tips\ndescription: Useful Go patterns\ntype: user\n---\nContent."
	content2 := "---\nname: python notes\ndescription: Python tips\ntype: reference\n---\nContent."

	if _, err := s.Write("golang tips", "Useful Go patterns", "user", content1); err != nil {
		t.Fatalf("Write 1 failed: %v", err)
	}
	if _, err := s.Write("python notes", "Python tips", "reference", content2); err != nil {
		t.Fatalf("Write 2 failed: %v", err)
	}

	results, err := s.Search("golang")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].Filename != "golang_tips.md" {
		t.Errorf("expected golang_tips.md, got %q", results[0].Filename)
	}
}

func TestStore_SearchNoMatch(t *testing.T) {
	s := NewStore(t.TempDir())

	content := "---\nname: test\ndescription: Test\ntype: user\n---\nContent."
	s.Write("test", "Test", "user", content)

	results, err := s.Search("zzzzz_nonexistent")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestStore_Count(t *testing.T) {
	s := NewStore(t.TempDir())

	if c := s.Count(); c != 0 {
		t.Errorf("expected 0, got %d", c)
	}

	content := "---\nname: test\ndescription: Test\ntype: user\n---\nContent."
	s.Write("test", "Test", "user", content)

	if c := s.Count(); c != 1 {
		t.Errorf("expected 1, got %d", c)
	}
}

func TestFormatMemoryManifest(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		out := FormatMemoryManifest(nil)
		if !strings.Contains(out, "No memories stored") {
			t.Errorf("expected 'No memories stored', got %q", out)
		}
	})

	t.Run("with headers", func(t *testing.T) {
		headers := []MemoryHeader{
			{
				Filename:    "user_role.md",
				FilePath:    "/tmp/user_role.md",
				ModTime:     time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC),
				Description: "Senior Go engineer",
				Type:        MemoryUser,
			},
			{
				Filename:    "feedback_testing.md",
				FilePath:    "/tmp/feedback_testing.md",
				ModTime:     time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC),
				Description: "No mocks in integration tests",
				Type:        MemoryFeedback,
			},
		}

		out := FormatMemoryManifest(headers)
		if !strings.Contains(out, "[user]") {
			t.Error("expected [user] tag")
		}
		if !strings.Contains(out, "[feedback]") {
			t.Error("expected [feedback] tag")
		}
		if !strings.Contains(out, "user_role.md") {
			t.Error("expected user_role.md")
		}
		if !strings.Contains(out, "Senior Go engineer") {
			t.Error("expected description")
		}
	})
}

func TestNameToFilename(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"user role", "user_role.md"},
		{"Golang Tips!", "golang_tips.md"},
		{"  spaces  here  ", "spaces_here.md"},
		{"a/b:c@test", "a_b_c_test.md"},
		{"!!!", "memory.md"},
	}

	for _, tt := range tests {
		got := nameToFilename(tt.name)
		if got != tt.expected {
			t.Errorf("nameToFilename(%q) = %q, want %q", tt.name, got, tt.expected)
		}
	}
}

func TestScanDir(t *testing.T) {
	s := NewStore(t.TempDir())

	// No directory yet.
	headers, err := s.scanDir()
	if err != nil {
		t.Fatalf("scanDir on non-existent dir: %v", err)
	}
	if len(headers) != 0 {
		t.Errorf("expected 0 headers, got %d", len(headers))
	}

	// Create dir and write a memory file.
	s.EnsureDir()
	content := "---\nname: test\ndescription: Scan test\ntype: user\n---\nBody."
	os.WriteFile(filepath.Join(s.dir, "scan_test.md"), []byte(content), 0644)

	headers, err = s.scanDir()
	if err != nil {
		t.Fatalf("scanDir failed: %v", err)
	}
	if len(headers) != 1 {
		t.Errorf("expected 1 header, got %d", len(headers))
	}
	if headers[0].Filename != "scan_test.md" {
		t.Errorf("expected scan_test.md, got %q", headers[0].Filename)
	}
}

func TestScanDir_SkipsMEMORYMD(t *testing.T) {
	s := NewStore(t.TempDir())
	s.EnsureDir()

	// Write MEMORY.md — should be skipped.
	os.WriteFile(filepath.Join(s.dir, ENTRYPOINT_NAME), []byte("# Memory\n"), 0644)
	// Write a real memory.
	content := "---\nname: real\ndescription: Real\ntype: user\n---\nBody."
	os.WriteFile(filepath.Join(s.dir, "real.md"), []byte(content), 0644)

	headers, err := s.scanDir()
	if err != nil {
		t.Fatalf("scanDir failed: %v", err)
	}
	if len(headers) != 1 {
		t.Errorf("expected 1 header (MEMORY.md skipped), got %d", len(headers))
	}
}

func TestScanDir_SkipsNonMD(t *testing.T) {
	s := NewStore(t.TempDir())
	s.EnsureDir()

	os.WriteFile(filepath.Join(s.dir, "notes.txt"), []byte("not markdown"), 0644)

	headers, err := s.scanDir()
	if err != nil {
		t.Fatalf("scanDir failed: %v", err)
	}
	if len(headers) != 0 {
		t.Errorf("expected 0 headers, got %d", len(headers))
	}
}

func TestScanDir_SkipsBadFrontmatter(t *testing.T) {
	s := NewStore(t.TempDir())
	s.EnsureDir()

	os.WriteFile(filepath.Join(s.dir, "bad.md"), []byte("no frontmatter here"), 0644)

	headers, err := s.scanDir()
	if err != nil {
		t.Fatalf("scanDir failed: %v", err)
	}
	if len(headers) != 0 {
		t.Errorf("expected 0 headers, got %d", len(headers))
	}
}

func TestBuildMemoryPrompt(t *testing.T) {
	s := NewStore(t.TempDir())
	_ = s.EnsureDir()

	prompt := s.BuildMemoryPrompt()

	// Check that all sections are present.
	if !strings.Contains(prompt, "# auto memory") {
		t.Error("missing header")
	}
	if !strings.Contains(prompt, "## Types of memory") {
		t.Error("missing types section")
	}
	if !strings.Contains(prompt, "## What NOT to save in memory") {
		t.Error("missing what-not-to-save section")
	}
	if !strings.Contains(prompt, "## How to save memories") {
		t.Error("missing how-to-save section")
	}
	if !strings.Contains(prompt, "## When to access memories") {
		t.Error("missing when-to-access section")
	}
	if !strings.Contains(prompt, "## Before recommending from memory") {
		t.Error("missing before-recommending section")
	}
	if !strings.Contains(prompt, ENTRYPOINT_NAME) {
		t.Error("missing MEMORY.md reference")
	}
}

func TestBuildMemoryPrompt_WithExistingMEMORYMD(t *testing.T) {
	s := NewStore(t.TempDir())
	_ = s.EnsureDir()

	// Write a MEMORY.md file.
	indexContent := "# Memory\n\n- [test.md](test.md) — a test entry\n"
	os.WriteFile(filepath.Join(s.dir, ENTRYPOINT_NAME), []byte(indexContent), 0644)

	prompt := s.BuildMemoryPrompt()
	if !strings.Contains(prompt, "a test entry") {
		t.Error("prompt should contain existing MEMORY.md content")
	}
}

func TestTruncateEntrypointContent(t *testing.T) {
	t.Run("no truncation needed", func(t *testing.T) {
		content := "short content"
		result := truncateEntrypointContent(content, 200, 25000)
		if result != content {
			t.Errorf("expected unchanged, got %q", result)
		}
	})

	t.Run("line truncation", func(t *testing.T) {
		var lines []string
		for i := 0; i < 250; i++ {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
		content := strings.Join(lines, "\n")
		result := truncateEntrypointContent(content, 200, 25000)
		if !strings.Contains(result, "WARNING") {
			t.Error("expected truncation warning")
		}
	})
}

func TestGetAgentMemoryDir(t *testing.T) {
	t.Run("user scope", func(t *testing.T) {
		dir := GetAgentMemoryDir("general", ScopeUser)
		if !strings.Contains(dir, "agent-memory") {
			t.Errorf("expected agent-memory in path, got %q", dir)
		}
		if !strings.HasSuffix(dir, string(filepath.Separator)) {
			t.Error("expected trailing separator")
		}
	})

	t.Run("project scope", func(t *testing.T) {
		dir := GetAgentMemoryDir("general", ScopeProject)
		if !strings.Contains(dir, ".claude") {
			t.Errorf("expected .claude in path, got %q", dir)
		}
	})

	t.Run("local scope", func(t *testing.T) {
		dir := GetAgentMemoryDir("general", ScopeLocal)
		if !strings.Contains(dir, "agent-memory-local") {
			t.Errorf("expected agent-memory-local in path, got %q", dir)
		}
	})

	t.Run("sanitizes agent type", func(t *testing.T) {
		dir := GetAgentMemoryDir("plugin:my-agent", ScopeUser)
		if strings.Contains(dir, ":") {
			t.Errorf("colon not sanitized in path: %q", dir)
		}
	})
}

func TestIsAgentMemoryPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()

	t.Run("user scope path", func(t *testing.T) {
		path := filepath.Join(home, ".tlaude-code", "agent-memory", "general", "MEMORY.md")
		if !IsAgentMemoryPath(path) {
			t.Error("expected true for user scope")
		}
	})

	t.Run("project scope path", func(t *testing.T) {
		path := filepath.Join(cwd, ".claude", "agent-memory", "general", "MEMORY.md")
		if !IsAgentMemoryPath(path) {
			t.Error("expected true for project scope")
		}
	})

	t.Run("local scope path", func(t *testing.T) {
		path := filepath.Join(cwd, ".claude", "agent-memory-local", "general", "MEMORY.md")
		if !IsAgentMemoryPath(path) {
			t.Error("expected true for local scope")
		}
	})

	t.Run("non-memory path", func(t *testing.T) {
		if IsAgentMemoryPath("/tmp/random/file.md") {
			t.Error("expected false for non-memory path")
		}
	})
}

func TestLoadAgentMemoryPrompt(t *testing.T) {
	prompt := LoadAgentMemoryPrompt("general", ScopeUser)
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "# auto memory") {
		t.Error("expected memory header")
	}
}

func TestGetMemoryScopeDisplay(t *testing.T) {
	if d := GetMemoryScopeDisplay(ScopeUser); !strings.Contains(d, "User") {
		t.Errorf("expected User in display, got %q", d)
	}
	if d := GetMemoryScopeDisplay(ScopeProject); !strings.Contains(d, "Project") {
		t.Errorf("expected Project in display, got %q", d)
	}
	if d := GetMemoryScopeDisplay(ScopeLocal); !strings.Contains(d, "Local") {
		t.Errorf("expected Local in display, got %q", d)
	}
}

func TestDefaultStoreConfig(t *testing.T) {
	cfg := DefaultStoreConfig("/tmp/test")
	if cfg.MaxIndexLines != 200 {
		t.Errorf("expected MaxIndexLines 200, got %d", cfg.MaxIndexLines)
	}
	if cfg.MaxIndexBytes != 25000 {
		t.Errorf("expected MaxIndexBytes 25000, got %d", cfg.MaxIndexBytes)
	}
	if cfg.MaxMemFiles != 200 {
		t.Errorf("expected MaxMemFiles 200, got %d", cfg.MaxMemFiles)
	}
	if cfg.BaseDir != "/tmp/test" {
		t.Errorf("expected BaseDir /tmp/test, got %q", cfg.BaseDir)
	}
}

func TestDefaultStore(t *testing.T) {
	s := DefaultStore()
	if s == nil {
		t.Fatal("expected store, got nil")
	}
	if !strings.Contains(s.dir, ".tlaude-code") {
		t.Errorf("expected .tlaude-code in default dir, got %q", s.dir)
	}
}

// Thread safety: concurrent reads should not race.
func TestStore_ConcurrentReads(t *testing.T) {
	s := NewStore(t.TempDir())

	content := "---\nname: test\ndescription: Concurrent\ntype: user\n---\nConcurrent test."
	filename, err := s.Write("test", "Concurrent", "user", content)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = s.Read(filename)
			_, _ = s.List()
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Thread safety: concurrent writes should not race.
func TestStore_ConcurrentWrites(t *testing.T) {
	s := NewStore(t.TempDir())

	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(idx int) {
			content := fmt.Sprintf("---\nname: test%d\ndescription: Test %d\ntype: user\n---\nBody %d.", idx, idx, idx)
			_, _ = s.Write(fmt.Sprintf("test%d", idx), fmt.Sprintf("Test %d", idx), "user", content)
			done <- true
		}(i)
	}
	for i := 0; i < 5; i++ {
		<-done
	}

	headers, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(headers) != 5 {
		t.Errorf("expected 5 files, got %d", len(headers))
	}
}
