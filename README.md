# sidequest
Play a tiny terminal game while waiting for a long-running command to finish.

Your computer has a task. So do you.

Sidequest turns terminal waiting time into a small playable break. Your command
keeps running in the main pane, while Snake is focused in the lower pane and
starts on your first move. When the command finishes, the game freezes on your
final score and Sidequest shows the command result.

sidequest -- sudo apt upgrade
sidequest -- docker build .
sidequest -- cargo build --release
sidequest -- ansible-playbook upgrade.yml

Try it with a harmless demo workload:

```bash
sidequest -- bash -c 'for i in {1..60}; do printf "working step %02d/60\n" "$i"; sleep 1; done'
```

The upper pane shows visible progress. Move with the arrow keys or `WASD`, press
`R` to restart after a round over, use `F12` to switch to the command pane, and
return to your shell with `F10`.

`F10` detaches from tmux and keeps the Sidequest session listed for later attach.
`Q` leaves the game pane; once the command is finished, Sidequest can clean up
the session.

Finished runs keep the visible command-pane output locally so you can inspect it
after tmux is gone. After cleanup, Sidequest prints the saved output path and the
matching `sidequest output <run-id>` command in your shell:

```bash
sidequest runs
sidequest show last
sidequest output last
sidequest purge <run-id>
```

Run history is stored under
`${XDG_STATE_HOME:-$HOME/.local/state}/sidequest/runs/`. It only stores visible
pane output plus result metadata, not the command or argument list. Terminal
output can still contain sensitive data such as tokens, environment values, file
contents or application data.

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
