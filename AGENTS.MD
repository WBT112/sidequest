# AGENTS.md

## Project overview

Sidequest is a Linux CLI application that runs a long-running command in one terminal pane while the user plays a small terminal game in another pane.

Example:

```bash
sidequest -- sudo apt upgrade
sidequest -- docker build .
sidequest -- cargo build --release
```

The command remains visible and interactive. Sidequest must never hide prompts, rewrite commands, upload data, or require elevated privileges for itself.

Tagline:

> Your computer has a task. So do you.

## Current project status

The repository is in the initial MVP implementation phase.

Before making changes:

1. Read the relevant GitHub issue completely.
2. Respect the dependencies listed in that issue.
3. Do not implement features assigned to later issues unless they are strictly required by the current task.
4. Keep changes small and focused.

The initial implementation order is represented by GitHub issues #1 through #11.

## Local QA

Before committing any code change, run the normal local QA suite:

```bash
./scripts/qa.sh
```

If Go is not available as `go` in `PATH`, set `GO` explicitly:

```bash
GO=/tmp/go/bin/go ./scripts/qa.sh
```

The script is intentionally small and runs the mandatory local checks:

```bash
go fmt ./...
go vet ./...
go test ./...
go build ./...
```

Use extended checks when the change touches concurrency, terminal handling or broad shared behavior:

```bash
./scripts/qa.sh --race
./scripts/qa.sh --cover
```

Pure unit tests must not require tmux, root privileges, network access or a graphical desktop. Tests that require a real tmux binary must call `t.Skip` with a clear message when tmux is unavailable.

## Technology decisions

These decisions are already made:

* Language: Go
* Supported platform for the MVP: Linux
* Supported architectures: amd64 and arm64
* Terminal pane management: tmux
* Terminal game rendering: tcell
* First game: Snake
* Distribution formats: standalone binary, DEB and RPM
* Release tooling: GoReleaser and nFPM
* Runtime dependency: tmux
* License: MIT

Do not replace these technologies without an explicit architecture decision.

## Public CLI contract

The primary invocation format is:

```bash
sidequest [sidequest options] -- <command> [arguments...]
```

Example:

```bash
sidequest --game snake -- docker build --no-cache .
```

Everything after `--` belongs to the requested command.

Sidequest must pass the executable and arguments as separate values. It must not join them into a shell command string.

## Command execution rules

Commands must be executed directly using Go `os/exec`.

Do not use:

* implicit `sh -c`;
* `eval`;
* concatenated shell command strings;
* automatic shell expansion;
* automatic interpretation of pipes, redirects, variables or glob patterns.

Users who need shell syntax must invoke a shell explicitly:

```bash
sidequest -- bash -lc 'make 2>&1 | tee build.log'
```

Sidequest must preserve interactive stdin, stdout and stderr in the command pane.

The following use cases must remain possible:

```bash
sidequest -- sudo apt upgrade
sidequest -- ssh server.example.com
sidequest -- ansible-playbook deploy.yml
```

## Security and privacy constraints

Security and privacy are core requirements.

Sidequest must not:

* store the requested command or its arguments on disk;
* record command output when `--no-history` is used;
* create shell history;
* inspect credentials;
* upload telemetry;
* access external network services;
* request root privileges for itself;
* modify the command before execution;
* silently answer interactive prompts;
* read unrelated user files or tmux sessions.

The requested command may use `sudo`, but Sidequest itself should continue running as the normal user:

```bash
sidequest -- sudo apt upgrade
```

Do not recommend:

```bash
sudo sidequest -- apt upgrade
```

## Runtime session storage

Use:

```text
$XDG_RUNTIME_DIR/sidequest/<session-id>/
```

When `XDG_RUNTIME_DIR` is unavailable, use a user-specific protected directory below `/tmp`.

Required permissions:

* runtime directory: `0700`;
* regular state files: `0600`;
* Unix socket: accessible only by the current user.

The command and argument array must be transferred through a protected Unix domain socket and must not be written into JSON, logs, environment dumps or temporary scripts.

Persist only non-sensitive session metadata such as:

* session ID;
* lifecycle state;
* start time;
* finish time;
* elapsed duration;
* exit code;
* signal-related termination state;
* tmux socket identifier.

By default, Sidequest may persist visible command-pane output in run history for
later local review. Users can disable that behavior with:

```bash
sidequest --no-history -- <command> [arguments...]
```

When `--no-history` is set, cleanup must not capture the tmux command pane,
must not write an output file, and must not create stored run metadata.

## tmux architecture

Each Sidequest run must use an isolated tmux server.

Use a unique tmux socket or server name, for example:

```bash
tmux -L sidequest-<session-id>
```

Do not modify or reuse the user's existing tmux sessions.

The Sidequest tmux environment must:

* ignore the user's normal tmux configuration where practical;
* contain one upper command pane;
* contain one lower game pane;
* start with focus in the command pane;
* disable the normal tmux status bar;
* bind `F12` to switch between panes;
* allow detach and later reattach;
* permit multiple concurrent Sidequest sessions.

Never terminate or clean up tmux sessions that are not owned by Sidequest.

