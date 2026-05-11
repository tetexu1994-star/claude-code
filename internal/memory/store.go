package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const ENTRYPOINT_NAME = "MEMORY.md"

// Store manages the file-based memory directory.
type Store struct {
	dir  string
	cfg  StoreConfig
	mu   sync.RWMutex
}

// NewStore creates a Store rooted at the given directory.
func NewStore(dir string) *Store {
	return &Store{
		dir: dir,
		cfg: DefaultStoreConfig(dir),
	}
}

// DefaultStore returns a Store using ~/.tlaude-code/memory/.
func DefaultStore() *Store {
	home, _ := os.UserHomeDir()
	return NewStore(filepath.Join(home, ".tlaude-code", "memory"))
}

// Dir returns the store's directory path.
func (s *Store) Dir() string {
	return s.dir
}

// EnsureDir creates the memory directory if it doesn't exist (idempotent).
func (s *Store) EnsureDir() error {
	return os.MkdirAll(s.dir, 0755)
}

// Write saves a memory file with YAML frontmatter. If filename is empty,
// it auto-generates one from the name. Returns the filename written.
func (s *Store) Write(name, description, mtype, content string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if name == "" {
		return "", fmt.Errorf("memory name is required")
	}
	if !validMemoryContent(content) {
		return "", fmt.Errorf("memory content must start with YAML frontmatter (---)")
	}

	mt, ok := ParseMemoryType(mtype)
	if !ok {
		return "", fmt.Errorf("invalid memory type: %q (must be one of: user, feedback, project, reference)", mtype)
	}

	if err := s.EnsureDir(); err != nil {
		return "", fmt.Errorf("creating memory dir: %w", err)
	}

	filename := nameToFilename(name)
	path := filepath.Join(s.dir, filename)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("writing memory file: %w", err)
	}

	// Update MEMORY.md index.
	_ = s.rebuildIndexLocked()

	_ = mt // consumed for validation above
	return filename, nil
}

// Read reads a memory file by filename.
func (s *Store) Read(filename string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Sanitize: prevent directory traversal.
	filename = filepath.Base(filename)
	path := filepath.Join(s.dir, filename)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("memory file %q not found", filename)
		}
		return "", fmt.Errorf("reading memory file: %w", err)
	}
	return string(data), nil
}

// Delete removes a memory file and rebuilds the index.
func (s *Store) Delete(filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename = filepath.Base(filename)
	path := filepath.Join(s.dir, filename)

	if filename == ENTRYPOINT_NAME {
		return fmt.Errorf("cannot delete %s directly", ENTRYPOINT_NAME)
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("memory file %q not found", filename)
		}
		return fmt.Errorf("deleting memory file: %w", err)
	}

	_ = s.rebuildIndexLocked()
	return nil
}

// List returns all memory headers, sorted newest-first.
func (s *Store) List() ([]MemoryHeader, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scanDir()
}

// Search returns memory headers matching a query (keyword match on filename + description).
func (s *Store) Search(query string) ([]MemoryHeader, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	headers, err := s.scanDir()
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var results []MemoryHeader
	for _, h := range headers {
		if strings.Contains(strings.ToLower(h.Filename), query) ||
			strings.Contains(strings.ToLower(h.Description), query) {
			results = append(results, h)
		}
	}
	return results, nil
}

// Count returns the number of memory files (excluding MEMORY.md).
func (s *Store) Count() int {
	headers, err := s.List()
	if err != nil {
		return 0
	}
	return len(headers)
}

// rebuildIndexLocked regenerates MEMORY.md from current memory files.
// Must be called with s.mu held (write lock).
func (s *Store) rebuildIndexLocked() error {
	headers, err := s.scanDir()
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("# Memory\n\n")
	sb.WriteString(fmt.Sprintf("You have a persistent, file-based memory system at `%s`. ", s.dir))
	sb.WriteString("This directory already exists — write to it directly with the Write tool (do not run mkdir or check for its existence).\n\n")

	for _, h := range headers {
		desc := h.Description
		if desc == "" {
			desc = h.Filename
		}
		// Truncate entry lines to ~150 chars.
		entry := fmt.Sprintf("- [%s](%s) — %s", h.Filename, h.Filename, desc)
		if len(entry) > 200 {
			entry = entry[:197] + "..."
		}
		sb.WriteString(entry)
		sb.WriteByte('\n')
	}

	content := sb.String()

	// Apply line cap.
	lines := strings.Split(content, "\n")
	maxLines := s.cfg.MaxIndexLines
	if maxLines <= 0 {
		maxLines = 200
	}
	if len(lines) > maxLines {
		content = strings.Join(lines[:maxLines], "\n")
		content += fmt.Sprintf("\n\n> WARNING: %s is %d lines (limit: %d). Only part of it was loaded. Keep index entries to one line under ~150 chars; move detail into topic files.",
			ENTRYPOINT_NAME, len(lines), maxLines)
	}

	path := filepath.Join(s.dir, ENTRYPOINT_NAME)
	return os.WriteFile(path, []byte(content), 0644)
}

// validContent checks if the content starts with YAML frontmatter.
func validMemoryContent(content string) bool {
	return strings.HasPrefix(strings.TrimSpace(content), "---")
}

// nameToFilename converts a memory name to a safe filename.
func nameToFilename(name string) string {
	// Lowercase, replace non-alphanumeric with underscore, trim underscores.
	re := regexp.MustCompile(`[^a-z0-9]+`)
	filename := strings.ToLower(name)
	filename = strings.TrimSpace(filename)
	filename = re.ReplaceAllString(filename, "_")
	filename = strings.Trim(filename, "_")
	if filename == "" {
		filename = "memory"
	}
	return filename + ".md"
}
