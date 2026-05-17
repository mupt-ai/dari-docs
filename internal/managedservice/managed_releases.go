package managedservice

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mupt-ai/dari-docs/internal/dari"
)

var (
	errNoActiveManagedAgentRelease = errors.New("no active managed agent release")
	errInvalidManagedAgentRelease  = errors.New("invalid managed agent release")
)

type managedAgentRelease struct {
	ID              string
	TesterAgentID   string
	TesterVersionID string
	EditorAgentID   string
	EditorVersionID string
	Source          string
	GitSHA          string
	GitHubRunID     string
	CreatedAt       time.Time
	Fallback        bool
}

type managedAgentReleaseResponse struct {
	ID              string `json:"id,omitempty"`
	TesterAgentID   string `json:"tester_agent_id"`
	TesterVersionID string `json:"tester_version_id"`
	EditorAgentID   string `json:"editor_agent_id"`
	EditorVersionID string `json:"editor_version_id"`
	Source          string `json:"source,omitempty"`
	GitSHA          string `json:"git_sha,omitempty"`
	GitHubRunID     string `json:"github_run_id,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	Fallback        bool   `json:"fallback,omitempty"`
}

type activateManagedAgentReleaseRequest struct {
	TesterVersionID string `json:"tester_version_id,omitempty"`
	EditorVersionID string `json:"editor_version_id,omitempty"`
	Source          string `json:"source,omitempty"`
	GitSHA          string `json:"git_sha,omitempty"`
	GitHubRunID     string `json:"github_run_id,omitempty"`
}

func (s *Server) handleManagedAgentRelease(w http.ResponseWriter, r *http.Request) {
	if !s.requireReleaseAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		release, err := s.loadManagedAgentRelease(r.Context())
		if err != nil {
			if errors.Is(err, errNoActiveManagedAgentRelease) {
				writeError(w, http.StatusNotFound, "no active managed agent release")
				return
			}
			writeLoggedError(w, http.StatusInternalServerError, "could not load managed agent release", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]managedAgentReleaseResponse{"release": managedAgentReleaseToResponse(release)})
	case http.MethodPost:
		var req activateManagedAgentReleaseRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		release, err := s.activateManagedAgentRelease(r.Context(), req)
		if err != nil {
			if errors.Is(err, errNoActiveManagedAgentRelease) {
				writeError(w, http.StatusBadRequest, "tester_version_id and editor_version_id are required for the first managed agent release")
				return
			}
			if errors.Is(err, errInvalidManagedAgentRelease) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeLoggedError(w, http.StatusBadGateway, "could not activate managed agent release", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]managedAgentReleaseResponse{"release": managedAgentReleaseToResponse(release)})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) requireReleaseAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !validBearerToken(r.Header.Get("Authorization"), s.cfg.ReleaseAdminToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

func validBearerToken(header string, expected string) bool {
	const prefix = "Bearer "
	if expected == "" || !strings.HasPrefix(header, prefix) {
		return false
	}
	supplied := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if supplied == "" {
		return false
	}
	expectedSum := sha256.Sum256([]byte(expected))
	suppliedSum := sha256.Sum256([]byte(supplied))
	return subtle.ConstantTimeCompare(expectedSum[:], suppliedSum[:]) == 1
}

func (s *Server) loadManagedAgentRelease(ctx context.Context) (managedAgentRelease, error) {
	release, ok, err := s.loadActiveManagedAgentRelease(ctx)
	if err != nil {
		return managedAgentRelease{}, err
	}
	if ok {
		return release, nil
	}
	if release, ok := s.configuredManagedAgentRelease(); ok {
		return release, nil
	}
	return managedAgentRelease{}, errNoActiveManagedAgentRelease
}

func (s *Server) loadActiveManagedAgentRelease(ctx context.Context) (managedAgentRelease, bool, error) {
	var release managedAgentRelease
	err := s.db.QueryRow(ctx, `
SELECT id, tester_agent_id, tester_version_id, editor_agent_id, editor_version_id,
       coalesce(source,''), coalesce(git_sha,''), coalesce(github_run_id,''), created_at
FROM managed_agent_releases
WHERE active
ORDER BY created_at DESC
LIMIT 1
`).Scan(
		&release.ID,
		&release.TesterAgentID,
		&release.TesterVersionID,
		&release.EditorAgentID,
		&release.EditorVersionID,
		&release.Source,
		&release.GitSHA,
		&release.GitHubRunID,
		&release.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return managedAgentRelease{}, false, nil
	}
	if err != nil {
		return managedAgentRelease{}, false, err
	}
	return release, true, nil
}

func (s *Server) configuredManagedAgentRelease() (managedAgentRelease, bool) {
	if s.cfg.ManagedTesterAgentID == "" || s.cfg.ManagedTesterVersionID == "" || s.cfg.ManagedEditorAgentID == "" || s.cfg.ManagedEditorVersionID == "" {
		return managedAgentRelease{}, false
	}
	return managedAgentRelease{
		TesterAgentID:   s.cfg.ManagedTesterAgentID,
		TesterVersionID: s.cfg.ManagedTesterVersionID,
		EditorAgentID:   s.cfg.ManagedEditorAgentID,
		EditorVersionID: s.cfg.ManagedEditorVersionID,
		Source:          "env_fallback",
		Fallback:        true,
	}, true
}

func (s *Server) activateManagedAgentRelease(ctx context.Context, req activateManagedAgentReleaseRequest) (managedAgentRelease, error) {
	current, err := s.loadManagedAgentRelease(ctx)
	if err != nil && !errors.Is(err, errNoActiveManagedAgentRelease) {
		return managedAgentRelease{}, err
	}
	if req.TesterVersionID == "" {
		req.TesterVersionID = current.TesterVersionID
	}
	if req.EditorVersionID == "" {
		req.EditorVersionID = current.EditorVersionID
	}
	if req.TesterVersionID == "" || req.EditorVersionID == "" {
		return managedAgentRelease{}, errNoActiveManagedAgentRelease
	}
	release := managedAgentRelease{
		ID:              "mar_" + randomToken(18),
		TesterAgentID:   s.cfg.ManagedTesterAgentID,
		TesterVersionID: strings.TrimSpace(req.TesterVersionID),
		EditorAgentID:   s.cfg.ManagedEditorAgentID,
		EditorVersionID: strings.TrimSpace(req.EditorVersionID),
		Source:          strings.TrimSpace(req.Source),
		GitSHA:          strings.TrimSpace(req.GitSHA),
		GitHubRunID:     strings.TrimSpace(req.GitHubRunID),
	}
	if release.Source == "" {
		release.Source = "manual"
	}
	if err := s.validateManagedAgentVersion(ctx, "tester_version_id", release.TesterAgentID, release.TesterVersionID); err != nil {
		return managedAgentRelease{}, err
	}
	if err := s.validateManagedAgentVersion(ctx, "editor_version_id", release.EditorAgentID, release.EditorVersionID); err != nil {
		return managedAgentRelease{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return managedAgentRelease{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE managed_agent_releases SET active=false WHERE active`); err != nil {
		return managedAgentRelease{}, err
	}
	err = tx.QueryRow(ctx, `
INSERT INTO managed_agent_releases (id, tester_agent_id, tester_version_id, editor_agent_id, editor_version_id, active, source, git_sha, github_run_id)
VALUES ($1, $2, $3, $4, $5, true, $6, $7, $8)
RETURNING created_at
`, release.ID, release.TesterAgentID, release.TesterVersionID, release.EditorAgentID, release.EditorVersionID, release.Source, release.GitSHA, release.GitHubRunID).Scan(&release.CreatedAt)
	if err != nil {
		return managedAgentRelease{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return managedAgentRelease{}, err
	}
	return release, nil
}

