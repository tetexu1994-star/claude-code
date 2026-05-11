package swarm

import (
	"context"
	"sync"
	"time"
)

// TeamMember represents a member of a swarm team.
type TeamMember struct {
	AgentID      string    `json:"agent_id"`
	Name         string    `json:"name"`
	AgentType    string    `json:"agent_type,omitempty"`
	Model        string    `json:"model,omitempty"`
	Prompt       string    `json:"prompt,omitempty"`
	Color        string    `json:"color,omitempty"`
	PlanRequired bool      `json:"plan_required"`
	JoinedAt     time.Time `json:"joined_at"`
	CWD          string    `json:"cwd"`
	SessionID    string    `json:"session_id,omitempty"`
	IsActive     bool      `json:"is_active"`
	Mode         string    `json:"mode"` // "default", "plan", "restricted"
}

// TeamFile is the persisted team configuration stored at
// ~/.tlaude-code/teams/<team_name>/config.json.
type TeamFile struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	LeadAgentID string       `json:"lead_agent_id"`
	Members     []TeamMember `json:"members"`
}

// MailboxMessage is a message exchanged between teammates via the file mailbox.
type MailboxMessage struct {
	From      string    `json:"from"`
	Text      string    `json:"text"`
	Color     string    `json:"color,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// MailboxEntry wraps a message with its read status and filename.
type MailboxEntry struct {
	Filename string         `json:"filename"`
	Read     bool           `json:"read"`
	Message  MailboxMessage `json:"message"`
}

// PermissionRequest is a permission request from a teammate to the leader.
type PermissionRequest struct {
	ID          string                 `json:"id"`
	ToolName    string                 `json:"tool_name"`
	ToolUseID   string                 `json:"tool_use_id"`
	Input       map[string]interface{} `json:"input"`
	Description string                 `json:"description"`
	WorkerID    string                 `json:"worker_id"`
	WorkerName  string                 `json:"worker_name"`
	Color       string                 `json:"color"`
	TeamName    string                 `json:"team_name"`
	CreatedAt   time.Time              `json:"created_at"`

	// ResponseCh is an in-memory channel for fast-path responses.
	// Not serialized.
	ResponseCh chan PermissionResponse `json:"-"`
}

// PermissionResponse is the leader's response to a permission request.
type PermissionResponse struct {
	RequestID    string                 `json:"request_id"`
	Approved     bool                   `json:"approved"`
	UpdatedInput map[string]interface{} `json:"updated_input,omitempty"`
	Feedback     string                 `json:"feedback,omitempty"`
}

// TeammateSpawnConfig holds configuration for spawning a new teammate agent.
type TeammateSpawnConfig struct {
	Name                  string
	TeamName              string
	Prompt                string
	Color                 string
	PlanRequired          bool
	Model                 string
	SystemPrompt          string
	SystemPromptMode      string // "default", "replace", "append"
	AllowedTools          []string
	AllowPermissionPrompts bool
}

// TeammateSpawnResult is the result of spawning a teammate.
type TeammateSpawnResult struct {
	Success bool
	AgentID string
	TaskID  string
	Cancel  context.CancelFunc
	Error   string
}

// TeammateExecutor defines the interface for executing teammate agents.
type TeammateExecutor interface {
	Type() string // "in-process" or "tmux"
	SetContext(ctx context.Context)
	Spawn(config TeammateSpawnConfig) (*TeammateSpawnResult, error)
	SendMessage(agentID string, msg MailboxMessage) error
	Terminate(agentID string) error
	Kill(agentID string) error
	IsActive(agentID string) (bool, error)
}

// SwarmStore is the central store for swarm team state.
// All methods are safe for concurrent use.
type SwarmStore struct {
	mu       sync.RWMutex
	teamsDir string
	teams    map[string]*TeamFile
	executor TeammateExecutor

	// Permission bridge: active permission requests awaiting leader response.
	permRequests map[string]*PermissionRequest
	permSeq      int
}
