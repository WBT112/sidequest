# sidequest

Play Snake (other games maybe later) while a long-running terminal command keeps working.

Sidequest runs your command in one tmux pane and focuses a small Snake game in
another. The command stays visible, the game starts on your first move, and when
the command finishes you can keep the same Snake round going.

## Installation

Sidequest currently supports Linux `amd64` and `arm64`. Windows users should run
Sidequest inside WSL 2. Native macOS is not supported yet.

Sidequest requires `tmux` at runtime:

```bash
sudo apt install tmux
sudo dnf install tmux
```

### Quick Install With Curl

Use this only after the repository and releases are public:

```bash
curl -fsSL https://raw.githubusercontent.com/WBT112/sidequest/main/install.sh | sh
```

Safer inspect-before-run variant:

```bash
curl -fsSLO https://raw.githubusercontent.com/WBT112/sidequest/main/install.sh
less install.sh
sh install.sh
```

The installer downloads the matching GitHub Release archive, verifies
`checksums.txt`, and installs to `$HOME/.local/bin` by default. To install
elsewhere:

```bash
SIDEQUEST_INSTALL_DIR=/usr/local/bin sh install.sh
```

### Private Repository Installation

While this repository is private, use an authenticated GitHub path:

```bash
gh auth login
gh release download v0.1.0 --repo WBT112/sidequest --pattern 'sidequest_*_linux_amd64.tar.gz'
gh release download v0.1.0 --repo WBT112/sidequest --pattern checksums.txt
```

Token-based downloads are supported by `install.sh` through `GITHUB_TOKEN` or
`GH_TOKEN`, but avoid placing tokens directly in shell history.

### Debian/Ubuntu Package

```bash
sudo apt install ./sidequest_0.1.0_linux_amd64.deb
sidequest --version
sudo apt remove sidequest
```

### Fedora/RHEL Package

```bash
sudo dnf install ./sidequest_0.1.0_linux_amd64.rpm
sidequest --version
sudo dnf remove sidequest
```

### Windows Via WSL 2

Install and run Sidequest inside a Linux distribution under WSL 2, not from
PowerShell or `cmd.exe`:

```bash
sudo apt update
sudo apt install tmux curl ca-certificates
curl -fsSL https://raw.githubusercontent.com/WBT112/sidequest/main/install.sh | sh
sidequest --version
```

If the repository is still private, replace the anonymous curl step with the
authenticated GitHub installation path above. PATH setup happens inside WSL, not
in the Windows PATH.

### macOS

Native macOS is not supported yet. The application currently validates Linux
terminals only, and release artifacts are built for Linux. macOS support needs a
dedicated tmux/preflight test matrix before it is advertised.

### Build From Source

```bash
go build -o sidequest ./cmd/sidequest
```

### Verify

```bash
command -v sidequest
sidequest --version
```

If `$HOME/.local/bin` is not on `PATH`, add it in your shell profile:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

## Quick Start

```bash
sidequest -- codex
sidequest -- claude "Run the test suite, fix any failures, and summarize the changes."
sidequest -- gemini
sidequest -- aider --message "Refactor the parser and run tests."
sidequest --mode quest -- make test
```

Try it with a harmless demo workload:

```bash
sidequest -- bash -c 'for i in {1..60}; do printf "working step %02d/60\n" "$i"; sleep 1; done'
```

## Gameplay

- `WASD` or arrow keys move Snake.
- `F9` hides or restores Sidequest while the command keeps running.
- `F12` switches between Snake and the command pane.
- Snake focus-pauses while the command pane is active and resumes when the game
  pane is active again, unless you paused manually.
- `F10` detaches back to your shell. If the command is still running, Sidequest
  prints the `sidequest attach <id>` command.
- `R` restarts Snake after a round over while the command keeps running.
- After the command finishes, `C` continues the current round and `Q` finalizes
  and quits.

Classic mode keeps Snake simple and adds Command Heat: the longer you actively
play, the faster Snake gets and the more food is worth. Time spent in the
command pane or on pause does not raise Heat. After the command has finished,
Heat stays frozen at the reached level while the round can continue.

Quest mode adds combo scoring, one mission per command, Golden Bytes, random
arena pickups and other stuff.

For complete controls and behavior details, use:

```bash
man sidequest
```

## Sessions and History

Runtime sessions:

```bash
sidequest list
sidequest attach <session-id>
```

Stored run history:

```bash
sidequest runs
sidequest show last
sidequest output last
sidequest purge <run-id>
```

Finished runs keep visible command-pane output under
`${XDG_STATE_HOME:-$HOME/.local/state}/sidequest/runs/`. Sidequest stores result
metadata and pane output, not the command or argument list. Terminal output may
still contain sensitive data.

Game statistics and separate Classic/Quest TOP 5 lists are stored locally in
`${XDG_STATE_HOME:-$HOME/.local/state}/sidequest/game-stats.json`.

## Requirements

Sidequest targets Linux terminals and requires `tmux` in `PATH`.

## Development

Run the normal local quality suite before committing:

```bash
./scripts/qa.sh
```

If Go is not available as `go` in `PATH`, point the script at the Go binary:

```bash
GO=/usr/local/go/bin/go ./scripts/qa.sh
```

Extended checks:

```bash
./scripts/qa.sh --race
./scripts/qa.sh --cover
./scripts/qa.sh --race --cover
```

## Scope

Sidequest is meant for builds, upgrades, deployments and scripts that should stay
visible but do not need constant attention. It does not modify the wrapped
command, replace tmux, hide interactive prompts or act as a full terminal
emulator.
