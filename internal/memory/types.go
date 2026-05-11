package memory

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// MemoryType represents one of the four valid memory categories.
type MemoryType string

const (
	MemoryUser      MemoryType = "user"
	MemoryFeedback  MemoryType = "feedback"
	MemoryProject   MemoryType = "project"
	MemoryReference MemoryType = "reference"
)

// ParseMemoryType converts a raw string to a MemoryType, case-insensitive.
func ParseMemoryType(raw string) (MemoryType, bool) {
	switch strings.ToLower(raw) {
	case "user":
		return MemoryUser, true
	case "feedback":
		return MemoryFeedback, true
	case "project":
		return MemoryProject, true
	case "reference":
		return MemoryReference, true
	default:
		return "", false
	}
}

// ValidMemoryTypes returns all valid memory types.
func ValidMemoryTypes() []MemoryType {
	return []MemoryType{MemoryUser, MemoryFeedback, MemoryProject, MemoryReference}
}

// MemoryFrontmatter is the YAML frontmatter parsed from each memory file.
type MemoryFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Type        string `yaml:"type"`
}

// ParseFrontmatter extracts YAML frontmatter from markdown content.
// Returns the frontmatter, the remaining body, or an error if invalid.
func ParseFrontmatter(content string) (*MemoryFrontmatter, string, error) {
	const delim = "---"
	if !strings.HasPrefix(strings.TrimSpace(content), delim) {
		return nil, content, fmt.Errorf("frontmatter must start with ---")
	}

	// Strip leading whitespace and first delimiter.
	rest := strings.TrimSpace(content)
	rest = rest[len(delim):]

	// Find closing delimiter.
	endIdx := strings.Index(rest, delim)
	if endIdx < 0 {
		return nil, content, fmt.Errorf("frontmatter closing --- not found")
	}

	fmRaw := rest[:endIdx]
	body := rest[endIdx+len(delim):]

	var fm MemoryFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return nil, content, fmt.Errorf("invalid frontmatter YAML: %w", err)
	}

	return &fm, strings.TrimSpace(body), nil
}

// MemoryHeader holds metadata for a single memory file (used in listings).
type MemoryHeader struct {
	Filename    string
	FilePath    string
	ModTime     time.Time
	Description string
	Type        MemoryType
}

// StoreConfig configures the memory store behavior.
type StoreConfig struct {
	BaseDir       string
	MaxIndexLines int // default 200
	MaxIndexBytes int // default 25000
	MaxMemFiles   int // default 200
}

// DefaultStoreConfig returns sensible defaults.
func DefaultStoreConfig(baseDir string) StoreConfig {
	return StoreConfig{
		BaseDir:       baseDir,
		MaxIndexLines: 200,
		MaxIndexBytes: 25000,
		MaxMemFiles:   200,
	}
}

// ParseType converts the frontmatter's raw type string to MemoryType.
func (fm *MemoryFrontmatter) ParseType() (MemoryType, bool) {
	return ParseMemoryType(fm.Type)
}

// IsValid checks that Name is non-empty and Type is valid.
func (fm *MemoryFrontmatter) IsValid() bool {
	_, ok := fm.ParseType()
	return fm.Name != "" && ok
}
