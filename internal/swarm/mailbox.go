package swarm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// mailboxState tracks read status for mailbox messages.
type mailboxState struct {
	mu   sync.Mutex
	read map[string]bool // filename -> true if read
}

func newMailboxState() *mailboxState {
	return &mailboxState{read: make(map[string]bool)}
}

// WriteToMailbox writes a message to the specified agent's mailbox.
func WriteToMailbox(teamName, agentName string, msg MailboxMessage) error {
	dir := AgentMailboxDir(teamName, agentName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create mailbox dir: %w", err)
	}

	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	filename := fmt.Sprintf("%d-%s.json", msg.Timestamp.UnixNano(), shortID())
	filePath := filepath.Join(dir, filename)

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

// ReadMailbox reads all messages from an agent's mailbox, ordered by timestamp.
// Returns entries with Read status based on the provided state tracker.
func ReadMailbox(teamName, agentName string, state *mailboxState) ([]MailboxEntry, error) {
	dir := AgentMailboxDir(teamName, agentName)
	return readMailboxDir(dir, state)
}

// readMailboxDir reads all .json messages from a directory.
func readMailboxDir(dir string, state *mailboxState) ([]MailboxEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read mailbox dir: %w", err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	var result []MailboxEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var msg MailboxMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		result = append(result, MailboxEntry{
			Filename: entry.Name(),
			Read:     state.read[entry.Name()],
			Message:  msg,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Message.Timestamp.Before(result[j].Message.Timestamp)
	})

	return result, nil
}

// MarkAsRead marks a message as read in the mailbox state.
func MarkAsRead(teamName, agentName, filename string, state *mailboxState) error {
	dir := AgentMailboxDir(teamName, agentName)
	filePath := filepath.Join(dir, filename)

	// Verify the file exists.
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("message file not found: %w", err)
	}

	state.mu.Lock()
	state.read[filename] = true
	state.mu.Unlock()
	return nil
}

// NewMailboxState creates a new mailbox state tracker for a SwarmStore.
func NewMailboxState() *mailboxState {
	return newMailboxState()
}