func (s *Server) validateManagedAgentVersion(ctx context.Context, fieldName, agentID, versionID string) error {
	if agentID == "" || versionID == "" {
		return fmt.Errorf("%w: %s is required", errInvalidManagedAgentRelease, fieldName)
	}
	detail, err := s.dari.GetAgentVersion(ctx, agentID, versionID)
	if err != nil {
		var httpErr *dari.HTTPError
		if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusBadRequest || httpErr.StatusCode == http.StatusNotFound) {
			return fmt.Errorf("%w: %s %s was not found for managed agent %s", errInvalidManagedAgentRelease, fieldName, versionID, agentID)
		}
		return fmt.Errorf("validate %s %s for agent %s: %w", fieldName, versionID, agentID, err)
	}
	if detail.Agent.ID != agentID || detail.Version.AgentID != agentID || detail.Version.ID != versionID {
		return fmt.Errorf("%w: %s %s does not belong to managed agent %s", errInvalidManagedAgentRelease, fieldName, versionID, agentID)
	}
	return nil
}

func managedAgentReleaseToResponse(release managedAgentRelease) managedAgentReleaseResponse {
	resp := managedAgentReleaseResponse{
		ID:              release.ID,
		TesterAgentID:   release.TesterAgentID,
		TesterVersionID: release.TesterVersionID,
		EditorAgentID:   release.EditorAgentID,
		EditorVersionID: release.EditorVersionID,
		Source:          release.Source,
		GitSHA:          release.GitSHA,
		GitHubRunID:     release.GitHubRunID,
		Fallback:        release.Fallback,
	}
	if !release.CreatedAt.IsZero() {
		resp.CreatedAt = release.CreatedAt.UTC().Format(time.RFC3339)
	}
	return resp
}
