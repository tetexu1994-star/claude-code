package swarm

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NewSwarmStore creates a new SwarmStore and loads existing teams from disk.
func NewSwarmStore() (*SwarmStore, error) {
	return NewSwarmStoreAt(TeamsDir())
}

// NewSwarmStoreAt creates a new SwarmStore using a custom base directory.
func NewSwarmStoreAt(dir string) (*SwarmStore, error) {
	s := &SwarmStore{
		teamsDir:     dir,
		teams:        make(map[string]*TeamFile),
		permRequests: make(map[string]*PermissionRequest),
	}
	if err := os.MkdirAll(s.teamsDir, 0755); err != nil {
		return nil, fmt.Errorf("create teams dir: %w", err)
	}
	if err := s.loadTeams(); err != nil {
		return nil, fmt.Errorf("load teams: %w", err)
	}
	return s, nil
}

// SetExecutor sets the TeammateExecutor for spawning agents.
func (s *SwarmStore) SetExecutor(exec TeammateExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executor = exec
}

// Executor returns the current TeammateExecutor.
func (s *SwarmStore) Executor() TeammateExecutor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.executor
}

// CreateTeam creates a new team and persists it to disk.
func (s *SwarmStore) CreateTeam(name, description, leadAgentID string) (*TeamFile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("team name is required")
	}
	if len(name) > MaxTeamNameLen {
		return nil, fmt.Errorf("team name too long (max %d)", MaxTeamNameLen)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.teams[name]; ok {
		return nil, fmt.Errorf("team %q already exists", name)
	}

	tf := &TeamFile{
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		LeadAgentID: leadAgentID,
		Members:     make([]TeamMember, 0),
	}

	if err := s.saveTeamLocked(tf); err != nil {
		return nil, fmt.Errorf("save team: %w", err)
	}

	s.teams[name] = tf
	return tf, nil
}

// GetTeam returns a team by name.
func (s *SwarmStore) GetTeam(name string) (*TeamFile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tf, ok := s.teams[name]
	return tf, ok
}

// ListTeams returns all teams.
func (s *SwarmStore) ListTeams() []*TeamFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*TeamFile, 0, len(s.teams))
	for _, tf := range s.teams {
		result = append(result, tf)
	}
	return result
}

// RemoveTeam deletes a team and all its data from disk.
func (s *SwarmStore) RemoveTeam(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.teams[name]; !ok {
		return fmt.Errorf("team %q not found", name)
	}

	dir := filepath.Join(s.teamsDir, name)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove team dir: %w", err)
	}

	delete(s.teams, name)
	return nil
}

// AddMember adds a teammate to a team and persists.
func (s *SwarmStore) AddMember(teamName string, member TeamMember) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tf, ok := s.teams[teamName]
	if !ok {
		return fmt.Errorf("team %q not found", teamName)
	}

	if member.AgentID == "" {
		member.AgentID = fmt.Sprintf("%s@%s", member.Name, teamName)
	}
	if member.Mode == "" {
		member.Mode = DefaultMode
	}
	if member.AgentType == "" {
		member.AgentType = DefaultAgentType
	}
	member.JoinedAt = time.Now()
	member.IsActive = true

	// Replace existing member with same AgentID if present.
	found := false
	for i, m := range tf.Members {
		if m.AgentID == member.AgentID {
			tf.Members[i] = member
			found = true
			break
		}
	}
	if !found {
		tf.Members = append(tf.Members, member)
	}

	return s.saveTeamLocked(tf)
}

// RemoveMember removes a teammate from a team and persists.
func (s *SwarmStore) RemoveMember(teamName, agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tf, ok := s.teams[teamName]
	if !ok {
		return fmt.Errorf("team %q not found", teamName)
	}

	for i, m := range tf.Members {
		if m.AgentID == agentID {
			tf.Members = append(tf.Members[:i], tf.Members[i+1:]...)
			return s.saveTeamLocked(tf)
		}
	}
	return fmt.Errorf("member %q not found in team %q", agentID, teamName)
}