## Session lifecycle

Expected lifecycle:

```text
created
→ waiting_for_command
→ running
→ completed | failed | interrupted
→ awaiting_user_exit
→ cleaned
```

Required behavior:

* Closing or detaching the terminal must not automatically kill a running command.
* `sidequest list` must show Sidequest sessions without exposing full command arguments.
* `sidequest attach <session-id>` must reconnect to the correct isolated tmux server.
* Ending the game must not stop the command.
* `Ctrl+C` in the command pane must reach the requested command.
* `Ctrl+C` or `Q` in the game pane must affect only the game.
* When the command finishes, the game must freeze promptly.
* The command pane must remain visible so the user can inspect its own terminal scrollback.
* Cleanup must occur only after the user closes the completed Sidequest session.

## Terminal requirements

The MVP requires:

```text
Minimum terminal size: 80 columns × 24 rows
Minimum usable game area: 40 columns × 10 rows
```

Before starting the requested command, validate:

* Linux platform;
* interactive terminal;
* tmux availability;
* supported terminal dimensions.

When validation fails, do not start the requested command.

Error messages must explain:

* what failed;
* what Sidequest expected;
* how the user can resolve it.

## Game architecture

Do not build an external plugin system during the MVP.

Games are compiled into the Sidequest binary and implement a small internal interface.

Keep game logic independent from terminal rendering where practical.

For Snake, separate at least:

* board state;
* snake movement;
* food placement;
* collision detection;
* score handling;
* tcell rendering;
* keyboard input.

Core game logic must be unit-testable without starting tmux or tcell.

The game must stop or freeze immediately when the associated command completes.

Do not persist high scores in version 0.1.

## Code organization

Use a structure similar to:

```text
cmd/
└── sidequest/
    └── main.go

internal/
├── cli/
├── preflight/
├── session/
├── tmux/
├── runner/
├── tui/
└── games/
    └── snake/
```

Guidelines:

* Keep `main.go` small.
* Keep packages focused.
* Prefer explicit dependencies over global state.
* Use `context.Context` for cancellation and lifecycle coordination.
* Wrap errors with relevant operational context.
* Avoid introducing abstractions before there are at least two concrete use cases.
* Do not add a framework where the standard library is sufficient.

## Testing requirements

Run before considering work complete:

```bash
go fmt ./...
go vet ./...
go test ./...
go build ./...
```

Add unit tests for logic that does not require a real terminal.

Important test areas include:

* CLI argument separation;
* commands containing spaces and special characters;
* runtime directory permissions;
* session state transitions;
* exit-code handling;
* signal-related termination;
* stale-session detection;
* Snake movement and collision rules;
* food placement;
* cleanup ownership checks.

Tests must never invoke destructive package-manager or system-administration commands.

Integration tests should use harmless commands such as:

```bash
true
false
sleep 1
bash -c 'exit 7'
printf 'test\n'
```

Tests requiring tmux must detect whether tmux is installed and skip cleanly when it is unavailable.

## Packaging requirements

The installed package should remain minimal:

```text
/usr/bin/sidequest
/usr/share/doc/sidequest/
/usr/share/man/man1/sidequest.1.gz
```

Do not install:

* systemd services;
* background daemons;
* files under `/etc`;
* user accounts;
* groups;
* scheduled jobs;
* telemetry components;
* Python or Go runtimes.

DEB and RPM packages must declare tmux as a runtime dependency.

The target installation commands are:

```bash
sudo apt install ./sidequest_<version>_linux_amd64.deb
sudo dnf install ./sidequest-<version>.x86_64.rpm
```

Publishing to official Debian or Fedora repositories is outside the initial MVP.

## Documentation rules

Update documentation when changing:

* public CLI syntax;
* keyboard controls;
* minimum terminal requirements;
* runtime dependencies;
* supported platforms;
* security or privacy behavior;
* package contents.

Examples in documentation must use the `--` separator.

Do not claim that Sidequest makes the executed command safer. Sidequest is only a terminal wrapper and game host. The requested command retains its normal effects and risks.

## Scope control

The following features are outside version 0.1 unless an issue explicitly adds them:

* additional games;
* external game plugins;
* persistent high scores;
* themes;
* configuration files;
* desktop notifications;
* command-output logging;
* telemetry;
* AI features;
* macOS support;
* Windows support;
* Docker-based execution;
* integration into an existing tmux session;
* custom APT or DNF repositories.

Do not implement speculative future features during MVP work.

## Pull request expectations

Each pull request should:

* address one primary issue;
* explain the implemented behavior;
* list relevant tests;
* mention known limitations;
* avoid unrelated refactoring;
* preserve the security and privacy constraints in this document.

Use closing references where appropriate:

```text
Closes #4
```

Do not mark an issue complete when acceptance criteria remain untested or only partially implemented.

## Design priority

When trade-offs occur, use this priority order:

1. Do not interfere with the requested command.
2. Do not expose or persist sensitive information.
3. Preserve terminal usability and recoverability.
4. Keep behavior predictable.
5. Keep the implementation small.
6. Add visual polish and additional game features.

A boring but reliable command runner is more important than an elaborate game.
