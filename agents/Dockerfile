# Optional controller image for non-Modal deployments. Task sandboxes are separate.
FROM python:3.13-slim-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends openssh-client ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY harbor_runner.py runner_api.py ./
COPY docs-user-tester-agent ./docs-user-tester-agent
COPY docs-editor-agent ./docs-editor-agent
CMD ["python", "harbor_runner.py", "--host", "0.0.0.0", "--service", "tester"]
