#!/usr/bin/env sh
set -eu

GO="${GO:-go}"

if ! command -v "$GO" >/dev/null 2>&1; then
	printf 'sidequest QA: Go executable %s was not found. Set GO=/path/to/go or add go to PATH.\n' "$GO" >&2
	exit 127
fi

usage() {
	cat <<'USAGE'
Usage:
  ./scripts/qa.sh [--race] [--cover]

Runs the normal local Sidequest QA suite:
  go fmt ./...
  go vet ./...
  go test ./...
  go build ./...

Optional extended checks:
  --race   also run go test -race ./...
  --cover  also run go test -cover ./...
USAGE
}

run_race=0
run_cover=0
for arg in "$@"; do
	case "$arg" in
		--race)
			run_race=1
			;;
		--cover)
			run_cover=1
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			usage >&2
			exit 2
			;;
	esac
done

run() {
	printf '\n==> %s\n' "$*"
	"$@"
}

run "$GO" fmt ./...
run "$GO" vet ./...
run "$GO" test ./...

if [ "$run_race" -eq 1 ]; then
	run "$GO" test -race ./...
fi

if [ "$run_cover" -eq 1 ]; then
	run "$GO" test -cover ./...
fi

run "$GO" build ./...
