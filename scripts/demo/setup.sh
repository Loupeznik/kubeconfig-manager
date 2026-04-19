#!/usr/bin/env bash
#
# scripts/demo/setup.sh
#
# Seeds ./.temp/demo/ with a clean sandbox for recording the README demo.
# Creates a fake $HOME with its own ~/.kube containing two dummy kubeconfigs,
# a fake XDG state dir with a pre-seeded kcm palette + alerts, a minimal
# zsh rc that loads starship with the kcm custom module, and stub kubectl +
# helm binaries. Because $HOME is pointed at the sandbox, every `kcm ...`
# command works without --dir and never reaches your real ~/.kube/.
#
# Requires: go, zsh, starship, asciinema (for the actual recording).
#
# Usage: ./scripts/demo/setup.sh
# Then:  . .temp/demo/env.sh
#        asciinema rec docs/images/demo.cast --command zsh --overwrite
#
# Idempotent — re-running wipes .temp/demo and starts fresh.

set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

# Belt-and-suspenders: snapshot the user's real zsh + starship configs before
# we touch anything. The setup script points HOME at a sandbox and never
# writes to the real ~/.zshrc, but if that ever drifted (or a future editor
# of this script got sloppy) the backup means no data loss. Backups live
# under .temp/backups/ so they survive re-runs of setup.sh (which wipes
# .temp/demo/). Remove the backup dir manually when you no longer want them.
BACKUP_DIR="$ROOT/.temp/backups"
mkdir -p "$BACKUP_DIR"
stamp=$(date +%Y%m%d-%H%M%S)
for target in "$HOME/.zshrc" "$HOME/.config/starship.toml"; do
	if [ -f "$target" ]; then
		dest="$BACKUP_DIR/$(basename "$target").$stamp"
		cp -p "$target" "$dest"
		echo "[demo] backed up $target -> $dest"
	fi
done

DEMO_DIR="$ROOT/.temp/demo"
HOME_DIR="$DEMO_DIR/home"
KUBE_DIR="$HOME_DIR/.kube"
XDG_CFG="$HOME_DIR/.config"
XDG_DATA="$HOME_DIR/.local/share"
BIN_DIR="$DEMO_DIR/bin"
VALUES_DIR="$HOME_DIR/work/helm-values"

rm -rf "$DEMO_DIR"
mkdir -p "$KUBE_DIR" "$XDG_CFG/kubeconfig-manager" "$XDG_DATA" "$BIN_DIR" "$VALUES_DIR"

echo "[demo] building fresh kcm binary"
go build -o "$BIN_DIR/kcm" ./cmd/kubeconfig-manager
# The kcm() shell hook calls `kubeconfig-manager` by default. Symlink the
# sandbox binary under that name so the hook resolves to OUR build, not
# whatever globally-installed kubeconfig-manager the user may have — which
# would otherwise silently win via $PATH and mask local fixes.
ln -sf kcm "$BIN_DIR/kubeconfig-manager"

echo "[demo] writing dummy kubeconfigs into $KUBE_DIR"
cat > "$KUBE_DIR/prod.yaml" <<'EOF'
apiVersion: v1
kind: Config
current-context: k8s-prod-01
clusters:
  - name: k8s-prod-01-cluster
    cluster:
      server: https://prod-01.example.com
  - name: k8s-prod-02-cluster
    cluster:
      server: https://prod-02.example.com
contexts:
  - name: k8s-prod-01
    context:
      cluster: k8s-prod-01-cluster
      user: admin
  - name: k8s-prod-02
    context:
      cluster: k8s-prod-02-cluster
      user: admin
users:
  - name: admin
    user:
      token: redacted
EOF

cat > "$KUBE_DIR/test.yaml" <<'EOF'
apiVersion: v1
kind: Config
current-context: k8s-test-01
clusters:
  - name: k8s-test-01-cluster
    cluster:
      server: https://test-01.example.com
contexts:
  - name: k8s-test-01
    context:
      cluster: k8s-test-01-cluster
      user: admin
users:
  - name: admin
    user:
      token: redacted
EOF

echo "[demo] writing sample helm values trees"
for name in k8s-prod-01 k8s-prod-02 k8s-test-01; do
	mkdir -p "$VALUES_DIR/clusters/$name"
	printf 'name: myapp\nreplicas: 3\n' > "$VALUES_DIR/clusters/$name/values.yaml"
