package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tetexu/tlaude-code/internal/llm"
)

func TestNewAndSave(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	sess := store.New("anthropic", "claude-sonnet-4-20250514")
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if sess.Provider != "anthropic" {
		t.Fatalf("expected provider anthropic, got %s", sess.Provider)
	}
	if sess.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("expected model claude-sonnet-4-20250514, got %s", sess.Model)
	}
	if len(sess.Messages) != 0 {
		t.Fatal("expected empty messages")
	}

	sess.Messages = append(sess.Messages, llm.Message{Role: "user", Content: "hello"})
	if err := store.Save(sess); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists.
	path := filepath.Join(dir, sess.ID+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("session file not created")
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	sess := store.New("openai", "gpt-4o")
	sess.Messages = append(sess.Messages,
		llm.Message{Role: "user", Content: "test"},
		llm.Message{Role: "assistant", Content: "response"},
	)
	if err := store.Save(sess); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.ID != sess.ID {
		t.Fatalf("ID mismatch: %s != %s", loaded.ID, sess.ID)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "test" {
		t.Fatalf("message mismatch: %s", loaded.Messages[0].Content)
	}
}

func TestListAndLatest(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	list, err := store.List()
	if err != nil {
		t.Fatalf("list on empty dir failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatal("expected empty list")
	}

	latest, err := store.Latest()
	if err != nil {
		t.Fatalf("latest on empty dir failed: %v", err)
	}
	if latest != nil {
		t.Fatal("expected nil latest")
	}

	s1 := store.New("a", "m1")
	s1.Messages = []llm.Message{{Role: "user", Content: "first"}}
	store.Save(s1)

	list, err = store.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 session, got %d", len(list))
	}

	latest, err = store.Latest()
	if err != nil {
		t.Fatalf("latest failed: %v", err)
	}
	if latest.ID != s1.ID {
		t.Fatalf("latest ID mismatch: %s != %s", latest.ID, s1.ID)
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	sess := store.New("a", "m")
	if err := store.Save(sess); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if err := store.Delete(sess.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err := store.Load(sess.ID)
	if err == nil {
		t.Fatal("expected error loading deleted session")
	}

	// Delete non-existent should not error.
	if err := store.Delete("nonexistent"); err != nil {
		t.Fatalf("delete nonexistent failed: %v", err)
	}
}

func TestDefaultStore(t *testing.T) {
	store := DefaultStore()
	if store.Dir() == "" {
		t.Fatal("expected non-empty default dir")
	}
}
