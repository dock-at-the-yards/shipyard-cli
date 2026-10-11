#!/usr/bin/env bash
#
# Shipyard CLI installer
#
# Installs a published Shipyard CLI release. Because the evaluation engine
# (shipyard-core) is a private repository, the CLI is NOT installable with
# `go install`. Distribution is exclusively through signed, checksummed
# release artifacts and the GHCR container image.
#
#   curl -fsSL https://raw.githubusercontent.com/dock-at-the-yards/shipyard-cli/main/install.sh | sh
#
# Channels:
#   stable (default)  latest non-prerelease          v0.0.1
#   test              latest alpha/beta/rc prerelease v0.0.1-alpha.1
#   dev               latest development build        v0.0.1-dev.42
#
# Usage:
#   install.sh [--channel stable|test|dev] [--version v0.0.1]
#              [--install-dir DIR] [--yes] [--dry-run] [--uninstall]
#
# Environment:
#   SHIPYARD_CHANNEL     same as --channel
#   SHIPYARD_VERSION     same as --version
#   SHIPYARD_INSTALL_DIR same as --install-dir

set -euo pipefail

REPO="dock-at-the-yards/shipyard-cli"
GITHUB_API="https://api.github.com"
DEFAULT_CHANNEL="stable"
BINARY="shipyard"
INSTALL_DIR=""

CHANNEL="${SHIPYARD_CHANNEL:-$DEFAULT_CHANNEL}"
VERSION="${SHIPYARD_VERSION:-}"
ASSUME_YES="0"
DRY_RUN="0"
UNINSTALL="0"

log()  { printf '%s\n' "$*" >&2; }
info() { printf '  %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$1" >&2; exit 1; }

usage() {
  sed -n '3,20p' "$0" | sed 's/^# \{0,1\}//'
  exit 0
}

while [ $# -gt 0 ]; do
  case "$1" in
    --channel)      CHANNEL="${2:?--channel requires a value}"; shift 2 ;;
    --channel=*)    CHANNEL="${1#*=}"; shift ;;
    --version)      VERSION="${2:?--version requires a value}"; shift 2 ;;
    --version=*)    VERSION="${1#*=}"; shift ;;
    --install-dir)  INSTALL_DIR="${2:?--install-dir requires a value}"; shift 2 ;;
    --install-dir=*) INSTALL_DIR="${1#*=}"; shift ;;
    --yes|-y)       ASSUME_YES=1; shift ;;
    --dry-run)      DRY_RUN=1; shift ;;
    --uninstall)    UNINSTALL=1; shift ;;
    --help|-h)      usage ;;
    *)              die "unknown argument: $1 (try --help)" ;;
  esac
done

# ---------------------------------------------------------------- platform --

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    linux|darwin) ;;
    *) die "unsupported operating system: $os (supported: linux, darwin)" ;;
  esac

  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64)  arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) die "unsupported architecture: $arch (supported: amd64, arm64)" ;;
  esac

  PLATFORM_OS="$os"
  PLATFORM_ARCH="$arch"
}

default_install_dir() {
  if [ -n "$INSTALL_DIR" ]; then
    printf '%s' "$INSTALL_DIR"; return
  fi
  case "$(uname -s)" in
    Darwin)
      if [ -d /opt/homebrew/bin ] && [ -w /opt/homebrew/bin ]; then
        printf '/opt/homebrew/bin'
      else
        printf '%s' "$HOME/.shipyard/bin"
      fi
      ;;
    *)
      if [ -w /usr/local/bin ] 2>/dev/null; then
        printf '/usr/local/bin'
      else
        printf '%s' "$HOME/.shipyard/bin"
      fi
      ;;
  esac
}

need() {
  command -v "$1" >/dev/null 2>&1 || die "required tool not found: $1"
}

# ------------------------------------------------------------- version res --

# Resolve a channel to a concrete version tag.
#
# The release pipeline maintains a `dist` branch in this repository holding
# one plain-text file per channel (stable, test, dev), each containing a
# single version tag. Reading it needs no JSON parser, no jq, and no
# python, so the installer stays dependency-free apart from curl and tar.
resolve_version() {
  need curl

  case "$CHANNEL" in
    stable|test|dev) ;;
    *) die "unknown channel '$CHANNEL' (expected: stable, test, dev)" ;;
  esac

  local tag=""
  if tag="$(curl -fsSL --max-time 30 \
        "https://raw.githubusercontent.com/${REPO}/dist/${CHANNEL}" 2>/dev/null)"; then
    tag="$(printf '%s' "$tag" | tr -d '[:space:]')"
  fi

  # Fallback for the stable channel only: GitHub's "latest" endpoint already
  # excludes drafts and prereleases, so it needs no parsing. The trailing `||`
  # keeps `set -e` from aborting the assignment when the endpoint 404s, which
  # would otherwise skip the diagnostic below and exit with a bare code 56.
  if [ -z "$tag" ] && [ "$CHANNEL" = "stable" ]; then
    log "channel manifest unavailable, falling back to the latest release"
    tag="$(curl -fsSL --max-time 30 \
          -H 'Accept: application/vnd.github+json' \
          "${GITHUB_API}/repos/${REPO}/releases/latest" 2>/dev/null \
        | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
        | head -n1 || true)"
  fi

  if [ -z "$tag" ]; then
    if [ "$CHANNEL" = "stable" ]; then
      die "no stable release found for $REPO.

  Every published release so far is a prerelease, and GitHub's 'latest'
  endpoint does not consider prereleases. Install the test channel instead:

    curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sh -s -- --channel test

  or pin an exact version with --version v<MAJOR.MINOR.PATCH>."
    fi
    die "could not resolve the '$CHANNEL' channel for $REPO.
