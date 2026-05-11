package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tetexu/tlaude-code/internal/llm"
)

// Session represents a persisted chat session.
type Session struct {
	ID               string        `json:"id"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
	Messages         []llm.Message `json:"messages"`
	Provider         string        `json:"provider"`
	Model            string        `json:"model"`
	CompactBoundary  string        `json:"compact_boundary,omitempty"`
}

// Store manages session persistence in ~/.tlaude-code/sessions/.
type Store struct {
	mu  sync.Mutex
	dir string
}

// DefaultStore returns a Store using the default session directory.
func DefaultStore() *Store {
	home, _ := os.UserHomeDir()
	return &Store{dir: filepath.Join(home, ".tlaude-code", "sessions")}
}

// NewStore returns a Store rooted at the given directory.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// New creates a new session with the given provider and model.
func (s *Store) New(provider, model string) *Session {
	return &Session{
		ID:        newID(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  make([]llm.Message, 0),
		Provider:  provider,
		Model:     model,
	}
}

// Save persists a session to disk. It updates UpdatedAt before writing.
func (s *Store) Save(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess.ID == "" {
		return fmt.Errorf("session ID is empty")
	}

	sess.UpdatedAt = time.Now()

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("creating session directory: %w", err)
	}

	path := filepath.Join(s.dir, sess.ID+".json")
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing session file: %w", err)
	}

	return nil
}

// Load reads a session from disk by ID.
func (s *Store) Load(id string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session %s not found", id)
		}
		return nil, fmt.Errorf("reading session file: %w", err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("unmarshaling session: %w", err)
	}

	return &sess, nil
}

// List returns all sessions sorted by UpdatedAt descending.
func (s *Store) List() ([]*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return nil, fmt.Errorf("creating session directory: %w", err)
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading session directory: %w", err)
	}

	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(s.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		sessions = append(sessions, &sess)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// Latest returns the most recently updated session, or nil if none exist.
func (s *Store) Latest() (*Session, error) {
	sessions, err := s.List()
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return sessions[0], nil
}

// Delete removes a session from disk by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting session file: %w", err)
	}
	return nil
}

// Dir returns the store's directory path.
func (s *Store) Dir() string {
	return s.dir
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}
