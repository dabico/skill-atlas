#!/usr/bin/env bash
# Record skill-atlas demos: the TUI with vhs, the --html report with Playwright.
# Usage: demo.sh [all|tui|html] [tape...]
set -euo pipefail

mode=${1:-all}
shift || true
case $mode in all | tui | html) ;; *) echo "usage: demo.sh [all|tui|html] [tape...]" >&2; exit 2 ;; esac

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
skill=$(dirname "$here")
# Record the checkout you run from, so a worktree records its own branch.
root=${DEMO_SRC:-$(git rev-parse --show-toplevel 2>/dev/null || git -C "$here" rev-parse --show-toplevel)}
url=${DEMO_URL:-https://github.com/zcaceres/skills.git}
ref=${DEMO_REF:-zoom@1.0.1}
out=${DEMO_OUT:-$root/docs/demo}
query=${DEMO_QUERY:-laconic}
# DEMO_ARGS replaces the scan arguments, e.g. several URLs; split on spaces.
if [[ -n ${DEMO_ARGS:-} ]]; then read -ra scan_args <<<"$DEMO_ARGS"; else scan_args=(--ref "$ref" "$url"); fi

need() { command -v "$1" >/dev/null || { echo "demo.sh: $1 not found. $2" >&2; exit 1; }; }
need go "Install Go."
[[ $mode == html ]] || need vhs "Run: brew install vhs"
[[ $mode == tui ]] || { need uv "Run: brew install uv"; need ffmpeg "Run: brew install ffmpeg"; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$out" "$work/bin"
out=$(cd "$out" && pwd)
(cd "$root" && go build -o "$work/bin/skill-atlas" ./cmd/skill-atlas)
echo "built skill-atlas from $root"

# Escape a value for the right-hand side of a sed s|||.
esc() { printf '%s' "$1" | sed -e 's/[\\&|]/\\&/g'; }

tui() {
	local tapes=("$@") cmd="skill-atlas scan ${scan_args[*]}"
	[[ ${#tapes[@]} -gt 0 ]] || tapes=("$skill/assets/tui-tour.tape")
	for t in "${tapes[@]}"; do
		local filled
		filled="$work/$(basename "$t")"
		sed -e "s|{{BIN}}|$(esc "$work/bin")|g" \
			-e "s|{{CMD}}|$(esc "$cmd")|g" \
			-e "s|{{OUT}}|$(esc "$out")|g" \
			-e "s|{{QUERY}}|$(esc "$query")|g" "$t" >"$filled"
		echo "vhs $(basename "$t")"
		(cd "$work" && vhs -q "$filled")
	done
}

html() {
	local log="$work/html.log" report
	if ! BROWSER=true TMPDIR="$work" "$work/bin/skill-atlas" scan --html "${scan_args[@]}" >/dev/null 2>"$log"; then
		cat "$log" >&2
		exit 1
	fi
	report=$(sed -n 's/^Report: //p' "$log")
	[[ -f $report ]] || { echo "demo.sh: no report path in output:" >&2; cat "$log" >&2; exit 1; }
	uv run --quiet "$here/html.py" "$report" "$out" "$query"
}

case $mode in
tui) tui "$@" ;;
html) html ;;
all) tui "$@"; html ;;
esac

echo "output in $out:"
ls -lh "$out"
