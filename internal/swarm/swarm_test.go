package swarm

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func tempDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "swarm-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

func TestNewSwarmStore(t *testing.T) {
	dir := tempDir(t)
	s, err := NewSwarmStoreAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s == nil {
		t.Fatal("nil store")
	}
	if len(s.ListTeams()) != 0 {
		t.Fatal("expected no teams")
	}
}

func TestCreateTeam(t *testing.T) {
	s, _ := NewSwarmStoreAt(tempDir(t))

	tf, err := s.CreateTeam("my-team", "A test team", "leader")
	if err != nil {
		t.Fatal(err)
	}
	if tf.Name != "my-team" {
		t.Fatalf("expected 'my-team', got %q", tf.Name)
	}
	if tf.Description != "A test team" {
		t.Fatalf("expected description, got %q", tf.Description)
	}
	if tf.LeadAgentID != "leader" {
		t.Fatalf("expected 'leader', got %q", tf.LeadAgentID)
	}
	if len(tf.Members) != 0 {
		t.Fatalf("expected 0 members, got %d", len(tf.Members))
	}

	// Verify it's persisted.
	configPath := TeamConfigPath("my-team")
	// We use the custom dir, so the file is at dir/my-team/config.json
	if _, err := os.Stat(filepath.Join(s.teamsDir, "my-team", ConfigFileName)); err != nil {
		t.Fatalf("config file not found: %v", err)
	}
	_ = configPath

	// Duplicate team name should fail.
	_, err = s.CreateTeam("my-team", "", "")
	if err == nil {
		t.Fatal("expected error for duplicate team")
	}
}

func TestGetAndListTeams(t *testing.T) {
	s, _ := NewSwarmStoreAt(tempDir(t))
	s.CreateTeam("alpha", "First", "leader")
	s.CreateTeam("beta", "Second", "leader")

	tf, ok := s.GetTeam("alpha")
	if !ok {
		t.Fatal("team alpha not found")
	}
	if tf.Name != "alpha" {
		t.Fatalf("expected 'alpha', got %q", tf.Name)
	}

	_, ok = s.GetTeam("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}

	teams := s.ListTeams()
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
}

func TestRemoveTeam(t *testing.T) {
	s, _ := NewSwarmStoreAt(tempDir(t))
	s.CreateTeam("test-team", "", "leader")

	if err := s.RemoveTeam("test-team"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetTeam("test-team"); ok {
		t.Fatal("team should be removed")
	}

	// Removing again should fail.
	if err := s.RemoveTeam("test-team"); err == nil {
		t.Fatal("expected error")
	}
}

func TestAddAndRemoveMember(t *testing.T) {
	s, _ := NewSwarmStoreAt(tempDir(t))
	s.CreateTeam("dev-team", "", "leader")

	member := TeamMember{
		Name:  "researcher",
		Color: "blue",
		Mode:  "default",
	}
	if err := s.AddMember("dev-team", member); err != nil {
		t.Fatal(err)
	}

	tf, _ := s.GetTeam("dev-team")
	if len(tf.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(tf.Members))
	}
	m := tf.Members[0]
	if m.Name != "researcher" {
		t.Fatalf("expected 'researcher', got %q", m.Name)
	}
	if m.AgentID != "researcher@dev-team" {
		t.Fatalf("expected 'researcher@dev-team', got %q", m.AgentID)
	}
	if !m.IsActive {
		t.Fatal("expected IsActive=true")
	}

	// Remove member.
	if err := s.RemoveMember("dev-team", "researcher@dev-team"); err != nil {
		t.Fatal(err)
	}
	tf, _ = s.GetTeam("dev-team")
	if len(tf.Members) != 0 {
		t.Fatalf("expected 0 members, got %d", len(tf.Members))
	}
}

func TestUpdateMemberStatus(t *testing.T) {
	s, _ := NewSwarmStoreAt(tempDir(t))
	s.CreateTeam("dev-team", "", "leader")
	s.AddMember("dev-team", TeamMember{Name: "coder"})

	if err := s.UpdateMemberStatus("dev-team", "coder@dev-team", false, "session-123"); err != nil {
		t.Fatal(err)
	}

	m, ok := s.GetMember("dev-team", "coder@dev-team")
	if !ok {
		t.Fatal("member not found")
	}
	if m.IsActive {
		t.Fatal("expected IsActive=false")
	}
	if m.SessionID != "session-123" {
		t.Fatalf("expected session-123, got %q", m.SessionID)
	}
}

func TestMailboxWriteAndRead(t *testing.T) {
	// Use a unique team name to avoid collisions.
	teamName := fmt.Sprintf("test-mailbox-%d", time.Now().UnixNano())
	agentName := "test-agent"
	mailboxDir := AgentMailboxDir(teamName, agentName)
	t.Cleanup(func() { os.RemoveAll(TeamDir(teamName)) })

	msg := MailboxMessage{
		From:  "leader",
		Text:  "Hello, teammate!",
		Color: "blue",
	}
	if err := WriteToMailbox(teamName, agentName, msg); err != nil {
		t.Fatal(err)
	}

	// Verify file exists.
	files, err := os.ReadDir(mailboxDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	// Read messages.
	state := NewMailboxState()
	entries, err := ReadMailbox(teamName, agentName, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Message.Text != "Hello, teammate!" {
		t.Fatalf("wrong message: %q", entries[0].Message.Text)
	}
	if entries[0].Message.From != "leader" {
		t.Fatalf("wrong from: %q", entries[0].Message.From)
	}
	if entries[0].Read {
		t.Fatal("expected unread")
	}
}

func TestMailboxMarkAsRead(t *testing.T) {
	teamName := fmt.Sprintf("test-markread-%d", time.Now().UnixNano())
	agentName := "test-agent"
	t.Cleanup(func() { os.RemoveAll(TeamDir(teamName)) })

	WriteToMailbox(teamName, agentName, MailboxMessage{From: "leader", Text: "msg1"})
	WriteToMailbox(teamName, agentName, MailboxMessage{From: "leader", Text: "msg2"})

	state := NewMailboxState()
	entries, _ := ReadMailbox(teamName, agentName, state)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Mark first as read.
	if err := MarkAsRead(teamName, agentName, entries[0].Filename, state); err != nil {
		t.Fatal(err)
	}

	// Re-read.
	entries, _ = ReadMailbox(teamName, agentName, state)
	if !entries[0].Read {
		t.Fatal("expected first entry to be read")
	}
	if entries[1].Read {
		t.Fatal("expected second entry to be unread")
	}
}

func TestPermissionRequest(t *testing.T) {
	s, _ := NewSwarmStoreAt(tempDir(t))

	req := &PermissionRequest{
		ToolName:    "Bash",
		ToolUseID:   "tool-123",
		Description: "Run ls command",
		WorkerID:    "worker-1",
		WorkerName:  "test-worker",
		TeamName:    "test-team",
	}

	ch, err := s.SubmitPermissionRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if req.ID == "" {
		t.Fatal("expected request ID to be set")
	}

	// Check pending requests.
	pending := s.GetPendingRequests("test-team")
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending request, got %d", len(pending))
	}

	// Resolve the request.
	resp := PermissionResponse{
		Approved:    true,
		Feedback:    "looks good",
	}
	if err := s.ResolvePermissionRequest(req.ID, resp); err != nil {
		t.Fatal(err)
	}

	// Should receive response on channel.
	select {
	case got := <-ch:
		if !got.Approved {
			t.Fatal("expected approved")
		}
		if got.Feedback != "looks good" {
			t.Fatalf("wrong feedback: %q", got.Feedback)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Resolving again should fail.
	if err := s.ResolvePermissionRequest(req.ID, resp); err == nil {
		t.Fatal("expected error for duplicate resolution")
	}
}

func TestConcurrentSafety(t *testing.T) {
	s, _ := NewSwarmStoreAt(tempDir(t))
	s.CreateTeam("concurrent", "", "leader")

	var wg sync.WaitGroup
	n := 50

	// Concurrent member adds.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("worker-%d", idx)
			_ = s.AddMember("concurrent", TeamMember{Name: name})
		}(i)
	}
	wg.Wait()

	tf, _ := s.GetTeam("concurrent")
	if len(tf.Members) != n {
		t.Fatalf("expected %d members, got %d", n, len(tf.Members))
	}
}

func TestConcurrentPermissionRequests(t *testing.T) {
	s, _ := NewSwarmStoreAt(tempDir(t))

	var wg sync.WaitGroup
	n := 20

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := &PermissionRequest{
				ToolName:  fmt.Sprintf("tool-%d", idx),
				WorkerID:  fmt.Sprintf("worker-%d", idx),
				TeamName:  "test-team",
			}
			s.SubmitPermissionRequest(req)
		}(i)
	}
	wg.Wait()

	all := s.GetAllPendingRequests()
	if len(all) != n {
		t.Fatalf("expected %d pending requests, got %d", n, len(all))
	}

	// Resolve all.
	for _, req := range all {
		s.ResolvePermissionRequest(req.ID, PermissionResponse{Approved: true})
	}

	remaining := s.GetAllPendingRequests()
	if len(remaining) != 0 {
		t.Fatalf("expected 0 remaining, got %d", len(remaining))
	}
}

