// Package coordinator implements Coordinator Mode for Tlaude Code.
//
// Coordinator mode transforms an agent into an orchestrator that delegates
// work to worker sub-agents. The coordinator's job is to:
// research → synthesize → implement (via workers) → verify (via workers).
// Workers run asynchronously and report back via task notifications.
package coordinator

import "os"

// CoordinatorEnvVar is the environment variable that enables coordinator mode.
const CoordinatorEnvVar = "TLAUDE_CODE_COORDINATOR_MODE"

// IsCoordinatorMode checks whether coordinator mode is enabled.
// Returns true when TLAUDE_CODE_COORDINATOR_MODE=1 is set.
func IsCoordinatorMode() bool {
	return os.Getenv(CoordinatorEnvVar) == "1"
}

// MatchSessionMode checks if the stored session mode matches the current
// coordinator mode state.
//
// If mismatched, it flips the environment variable so IsCoordinatorMode()
// returns the correct value for the resumed session. Returns a warning
// message and true if the mode was switched, or an empty string and false
// if no switch was needed.
func MatchSessionMode(sessionMode string) (warning string, switched bool) {
	// No stored mode — do nothing
	if sessionMode == "" {
		return "", false
	}

	currentIsCoordinator := IsCoordinatorMode()
	sessionIsCoordinator := sessionMode == "coordinator"

	if currentIsCoordinator == sessionIsCoordinator {
		return "", false
	}

	// Flip the env var
	if sessionIsCoordinator {
		os.Setenv(CoordinatorEnvVar, "1")
		return "Entered coordinator mode to match resumed session.", true
	}

	os.Unsetenv(CoordinatorEnvVar)
	return "Exited coordinator mode to match resumed session.", true
}
