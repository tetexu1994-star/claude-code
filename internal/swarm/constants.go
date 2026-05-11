package swarm

import (
	"os"
	"path/filepath"
)

const (
	// DirName is the directory name under ~/.tlaude-code/ for swarm data.
	DirName = "teams"

	// MailboxDirName is the subdirectory for agent mailboxes.
	MailboxDirName = "mailbox"

	// ConfigFileName is the team configuration file name.
	ConfigFileName = "config.json"

	// DefaultAgentType is the default agent type for teammates.
	DefaultAgentType = "general"

	// DefaultMode is the default teammate mode.
	DefaultMode = "default"

	// MaxTeamNameLen is the maximum length for a team name.
	MaxTeamNameLen = 64
)

// TeamsDir returns the base directory for all team data.
func TeamsDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".tlaude-code", DirName)
}

// TeamDir returns the directory for a specific team.
func TeamDir(teamName string) string {
	return filepath.Join(TeamsDir(), teamName)
}

// TeamConfigPath returns the path to a team's config.json.
func TeamConfigPath(teamName string) string {
	return filepath.Join(TeamDir(teamName), ConfigFileName)
}

// MailboxDir returns the base mailbox directory for a team.
func MailboxDir(teamName string) string {
	return filepath.Join(TeamDir(teamName), MailboxDirName)
}

// AgentMailboxDir returns the mailbox directory for a specific agent in a team.
func AgentMailboxDir(teamName, agentName string) string {
	return filepath.Join(MailboxDir(teamName), agentName)
}
