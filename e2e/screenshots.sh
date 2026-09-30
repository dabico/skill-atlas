#!/bin/sh
# Runs the HTML screenshot tests in the pinned Linux container the baselines are rendered in.
# Usage: e2e/screenshots.sh [-update]
#   -update  rewrite e2e/testdata/screenshots/*.png instead of comparing
# E2E_ARTIFACTS_DIR (default: $TMPDIR/skill-atlas-screenshots) receives expected/actual/diff PNGs of failing shots.
set -eu

# Bump tag and digest together; then rewrite the baselines with -update.
IMAGE='golang:1.27.1-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195'

root=$(cd "$(dirname "$0")/.." && pwd)
artifacts=${E2E_ARTIFACTS_DIR:-${TMPDIR:-/tmp}/skill-atlas-screenshots}
mkdir -p "$artifacts"

flags=''
case "${1:-}" in
'') ;;
-update) flags='-update' ;;
*) echo "usage: $0 [-update]" >&2; exit 2 ;;
esac

# amd64 matches CI; it is emulated on Apple silicon. The volumes keep Go and browser downloads between runs.
# -buildvcs=false: the worktree's .git points outside the mount.
status=0
docker run --rm --platform linux/amd64 --shm-size=1g \
	-v "$root":/src -w /src \
	-v "$artifacts":/artifacts \
	-v skill-atlas-e2e-gomod:/go/pkg/mod \
	-v skill-atlas-e2e-cache:/root/.cache \
	-e SKILL_ATLAS_E2E_SCREENSHOTS=1 \
	-e E2E_ARTIFACTS_DIR=/artifacts \
	-e GOFLAGS=-buildvcs=false \
	-e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" \
	-e FLAGS="$flags" \
	"$IMAGE" sh -c '
		# Files written in the mount belong to the caller, not root.
		trap "chown -R $HOST_UID:$HOST_GID /src/e2e/testdata /artifacts 2>/dev/null || true" EXIT
		go run github.com/mxschmitt/playwright-go/cmd/playwright install --with-deps chromium &&
		go test -tags e2e -count=1 -timeout 10m -run "^TestScreenshots$" ./e2e/... -v $FLAGS
	' || status=$?

if [ "$status" -ne 0 ] && [ -n "$(ls -A "$artifacts" 2>/dev/null)" ]; then
	echo "Failing screenshots are in $artifacts/screenshots" >&2
fi
exit "$status"
