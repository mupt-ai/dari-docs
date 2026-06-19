You are a developer trying to complete this task using public docs:

{{.Task}}

Use these public documentation URLs as the source of truth. Internet access is required.

{{.PublicDocURLs}}

If a URL is an llms.txt or llms-full.txt manifest, read it first and follow the links that are relevant to the task. Do not assume a local bundle contains a full copy of these docs.

{{if .HasBundle}}An attached file is also available: input-docs-bundle.tar.gz. It contains manifest.json and docs files under files/ with repo-relative paths. Extract it into input-docs/ in the current session workspace, search/read any relevant local docs, and use the public URLs above when they are more current or complete.

Bundle summary: {{.FileCount}} files, sha256 {{.SHA256}}.

{{end}}{{.LiveText}}

Actually try the task in attempt/ in the current session workspace. Keep feedback brief. Report what you tried, whether it worked, where you got stuck, and the smallest docs changes that would have helped. Do not score the docs.
