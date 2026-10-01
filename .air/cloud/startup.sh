#!/usr/bin/env bash
# Air cloud environment startup for skill-atlas.
# WARMUP (snapshot bake): installs everything, primes Go/Docker caches and runs the CI test tiers.
# TASK: re-applies the cheap per-boot bits and exits promptly.
set -euo pipefail

if [ "${AIR_STARTUP_MODE:-}" = warmup ]; then WARMUP=1; else WARMUP=; fi

REPO=$(cd "$(dirname "$0")/../.." && pwd)
GCC_PREFIX=$HOME/.local/share/air-gcc
APT_DIR=$HOME/.cache/air-apt
ENV_FILE=$HOME/.config/skill-atlas-env.sh
# Pinned image from e2e/screenshots.sh
SCREENSHOT_IMAGE=$(sed -n "s/^IMAGE='\(.*\)'$/\1/p" "$REPO/e2e/screenshots.sh")

log() { printf '[startup %s] %s\n' "$(date +%H:%M:%S)" "$*"; }

# The image has no C compiler and sudo needs a password, but `go test -race` needs cgo.
# Fetch Ubuntu's gcc + libc6-dev .debs without root and unpack them into a sysroot under $HOME.
install_gcc() {
	if [ -x "$GCC_PREFIX/usr/bin/gcc-13" ] && [ -x "$HOME/.local/bin/gcc" ]; then
		log "userspace gcc already installed"
		return
	fi
	log "installing userspace gcc into $GCC_PREFIX"
	mkdir -p "$APT_DIR/lists/partial" "$APT_DIR/archives/partial" "$GCC_PREFIX" "$HOME/.local/bin"
	local apt_opts=(
		-o "Dir::State::Lists=$APT_DIR/lists" -o "Dir::Cache=$APT_DIR"
		-o "Dir::Cache::archives=$APT_DIR/archives" -o Debug::NoLocking=1
		-o "APT::Sandbox::User=$(id -un)"
	)
	apt-get "${apt_opts[@]}" update -qq
	apt-get "${apt_opts[@]}" install -y -qq --download-only --no-install-recommends gcc libc6-dev
	local deb pkg
	for deb in "$APT_DIR"/archives/*.deb; do
		pkg=$(dpkg-deb -f "$deb" Package)
		# Keep the system's runtime libs (libc6, libgcc-s1, ...), extract only what is missing
		if dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | grep -q 'ok installed'; then continue; fi
		dpkg-deb -x "$deb" "$GCC_PREFIX"
	done
	# Sysroot: libc.so's linker script names /lib/... and /lib64/...; point runtime libs at the real ones
	[ -e "$GCC_PREFIX/lib" ] || ln -s usr/lib "$GCC_PREFIX/lib"
	[ -e "$GCC_PREFIX/lib64" ] || ln -s usr/lib64 "$GCC_PREFIX/lib64"
	mkdir -p "$GCC_PREFIX/usr/lib64"
	local d f
	for d in /usr/lib/x86_64-linux-gnu /usr/lib64; do
		for f in "$d"/*.so*; do
			[ -e "$GCC_PREFIX$f" ] || [ -L "$GCC_PREFIX$f" ] || ln -s "$f" "$GCC_PREFIX$f"
		done
	done
	cat >"$HOME/.local/bin/gcc" <<'EOF'
#!/bin/sh
P=$HOME/.local/share/air-gcc
LD_LIBRARY_PATH=$P/usr/lib/x86_64-linux-gnu${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH} PATH=$P/usr/bin:$PATH exec "$P/usr/bin/gcc-13" --sysroot="$P" "$@"
EOF
	chmod +x "$HOME/.local/bin/gcc"
	ln -sf gcc "$HOME/.local/bin/cc"
	rm -f "$APT_DIR"/archives/*.deb
	log "userspace gcc installed: $("$HOME/.local/bin/gcc" --version | head -1)"
}

# Agent shells are fresh login shells, so persist env through the profile, not a bare export.
write_env() {
	mkdir -p "$(dirname "$ENV_FILE")"
	cat >"$ENV_FILE" <<'EOF'
# skill-atlas dev environment (written by .air/cloud/startup.sh)
case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) export PATH="$HOME/.local/bin:$PATH" ;; esac
export CC="$HOME/.local/bin/gcc"
export CGO_ENABLED=1
EOF
	local line="[ -f \"$ENV_FILE\" ] && . \"$ENV_FILE\" # skill-atlas-env"
	local profile rc
	profile=
	for rc in "$HOME/.bash_profile" "$HOME/.bash_login" "$HOME/.profile"; do
		if [ -f "$rc" ]; then profile=$rc; break; fi
	done
	[ -n "$profile" ] || { profile=$HOME/.profile; touch "$profile"; }
	for rc in "$profile" "$HOME/.bashrc"; do
		grep -q '# skill-atlas-env' "$rc" 2>/dev/null || printf '\n%s\n' "$line" >>"$rc"
	done
	# shellcheck disable=SC1090
	. "$ENV_FILE"
}

# Nested containers (e2e/screenshots.sh) need the egress proxy too; the Docker client injects it.
configure_docker_proxy() {
	[ -n "${HTTPS_PROXY:-}" ] || return 0
	mkdir -p "$HOME/.docker"
	python3 - "$HOME/.docker/config.json" <<'EOF'
import json, os, sys
path = sys.argv[1]
try:
    cfg = json.load(open(path))
except (FileNotFoundError, ValueError):
    cfg = {}
cfg.setdefault("proxies", {})["default"] = {
    "httpProxy": os.environ.get("HTTP_PROXY", os.environ["HTTPS_PROXY"]),
    "httpsProxy": os.environ["HTTPS_PROXY"],
    "noProxy": os.environ.get("NO_PROXY", "localhost,127.0.0.1"),
}
json.dump(cfg, open(path, "w"), indent=2)
EOF
	log "docker client proxy configured"
}

wait_docker() {
	until docker info >/dev/null 2>&1; do log "waiting for docker daemon"; sleep 3; done
}

warm_caches() {
	cd "$REPO"
	log "go mod download"
	go mod download
	log "go build ./..."
	go build ./...
	wait_docker
	log "pulling screenshot image $SCREENSHOT_IMAGE"
	docker pull -q "$SCREENSHOT_IMAGE"
}

# Runs the CI test tiers; also primes the build, test and Playwright caches for the snapshot.
healthcheck() {
	cd "$REPO"
	wait_docker
	log "healthcheck: race build of the CLI"
	go build -race -o /tmp/skill-atlas-healthcheck ./cmd/skill-atlas
	/tmp/skill-atlas-healthcheck --help >/dev/null 2>&1 || [ $? -eq 2 ]
	rm -f /tmp/skill-atlas-healthcheck
	log "healthcheck: unit tests (go test -short -race ./...)"
	go test -short -race ./...
	log "healthcheck: clone tests against GitHub"
	go test -race -count=1 ./internal/repo/...
	log "healthcheck: e2e tmux tests"
	go test -tags e2e -count=1 -timeout 10m ./e2e/...
	log "healthcheck: lint"
	test -z "$(gofmt -l .)"
	go vet ./...
	go vet -tags e2e ./...
	go mod tidy -diff
	log "healthcheck: HTML screenshot tests (docker)"
	E2E_ARTIFACTS_DIR=/tmp/skill-atlas-screenshots e2e/screenshots.sh
	log "healthcheck: OK"
}

log "mode: ${AIR_STARTUP_MODE:-unset}"
install_gcc
write_env
configure_docker_proxy
if [ -n "$WARMUP" ]; then
	warm_caches
	healthcheck
fi
log "done"
