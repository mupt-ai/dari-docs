package managedservice

import (
	"errors"
	"strings"
)

type persistedErrorCode string

const (
	persistedErrRunFailed              persistedErrorCode = "run_failed"
	persistedErrBundleStageFailed      persistedErrorCode = "bundle_stage_failed"
	persistedErrBundleUploadFailed     persistedErrorCode = "bundle_upload_failed"
	persistedErrBundleUploadIncomplete persistedErrorCode = "bundle_upload_incomplete"
	persistedErrRunQueueFailed         persistedErrorCode = "run_queue_failed"

	persistedErrRuntimeSecretsLoadFailed persistedErrorCode = "runtime_secrets_load_failed"
	persistedErrSessionCreateFailed      persistedErrorCode = "session_create_failed"
	persistedErrSessionMessageFailed     persistedErrorCode = "session_message_failed"
	persistedErrSessionFailed            persistedErrorCode = "session_failed"
	persistedErrSessionPollFailed        persistedErrorCode = "session_poll_failed"
	persistedErrSessionPollStale         persistedErrorCode = "session_poll_stale"
	persistedErrSessionStale             persistedErrorCode = "session_stale"

	persistedErrAgentDeployFailed              persistedErrorCode = "agent_deploy_failed"
	persistedErrAgentDeployStale               persistedErrorCode = "agent_deploy_stale"
	persistedErrAgentDeployPublishTesterFailed persistedErrorCode = "agent_deploy_publish_tester_failed"
	persistedErrAgentDeployPublishEditorFailed persistedErrorCode = "agent_deploy_publish_editor_failed"
	persistedErrAgentDeployUpdateFailed        persistedErrorCode = "agent_deploy_update_failed"
	persistedErrAgentDeployApplyFailed         persistedErrorCode = "agent_deploy_apply_failed"
)

var validPersistedErrorCodes = map[persistedErrorCode]bool{
	// Run setup / bundle handling.
	persistedErrRunFailed:              true,
	persistedErrBundleStageFailed:      true,
	persistedErrBundleUploadFailed:     true,
	persistedErrBundleUploadIncomplete: true,
	persistedErrRunQueueFailed:         true,

	// Dari session execution.
	persistedErrRuntimeSecretsLoadFailed: true,
	persistedErrSessionCreateFailed:      true,
	persistedErrSessionMessageFailed:     true,
	persistedErrSessionFailed:            true,
	persistedErrSessionPollFailed:        true,
	persistedErrSessionPollStale:         true,
	persistedErrSessionStale:             true,

	// Managed agent deployment.
	persistedErrAgentDeployFailed:              true,
	persistedErrAgentDeployStale:               true,
	persistedErrAgentDeployPublishTesterFailed: true,
	persistedErrAgentDeployPublishEditorFailed: true,
	persistedErrAgentDeployUpdateFailed:        true,
	persistedErrAgentDeployApplyFailed:         true,
}

type persistedCodeError struct {
	code persistedErrorCode
	err  error
}

func (e *persistedCodeError) Error() string {
	if e.err == nil {
		return string(e.code)
	}
	return e.err.Error()
}

func (e *persistedCodeError) Unwrap() error {
	return e.err
}

func withPersistedErrorCode(code persistedErrorCode, err error) error {
	if err == nil {
		return nil
	}
	return &persistedCodeError{code: code, err: err}
}

func persistedErrorString(code persistedErrorCode) string {
	if !validPersistedErrorCodes[code] {
		return string(persistedErrRunFailed)
	}
	return string(code)
}

func persistedErrorCodeFromString(raw string, fallback persistedErrorCode) persistedErrorCode {
	code := persistedErrorCode(strings.TrimSpace(raw))
	if validPersistedErrorCodes[code] {
		return code
	}
	if validPersistedErrorCodes[fallback] {
		return fallback
	}
	return persistedErrRunFailed
}

func persistedErrorCodeFromError(err error, fallback persistedErrorCode) persistedErrorCode {
	var coded *persistedCodeError
	if errors.As(err, &coded) && validPersistedErrorCodes[coded.code] {
		return coded.code
	}
	if validPersistedErrorCodes[fallback] {
		return fallback
	}
	return persistedErrRunFailed
}