// UpdateMemberStatus updates the IsActive and SessionID fields of a teammate.
func (s *SwarmStore) UpdateMemberStatus(teamName, agentID string, active bool, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tf, ok := s.teams[teamName]
	if !ok {
		return fmt.Errorf("team %q not found", teamName)
	}

	for i, m := range tf.Members {
		if m.AgentID == agentID {
			tf.Members[i].IsActive = active
			tf.Members[i].SessionID = sessionID
			return s.saveTeamLocked(tf)
		}
	}
	return fmt.Errorf("member %q not found in team %q", agentID, teamName)
}

// GetMember returns a teammate by agentID within a team.
func (s *SwarmStore) GetMember(teamName, agentID string) (*TeamMember, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tf, ok := s.teams[teamName]
	if !ok {
		return nil, false
	}
	for _, m := range tf.Members {
		if m.AgentID == agentID {
			return &m, true
		}
	}
	return nil, false
}

// SpawnTeammate spawns a new teammate agent via the configured executor.
func (s *SwarmStore) SpawnTeammate(config TeammateSpawnConfig) (*TeammateSpawnResult, error) {
	exec := s.Executor()
	if exec == nil {
		return nil, fmt.Errorf("no executor configured")
	}

	// Ensure team exists.
	if _, ok := s.GetTeam(config.TeamName); !ok {
		return nil, fmt.Errorf("team %q not found", config.TeamName)
	}

	// Register member in team.
	member := TeamMember{
		Name:         config.Name,
		AgentType:    DefaultAgentType,
		Model:        config.Model,
		Prompt:       config.Prompt,
		Color:        config.Color,
		PlanRequired: config.PlanRequired,
		IsActive:     true,
		Mode:         DefaultMode,
	}
	if err := s.AddMember(config.TeamName, member); err != nil {
		return nil, fmt.Errorf("add member: %w", err)
	}

	result, err := exec.Spawn(config)
	if err != nil {
		// Rollback member registration on failure.
		_ = s.RemoveMember(config.TeamName, member.AgentID)
		return nil, fmt.Errorf("spawn teammate: %w", err)
	}

	if !result.Success {
		_ = s.RemoveMember(config.TeamName, member.AgentID)
		return result, nil
	}

	// Update member with the actual agentID from the spawn result.
	_ = s.UpdateMemberStatus(config.TeamName, result.AgentID, true, result.TaskID)
	return result, nil
}

// KillTeammate stops a teammate agent.
func (s *SwarmStore) KillTeammate(teamName, agentID string) error {
	exec := s.Executor()
	if exec == nil {
		return fmt.Errorf("no executor configured")
	}

	if err := exec.Kill(agentID); err != nil {
		return fmt.Errorf("kill teammate: %w", err)
	}

	_ = s.UpdateMemberStatus(teamName, agentID, false, "")
	return nil
}

// SendTeammateMessage sends a message to a teammate's mailbox.
func (s *SwarmStore) SendTeammateMessage(teamName, agentID string, msg MailboxMessage) error {
	exec := s.Executor()
	if exec == nil {
		return fmt.Errorf("no executor configured")
	}
	return exec.SendMessage(agentID, msg)
}

// loadTeams loads all team configs from disk.
func (s *SwarmStore) loadTeams() error {
	entries, err := os.ReadDir(s.teamsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		configPath := filepath.Join(s.teamsDir, entry.Name(), ConfigFileName)
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}
		var tf TeamFile
		if err := json.Unmarshal(data, &tf); err != nil {
			continue
		}
		if tf.Name == "" {
			tf.Name = entry.Name()
		}
		if tf.Members == nil {
			tf.Members = make([]TeamMember, 0)
		}
		s.teams[tf.Name] = &tf
	}
	return nil
}

// saveTeamLocked persists a team to disk. Caller must hold s.mu.
func (s *SwarmStore) saveTeamLocked(tf *TeamFile) error {
	dir := filepath.Join(s.teamsDir, tf.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create team dir: %w", err)
	}

	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal team: %w", err)
	}

	configPath := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write team config: %w", err)
	}
	return nil
}

func shortID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