done

echo "[demo] writing stub kubectl + helm"
cat > "$BIN_DIR/kubectl" <<'EOF'
#!/bin/sh
echo "kubectl stub: $@"
EOF
cat > "$BIN_DIR/helm" <<'EOF'
#!/bin/sh
echo "helm stub: $@"
EOF
chmod +x "$BIN_DIR/kubectl" "$BIN_DIR/helm"

echo "[demo] writing sandbox .zshrc + starship.toml"
cat > "$HOME_DIR/.zshrc" <<'EOF'
# Sandbox .zshrc for the asciinema demo — intentionally minimal.
# Loaded because the recording sets HOME (and therefore zsh's default dotfile
# location) to the sandbox dir, so your real ~/.zshrc never runs.

setopt PROMPT_SUBST
autoload -Uz colors && colors
bindkey -e

# In-memory history only.
HISTSIZE=100
SAVEHIST=0

# Starship prompt with the kcm custom module. The kcm shell hook block is
# appended below by setup.sh — it supplies the kcm() function + kubectl/helm
# aliases so the guard wraps plain `kubectl`/`helm` calls during the demo.
eval "$(starship init zsh)"
EOF

# Append the kcm shell hook to the sandbox .zshrc so plain kubectl/helm route
# through the guard and `kcm use`/`kcm tui` eval their export lines.
"$BIN_DIR/kcm" install-shell-hook \
	--shell=zsh \
	--rc="$HOME_DIR/.zshrc" \
	--alias-helm \
	>/dev/null

cat > "$XDG_CFG/starship.toml" <<'EOF'
add_newline = false

format = """
$directory\
${custom.kcm}\
$git_branch\
$character
"""

[directory]
truncation_length = 2
style = "bold cyan"

[git_branch]
symbol = ""
format = "on [$branch]($style) "
style = "purple"

[character]
success_symbol = "[❯](bold green)"
error_symbol = "[❯](bold red)"

[custom.kcm]
command = "kcm starship"
when = "kcm starship | grep -q ."
format = "[⎈ $output]($style) "
style = "bold yellow"
EOF

echo "[demo] writing env.sh"
cat > "$DEMO_DIR/env.sh" <<EOF
# source me: . $DEMO_DIR/env.sh
# Shadow any outer kcm() function or kubectl/helm alias.
unset -f kcm 2>/dev/null || true
unalias kcm 2>/dev/null || true
unalias kubectl 2>/dev/null || true
unalias helm 2>/dev/null || true

export HOME="$HOME_DIR"
export XDG_CONFIG_HOME="$XDG_CFG"
export XDG_DATA_HOME="$XDG_DATA"
export KUBECONFIG="$KUBE_DIR/prod.yaml"
export PATH="$BIN_DIR:\$PATH"

# Force a TERM that every system's terminfo knows about. asciinema records
# what's on screen, so the actual terminal emulator (alacritty, kitty, wezterm,
# …) is irrelevant here — but \`clear\`/tput need terminfo lookups to succeed.
export TERM=xterm-256color

# zsh will pick up the sandbox .zshrc because HOME is set (or set ZDOTDIR
# explicitly if your zsh is configured with a different dotfile location).
export ZDOTDIR="\$HOME"

# Start the working directory somewhere friendly for the demo.
cd "\$HOME/work" 2>/dev/null || cd "\$HOME"
EOF

echo "[demo] seeding state (tag palette + per-entry tags + alerts)"
# shellcheck disable=SC1090
. "$DEMO_DIR/env.sh"
kcm tag palette add prod staging eu us critical >/dev/null
kcm tag add prod prod eu critical >/dev/null
kcm tag add test staging                        >/dev/null
kcm alert enable prod                           >/dev/null

cat <<EOF

[demo] setup complete. To start recording:

  . $DEMO_DIR/env.sh
  asciinema rec docs/images/demo.cast \\
    --title 'kubeconfig-manager — prod/test guard in action' \\
    --command zsh \\
    --idle-time-limit 2 \\
    --overwrite

  # then follow the steps in scripts/demo/walkthrough.md

EOF
