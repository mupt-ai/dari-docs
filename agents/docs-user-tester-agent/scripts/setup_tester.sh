#!/usr/bin/env bash
set -euo pipefail

# Install Homebrew/Linuxbrew only. Do NOT install Dari here; installing the Dari
# CLI via `brew install mupt-ai/tap/dari` is part of the docs/dev-ex test.
REAL_BREW=/home/linuxbrew/.linuxbrew/bin/brew
WRAPPER=/usr/local/bin/brew

if [ -x "$REAL_BREW" ]; then
  ln -sf "$REAL_BREW" "$WRAPPER" || true
  "$REAL_BREW" --version || true
  exit 0
fi

apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
  build-essential curl file git procps ca-certificates ruby-full sudo

if ! id linuxbrew >/dev/null 2>&1; then
  useradd -m -s /bin/bash linuxbrew
fi

mkdir -p /home/linuxbrew/.linuxbrew
chown -R linuxbrew:linuxbrew /home/linuxbrew

# Run the official installer as a non-root user. Homebrew refuses to install or
# run as root, so tests should also exercise it as a normal Linuxbrew user.
su - linuxbrew -c 'NONINTERACTIVE=1 CI=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"'

if [ ! -x "$REAL_BREW" ]; then
  echo "Homebrew install completed but $REAL_BREW was not found" >&2
  exit 1
fi

# Expose a root-callable wrapper. Pi/bash tools may run as root; this wrapper
# runs brew itself as the linuxbrew user while preserving arguments. It still
# leaves installing Dari to the user/docs task.
cat > "$WRAPPER" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exec su - linuxbrew -c "$(printf '%q ' /home/linuxbrew/.linuxbrew/bin/brew "$@")"
EOF
chmod +x "$WRAPPER"

"$REAL_BREW" --version || true
