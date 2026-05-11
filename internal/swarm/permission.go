package swarm

import (
	"fmt"
	"time"
)

// SubmitPermissionRequest registers a permission request and returns a channel
// that will receive the leader's response. This is the fast-path (in-memory).
func (s *SwarmStore) SubmitPermissionRequest(req *PermissionRequest) (<-chan PermissionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.permSeq++
	if req.ID == "" {
		req.ID = fmt.Sprintf("perm-%d-%s", s.permSeq, shortID())
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}
	if req.ResponseCh == nil {
		req.ResponseCh = make(chan PermissionResponse, 1)
	}

	s.permRequests[req.ID] = req
	return req.ResponseCh, nil
}

// ResolvePermissionRequest resolves a pending permission request with a response.
// Returns an error if the request is not found.
func (s *SwarmStore) ResolvePermissionRequest(requestID string, resp PermissionResponse) error {
	s.mu.Lock()
	req, ok := s.permRequests[requestID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("permission request %q not found", requestID)
	}
	delete(s.permRequests, requestID)
	s.mu.Unlock()

	resp.RequestID = requestID

	// Send response via the channel (non-blocking, with recovery).
	if req.ResponseCh != nil {
		select {
		case req.ResponseCh <- resp:
		default:
		}
	}
	return nil
}

// GetPendingRequests returns all pending permission requests for a team.
func (s *SwarmStore) GetPendingRequests(teamName string) []*PermissionRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*PermissionRequest
	for _, req := range s.permRequests {
		if req.TeamName == teamName {
			result = append(result, req)
		}
	}
	return result
}

// GetAllPendingRequests returns all pending permission requests across all teams.
func (s *SwarmStore) GetAllPendingRequests() []*PermissionRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*PermissionRequest, 0, len(s.permRequests))
	for _, req := range s.permRequests {
		result = append(result, req)
	}
	return result
}

// SubmitPermissionFallback writes a permission request to the leader's mailbox
// as a fallback when the in-memory channel is not available or times out.
func (s *SwarmStore) SubmitPermissionFallback(teamName, leaderName string, req *PermissionRequest) error {
	msg := MailboxMessage{
		From:      req.WorkerName,
		Text:      fmt.Sprintf("[PERMISSION_REQUEST id=%s tool=%s] %s\nInput: %v", req.ID, req.ToolName, req.Description, req.Input),
		Color:     req.Color,
		Timestamp: time.Now(),
	}
	return WriteToMailbox(teamName, leaderName, msg)
}

// LeaderMailboxPoll polls the leader's mailbox for any new permission responses.
// Used by teammates to check for permission responses via the fallback path.
func (s *SwarmStore) LeaderMailboxPoll(teamName, leaderName string, state *mailboxState) ([]MailboxEntry, error) {
	return ReadMailbox(teamName, leaderName, state)
}

// CleanupPermissionRequests removes all permission requests for a given worker.
func (s *SwarmStore) CleanupPermissionRequests(workerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, req := range s.permRequests {
		if req.WorkerID == workerID {
			if req.ResponseCh != nil {
				close(req.ResponseCh)
			}
			delete(s.permRequests, id)
		}
	}
}
