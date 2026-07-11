# sidequest
Play a tiny terminal game while waiting for a long-running command to finish.

Your computer has a task. So do you.

sidequest -- sudo apt upgrade
sidequest -- docker build .
sidequest -- cargo build --release
sidequest -- ansible-playbook upgrade.yml

Sidequest runs your command in one terminal pane and opens a small retro game in another. When the command finishes, the game stops and Sidequest shows the exit status and runtime.

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
