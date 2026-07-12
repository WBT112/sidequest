# sidequest

Play Snake while a long-running terminal command keeps working.

Sidequest runs your command in one tmux pane and focuses a small Snake game in
another. The command stays visible, the game starts on your first move, and when
the command finishes Sidequest freezes the round with the result and your local
TOP 5.

## Quick Start

```bash
sidequest -- codex
sidequest -- claude
sidequest -- docker build .
sidequest --mode quest -- make test
sidequest -- sudo apt upgrade
```

Interactive developer CLIs are treated as one running command, so Snake remains
available until the tool exits.

Pass an initial task to Claude Code when it should start working immediately:

```bash
sidequest -- claude "Run the test suite, fix any failures, and summarize the changes."
```

Try it with a harmless demo workload:

```bash
sidequest -- bash -c 'for i in {1..60}; do printf "working step %02d/60\n" "$i"; sleep 1; done'
```

## Shell shortcuts

For a dedicated shortcut, add an alias to the startup file of your shell.
For Bash, add this to `~/.bashrc`; for Zsh, add it to `~/.zshrc`:

```bash
alias dbuild='sidequest -- docker build'
```

Reload the file or open a new terminal:

```bash
source ~/.bashrc  # use ~/.zshrc for Zsh
dbuild .
dbuild --no-cache -t example:dev .
```

A normal alias cannot transparently replace only the two-word command
`docker build` while leaving commands such as `docker ps` unchanged. Use a shell
function for that behavior:

```bash
docker() {
    if [ "${1:-}" = "build" ]; then
        shift
        sidequest -- docker build "$@"
    else
        command docker "$@"
    fi
}
```

Put the function in `~/.bashrc` or `~/.zshrc` and reload the file. Afterwards,
`docker build ...` starts through Sidequest automatically, while other Docker
subcommands still call Docker directly:

```bash
docker build -t example:dev .  # runs through Sidequest
docker ps                      # runs normally
```

To disable the wrapper for the current shell, run `unset -f docker`. Remove the
function from the shell startup file to disable it permanently.

## Gameplay

- `WASD` or arrow keys move Snake.
- `F12` switches between Snake and the command pane.
- Snake focus-pauses while the command pane is active and resumes when the game
  pane is active again, unless you paused manually.
- `F10` detaches back to your shell. If the command is still running, Sidequest
  prints the `sidequest attach <id>` command.
- `R` restarts Snake after a round over while the command keeps running.
- `Q` leaves the game pane; finished sessions can then be cleaned up.

Classic mode keeps Snake simple and adds Command Heat: the longer you actively
play, the faster Snake gets and the more food is worth. Time spent in the
command pane or on pause does not raise Heat. Qualifying round results open a
quick arcade-style name entry before they land in the local TOP 5.

Quest mode adds combo scoring, one mission per command, Golden Bytes, random
arena pickups, completion bonuses and its own TOP 5.

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
