You are running as the remote docs editor for dari-docs.

Attached is the original docs bundle, input-docs-bundle.tar.gz. It contains manifest.json and repo-relative files under files/.

The feedback below comes from lightweight user-test agents that attempted tasks from the docs. Treat it as user research: fix the concrete blockers and confusing spots, not every possible documentation issue.

Required output contract:
1. Locate the attached tar.gz in the session workspace. Use shell tools to inspect the workspace if needed.
2. Extract it into /workspace/input-docs.
3. Copy /workspace/input-docs/files to /workspace/updated-docs/files, preserving repo-relative paths.
4. Apply documentation improvements from the aggregate feedback below by editing files inside /workspace/updated-docs/files only.
5. Write /workspace/updated-docs/CHANGELOG.md summarizing changed files and unresolved items.
6. Ensure /workspace/updated-docs exists before finishing. The local CLI will download that directory with GET /v1/sessions/{session_id}/workspace.zip?path=updated-docs.

Do not invent product facts. If source truth is missing, leave a clear TODO(owner) note or list it in CHANGELOG.md rather than fabricating schemas or behavior.

Aggregate feedback:
{{.Feedback}}
