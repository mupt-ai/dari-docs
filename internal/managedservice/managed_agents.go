package managedservice

import "errors"

var errManagedAgentsNotConfigured = errors.New("managed agents are not configured")

// managedAgentVersionCompatibilityValue keeps legacy NOT NULL version columns
// populated while managed release/version pinning is removed without a
// destructive same-deploy schema change.
const managedAgentVersionCompatibilityValue = ""

type managedAgents struct {
	TesterAgentID string
	EditorAgentID string
}

func (s *Server) configuredManagedAgents() (managedAgents, error) {
	if s.cfg.ManagedTesterAgentID == "" || s.cfg.ManagedEditorAgentID == "" {
		return managedAgents{}, errManagedAgentsNotConfigured
	}
	return managedAgents{
		TesterAgentID: s.cfg.ManagedTesterAgentID,
		EditorAgentID: s.cfg.ManagedEditorAgentID,
	}, nil
}
