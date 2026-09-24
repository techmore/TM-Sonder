#!/usr/bin/env bash
# Prepare a host for a TM Sonder installation.
#
# FFmpeg provides both ffmpeg and ffprobe on the supported package managers.
# The server can start without them, but probing, chapters, thumbnails,
# transcoding, and audiobook optimization will not work.
set -euo pipefail

usage() {
    cat <<'EOF'
Usage: deploy/prepare-install.sh [--check|--install]

  --check    Verify ffmpeg and ffprobe are available (default).
  --install  Install the ffmpeg package with the detected package manager,
             then verify both executables.

SONDER_FFMPEG_PATH and SONDER_FFPROBE_PATH may point at custom executables.
EOF
}

mode="check"
case "${1:-}" in
    "") ;;
    --check) mode="check" ;;
    --install) mode="install" ;;
    -h|--help)
        usage
        exit 0
        ;;
    *)
        usage >&2
        exit 2
        ;;
esac

run_privileged() {
    if [[ "${EUID}" -eq 0 ]]; then
        "$@"
    else
        sudo "$@"
    fi
}

resolve_tool() {
    local configured="$1"
    local name="$2"

    if [[ -n "$configured" ]]; then
        if [[ "$configured" == */* ]]; then
            [[ -x "$configured" ]] && printf '%s\n' "$configured"
        else
            command -v "$configured" 2>/dev/null || true
        fi
        return
    fi
    command -v "$name" 2>/dev/null || true
}

tool_works() {
    local path="$1"
    [[ -n "$path" && -x "$path" ]] && "$path" -version >/dev/null 2>&1
}

ffmpeg_path="$(resolve_tool "${SONDER_FFMPEG_PATH:-}" ffmpeg)"
ffprobe_path="$(resolve_tool "${SONDER_FFPROBE_PATH:-}" ffprobe)"

if ! tool_works "$ffmpeg_path" || ! tool_works "$ffprobe_path"; then
    if [[ "$mode" == "install" ]]; then
        if command -v brew >/dev/null 2>&1; then
            brew install ffmpeg
        elif command -v apt-get >/dev/null 2>&1; then
            run_privileged apt-get update
            DEBIAN_FRONTEND=noninteractive run_privileged apt-get install -y ffmpeg
        elif command -v dnf >/dev/null 2>&1; then
            run_privileged dnf install -y ffmpeg
        elif command -v apk >/dev/null 2>&1; then
            run_privileged apk add --no-cache ffmpeg
        elif command -v pacman >/dev/null 2>&1; then
            run_privileged pacman -Sy --needed --noconfirm ffmpeg
        else
            echo "No supported package manager found; install FFmpeg manually." >&2
        fi

        ffmpeg_path="$(resolve_tool "${SONDER_FFMPEG_PATH:-}" ffmpeg)"
        ffprobe_path="$(resolve_tool "${SONDER_FFPROBE_PATH:-}" ffprobe)"
    fi
fi

missing=()
tool_works "$ffmpeg_path" || missing+=(ffmpeg)
tool_works "$ffprobe_path" || missing+=(ffprobe)

if (( ${#missing[@]} > 0 )); then
    echo "TM Sonder install preflight failed: missing or unusable ${missing[*]}." >&2
    echo "Install the FFmpeg package; it provides both ffmpeg and ffprobe." >&2
    echo "  macOS:  brew install ffmpeg" >&2
    echo "  Ubuntu: sudo apt-get update && sudo apt-get install -y ffmpeg" >&2
    exit 1
fi

printf 'TM Sonder media tools ready: ffmpeg=%s ffprobe=%s\n' \
    "$ffmpeg_path" "$ffprobe_path"
