package memory

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// MAX_MEMORY_FILES caps the number of memory files scanned.
	MAX_MEMORY_FILES = 200
	// FRONTMATTER_MAX_LINES limits how many lines we read for frontmatter parsing.
	FRONTMATTER_MAX_LINES = 30
)

// scanDir reads a memory directory, parses all .md files (except MEMORY.md),
// extracts frontmatter, and returns sorted MemoryHeaders.
func (s *Store) scanDir() ([]MemoryHeader, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var headers []MemoryHeader
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == ENTRYPOINT_NAME {
			continue
		}

		filePath := filepath.Join(s.dir, entry.Name())
		fm, _, err := readFrontmatter(filePath)
		if err != nil {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		mt, _ := fm.ParseType()
		headers = append(headers, MemoryHeader{
			Filename:    entry.Name(),
			FilePath:    filePath,
			ModTime:     info.ModTime(),
			Description: fm.Description,
			Type:        mt,
		})
	}

	// Sort newest-first by modification time.
	sort.Slice(headers, func(i, j int) bool {
		return headers[i].ModTime.After(headers[j].ModTime)
	})

	// Cap at MAX_MEMORY_FILES.
	if len(headers) > MAX_MEMORY_FILES {
		headers = headers[:MAX_MEMORY_FILES]
	}

	return headers, nil
}

// readFrontmatter reads the first FRONTMATTER_MAX_LINES of a file and parses
// YAML frontmatter. Returns the frontmatter, the remaining body, or an error.
func readFrontmatter(path string) (*MemoryFrontmatter, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}

	content := string(data)

	// Only read up to FRONTMATTER_MAX_LINES worth of content for parsing.
	lines := strings.SplitN(content, "\n", FRONTMATTER_MAX_LINES+1)
	if len(lines) > FRONTMATTER_MAX_LINES {
		content = strings.Join(lines[:FRONTMATTER_MAX_LINES], "\n")
	}

	return ParseFrontmatter(content)
}

// FormatMemoryManifest formats memory headers as a text listing.
// One line per file: "- [type] filename (timestamp): description".
func FormatMemoryManifest(headers []MemoryHeader) string {
	if len(headers) == 0 {
		return "No memories stored."
	}

	var sb strings.Builder
	for _, h := range headers {
		if h.Type != "" {
			sb.WriteString("- [")
			sb.WriteString(string(h.Type))
			sb.WriteString("] ")
		} else {
			sb.WriteString("- ")
		}
		sb.WriteString(h.Filename)
		sb.WriteString(" (")
		sb.WriteString(h.ModTime.Format("2006-01-02 15:04"))
		sb.WriteString(")")
		if h.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(h.Description)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}