func TestParseAgentID(t *testing.T) {
	tests := []struct {
		agentID        string
		expectTeam     string
		expectAgent    string
	}{
		{"researcher@my-team", "my-team", "researcher"},
		{"simple", "simple", "simple"},
		{"a@b@c", "c", "a@b"},
	}

	for _, tt := range tests {
		team, agent := parseAgentID(tt.agentID)
		if team != tt.expectTeam || agent != tt.expectAgent {
			t.Errorf("parseAgentID(%q) = (%q, %q), want (%q, %q)",
				tt.agentID, team, agent, tt.expectTeam, tt.expectAgent)
		}
	}
}

func TestTeamPersistence(t *testing.T) {
	dir := tempDir(t)
	s, _ := NewSwarmStoreAt(dir)
	s.CreateTeam("persist", "Persistent team", "leader")
	s.AddMember("persist", TeamMember{Name: "worker"})

	// Create a new SwarmStore loading from the same directory.
	s2, err := NewSwarmStoreAt(dir)
	if err != nil {
		t.Fatal(err)
	}

	tf, ok := s2.GetTeam("persist")
	if !ok {
		t.Fatal("team not loaded from disk")
	}
	if tf.Description != "Persistent team" {
		t.Fatalf("expected description, got %q", tf.Description)
	}
	if len(tf.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(tf.Members))
	}
	if tf.Members[0].Name != "worker" {
		t.Fatalf("expected 'worker', got %q", tf.Members[0].Name)
	}
}

func TestMailboxConcurrentReadWrite(t *testing.T) {
	teamName := fmt.Sprintf("test-conc-mb-%d", time.Now().UnixNano())
	agentName := "agent"
	t.Cleanup(func() { os.RemoveAll(TeamDir(teamName)) })

	var wg sync.WaitGroup
	n := 30

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = WriteToMailbox(teamName, agentName, MailboxMessage{
				From: fmt.Sprintf("sender-%d", idx),
				Text: fmt.Sprintf("message-%d", idx),
			})
		}(i)
	}
	wg.Wait()

	state := NewMailboxState()
	entries, err := ReadMailbox(teamName, agentName, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != n {
		t.Fatalf("expected %d entries, got %d", n, len(entries))
	}
}