The dist channel manifest may not be published yet, or no release exists
on this channel. Publish a release, or pin explicitly with --version."
  fi
  printf '%s' "$tag"
}

# ----------------------------------------------------------------- fetch ----

verify_checksum() {
  # $1 = downloaded tarball, $2 = checksums file, $3 = expected filename
  local expected actual
  expected="$(awk -v f="$3" '$2 == f || $2 == "*"f {print $1}' "$2" | head -n1)"
  [ -n "$expected" ] || die "checksum for $3 not found in checksums file"

  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$1" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$1" | awk '{print $1}')"
  else
    log "warning: no sha256 tool found, skipping checksum verification"
    return 0
  fi

  [ "$expected" = "$actual" ] || die "checksum mismatch for $3
  expected: $expected
  actual:   $actual
The download may be corrupt or tampered with. Not installing."
  info "checksum verified"
}

# ---------------------------------------------------------------- install --

do_uninstall() {
  local dir; dir="$(default_install_dir)"
  local target="$dir/$BINARY"
  if [ -e "$target" ]; then
    rm -f "$target"
    log "removed $target"
  else
    log "nothing to remove at $target"
  fi
}

main() {
  detect_platform

  if [ "$UNINSTALL" = "1" ]; then
    do_uninstall
    return 0
  fi

  if [ -z "$VERSION" ]; then
    log "Resolving the '$CHANNEL' channel..."
    VERSION="$(resolve_version)"
    info "resolved $CHANNEL -> $VERSION"
  else
    case "$VERSION" in
      v*) ;;
      *) VERSION="v$VERSION" ;;
    esac
    info "using pinned version $VERSION"
  fi

  local dir; dir="$(default_install_dir)"
  local asset="${BINARY}-${VERSION}-${PLATFORM_OS}-${PLATFORM_ARCH}.tar.gz"
  local base="https://github.com/${REPO}/releases/download/${VERSION}"
  local tmp
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064
  trap "rm -rf '$tmp'" EXIT

  log "Downloading ${asset}..."
  curl -fsSL --progress-bar -o "$tmp/$asset" "${base}/${asset}" \
    || die "download failed: ${base}/${asset}
The asset name follows ${BINARY}-<version>-<os>-<arch>.tar.gz
Check that a release exists for ${VERSION} on the '${CHANNEL}' channel."

  log "Downloading checksums.txt..."
  curl -fsSL --progress-bar -o "$tmp/checksums.txt" "${base}/checksums.txt" \
    || die "checksums.txt not found for ${VERSION} (release may be incomplete)"

  verify_checksum "$tmp/$asset" "$tmp/checksums.txt" "$asset"

  log "Extracting..."
  tar -xzf "$tmp/$asset" -C "$tmp"
  [ -f "$tmp/$BINARY" ] || die "expected a '$BINARY' binary inside $asset"
  chmod +x "$tmp/$BINARY"

  if [ "$DRY_RUN" = "1" ]; then
    log ""
    log "dry run, not installing."
    log "  version:  $VERSION"
    log "  channel:  $CHANNEL"
    log "  target:   $dir/$BINARY"
    log "  size:     $(wc -c < "$tmp/$BINARY" | tr -d ' ') bytes"
    return 0
  fi

  if [ -e "$dir/$BINARY" ] && [ "$ASSUME_YES" != "1" ] && [ ! -t 0 ]; then
    die "$dir/$BINARY already exists. Re-run with --yes to overwrite."
  fi

  log "Installing to $dir/$BINARY..."
  mkdir -p "$dir"
  # Install atomically so a concurrent install cannot see a partial binary.
  cp "$tmp/$BINARY" "$dir/.$BINARY.tmp.$$"
  chmod +x "$dir/.$BINARY.tmp.$$"
  mv -f "$dir/.$BINARY.tmp.$$" "$dir/$BINARY"

  log ""
  log "Shipyard $VERSION installed to $dir/$BINARY"
  log ""

  case ":$PATH:" in
    *":$dir:"*) ;;
    *)
      log "Note: $dir is not on your PATH."
      log ""
      case "$(uname -s)" in
        Darwin)
          log "Add this to your ~/.zshrc:"
          log "  export PATH=\"$dir:\$PATH\""
          ;;
        *)
          log "Add this to your ~/.bashrc:"
          log "  export PATH=\"$dir:\$PATH\""
          ;;
      esac
      log ""
      ;;
  esac

  log "Verify with:  $BINARY version"
  log "Upgrade with: curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sh -s -- --channel ${CHANNEL}"
}

main "$@"
