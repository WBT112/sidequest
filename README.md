# sidequest
Play a tiny terminal game while waiting for a long-running command to finish.

Your computer has a task. So do you.

Sidequest turns terminal waiting time into a small playable break. Your command
keeps running in the main pane, while Snake starts in the lower pane when you
switch down and make the first move. When the command finishes, the game freezes
on your final score and Sidequest shows the command result.

sidequest -- sudo apt upgrade
sidequest -- docker build .
sidequest -- cargo build --release
sidequest -- ansible-playbook upgrade.yml

Sidequest is meant for the boring middle of long commands: builds, upgrades,
deployments and scripts that need to stay visible but do not need your constant
attention.

Requirements

Sidequest currently targets Linux terminals with tmux available in PATH.
The initial layout requires an interactive terminal of at least 80 columns by 24 rows.

Local QA

Run the complete normal local quality suite before committing:

```bash
./scripts/qa.sh
```

If Go is not available as `go` in `PATH`, point the script at the Go binary:

```bash
GO=/tmp/go/bin/go ./scripts/qa.sh
```

The script runs:

```bash
go fmt ./...
go vet ./...
go test ./...
go build ./...
```

Extended checks are available when useful:

```bash
./scripts/qa.sh --race
./scripts/qa.sh --cover
./scripts/qa.sh --race --cover
```

Pure unit tests do not require tmux or a graphical desktop. Tests that exercise a real tmux server detect whether tmux is installed and skip clearly when it is unavailable.

Goals

Sidequest should be:

simple to start;
safe to use with arbitrary commands;
usable through SSH;
independent of a graphical desktop;
lightweight and local;
fun without getting in the way;
easy to extend with additional games.
Non-goals

Sidequest is not intended to:

modify the command being executed;
replace a terminal multiplexer;
hide interactive prompts;
become a full terminal emulator.
