package swarm

import (
	"context"
	"fmt"
	"time"

	"github.com/tetexu/tlaude-code/internal/agent"
)

// runTeammate is the goroutine that executes a teammate agent.
// It uses the AgentRuntime to run the agent asynchronously, polls for completion,
// and writes the result to the leader's mailbox.
func (b *InProcessBackend) runTeammate(ctx context.Context, agentID string, def *agent.AgentDefinition, config TeammateSpawnConfig) {
	teamName, agentName := parseAgentID(agentID)
	logger := b.logger.With("agent_id", agentID, "team", teamName)
	logger.Info("teammate starting")

	// Ensure cleanup on exit.
	defer func() {
		if r := recover(); r != nil {
			logger.Error("teammate panicked", "panic", r)
		}
		b.mu.Lock()
		delete(b.activeAgents, agentID)
		delete(b.mailboxStates, agentID)
		b.mu.Unlock()

		// Notify leader of completion via mailbox.
		notifyMsg := MailboxMessage{
			From:      agentName,
			Text:      fmt.Sprintf("[TEAMMATE_COMPLETE] Agent %q has finished.", agentID),
			Color:     config.Color,
			Timestamp: time.Now(),
		}
		_ = WriteToMailbox(teamName, "leader", notifyMsg)

		logger.Info("teammate finished")
	}()

	// Build the agent prompt. If plan mode is required, prepend plan instructions.
	prompt := config.Prompt
	if config.PlanRequired {
		prompt = fmt.Sprintf(
			"IMPORTANT: You must enter plan mode before implementing any changes. "+
				"Use the EnterPlanMode tool to create a plan, get approval, then use ExitPlanMode to proceed.\n\n%s",
			prompt,
		)
	}

	// Resolve options for the agent run.
	opts := &agent.RunOptions{
		SessionModel:    config.Model,
		SessionProvider: def.Provider,
	}

	// Start the agent asynchronously using the existing AgentRuntime.
	runID, err := b.runtime.RunAgentAsync(ctx, def, prompt, opts)
	if err != nil {
		logger.Error("failed to start agent", "error", err)
		_ = WriteToMailbox(teamName, "leader", MailboxMessage{
			From:      agentName,
			Text:      fmt.Sprintf("[TEAMMATE_ERROR] Failed to start: %v", err),
			Color:     config.Color,
			Timestamp: time.Now(),
		})
		return
	}

	logger.Info("teammate agent started", "run_id", runID)

	// Poll for agent completion and handle mailbox messages.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("teammate context cancelled, stopping agent")
			_ = b.runtime.StopAgent(runID)
			return

		case <-ticker.C:
			// Check if agent has completed.
			run, ok := b.runtime.GetAgent(runID)
			if !ok {
				logger.Info("teammate agent not found in runtime (may have completed)")
				return
			}

			state := run.GetState()
			switch state {
			case agent.AgentCompleted:
				result := run.Result
				logger.Info("teammate agent completed", "result_len", len(result))
				_ = WriteToMailbox(teamName, "leader", MailboxMessage{
					From:      agentName,
					Text:      fmt.Sprintf("[TEAMMATE_RESULT]\nAgent: %s\nPrompt: %s\n\nResult:\n%s", agentID, config.Prompt, result),
					Color:     config.Color,
					Timestamp: time.Now(),
				})
				return

			case agent.AgentFailed:
				logger.Error("teammate agent failed", "error", run.Error)
				_ = WriteToMailbox(teamName, "leader", MailboxMessage{
					From:      agentName,
					Text:      fmt.Sprintf("[TEAMMATE_ERROR] %s", run.Error),
					Color:     config.Color,
					Timestamp: time.Now(),
				})
				return

			case agent.AgentCancelled:
				logger.Info("teammate agent cancelled")
				return
			}

			// Check for incoming messages from the leader.
			entries, err := b.ReadMessages(agentID)
			if err != nil {
				logger.Warn("failed to read messages", "error", err)
				continue
			}

			for _, entry := range entries {
				if entry.Read {
					continue
				}
				_ = b.MarkMessageRead(agentID, entry.Filename)

				// Handle special messages from the leader.
				switch entry.Message.Text {
				case "/terminate":
					logger.Info("teammate received terminate command")
					_ = b.runtime.StopAgent(runID)
					return
				case "/status":
					// Respond with current status.
					_ = WriteToMailbox(teamName, "leader", MailboxMessage{
						From:  agentName,
						Text:  fmt.Sprintf("[STATUS] agent_id=%s state=%s", agentID, state),
						Color: config.Color,
					})
				default:
					logger.Debug("teammate received message", "from", entry.Message.From, "text_len", len(entry.Message.Text))
				}
			}
		}
	}
}
