#!/bin/bash

set -o errexit -o nounset -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "${SCRIPT_DIR}")"
TARGET_DIR="${PROJECT_ROOT}/build/bin"
TARGET_BIN="${TARGET_DIR}/forgekit"

DEFAULT_MIN_VERSION="${FORGEKIT_MIN_VERSION:-0.6.1}"
DEFAULT_BEST_VERSION="${FORGEKIT_BEST_VERSION:-0.6.1}"
FORGEKIT_REPO="${FORGEKIT_REPO:-ben-wangz/forgekit}"

usage() {
    cat <<'EOF'
Usage: setup/forgekit.sh [min-version] [best-version]

Ensures a usable ForgeKit binary is available in build/bin/forgekit.

Environment:
  FORGEKIT_MIN_VERSION   Minimum accepted version (default: 0.6.1)
  FORGEKIT_BEST_VERSION  Preferred release to download (default: 0.6.1)
  FORGEKIT_REPO          GitHub owner/repository (default: ben-wangz/forgekit)
  FORGEKIT_DOWNLOAD_BASE Override release base URL for mirrors

The resolved binary path is printed to stdout.
EOF
}

normalize_version() {
    local raw="$1"
    raw="${raw#v}"
    raw="${raw%%-*}"
    raw="${raw%%+*}"
    [[ "$raw" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || return 1
    printf '%s\n' "$raw"
}

version_ge() {
    local left right
    left="$(normalize_version "$1")" || return 1
    right="$(normalize_version "$2")" || return 1
    local l_major l_minor l_patch r_major r_minor r_patch
    IFS='.' read -r l_major l_minor l_patch <<< "$left"
    IFS='.' read -r r_major r_minor r_patch <<< "$right"
    ((l_major > r_major || (l_major == r_major && l_minor > r_minor) ||
        (l_major == r_major && l_minor == r_minor && l_patch >= r_patch)))
}

read_forgekit_version() {
    local output
    output="$(${1} --version 2>/dev/null)" || return 1
    [[ "$output" =~ forgekit[[:space:]]+([^[:space:]]+) ]] || return 1
    normalize_version "${BASH_REMATCH[1]}"
}

detect_os() {
    case "$(uname -s | tr '[:upper:]' '[:lower:]')" in
        linux|darwin) uname -s | tr '[:upper:]' '[:lower:]' ;;
        *) echo "Error: unsupported operating system" >&2; return 1 ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) printf 'amd64\n' ;;
        aarch64|arm64) printf 'arm64\n' ;;
        *) echo "Error: unsupported architecture" >&2; return 1 ;;
    esac
}

download_release() {
    local best_version="$1" os arch filename tmp_dir base success=false
    os="$(detect_os)"
    arch="$(detect_arch)"
    filename="forgekit_${os}_${arch}"
    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "${tmp_dir:-}"; trap - RETURN' RETURN
    command -v curl >/dev/null 2>&1 || {
        echo "Error: curl is required to download ForgeKit" >&2
        return 1
    }

    local -a bases
    if [[ -n "${FORGEKIT_DOWNLOAD_BASE:-}" ]]; then
        bases=("${FORGEKIT_DOWNLOAD_BASE%/}/${FORGEKIT_REPO}/releases/download/v${best_version}")
    else
        bases=(
            "https://github.com/${FORGEKIT_REPO}/releases/download/v${best_version}"
            "https://files.m.daocloud.io/github.com/${FORGEKIT_REPO}/releases/download/v${best_version}"
        )
    fi
    for base in "${bases[@]}"; do
        if curl -fsSL --retry 3 --connect-timeout 10 --max-time 120 \
            -o "${tmp_dir}/${filename}" "${base}/${filename}" && \
            curl -fsSL --retry 3 --connect-timeout 10 --max-time 120 \
            -o "${tmp_dir}/checksums.txt" "${base}/checksums.txt"; then
            success=true
            break
        fi
    done
    [[ "$success" == true ]] || {
        echo "Error: failed to download ForgeKit v${best_version}" >&2
        return 1
    }

    if command -v sha256sum >/dev/null 2>&1; then
        (cd "$tmp_dir" && sha256sum --check --status checksums.txt --ignore-missing) || {
            echo "Error: ForgeKit checksum verification failed" >&2
            return 1
        }
    else
        echo "Warning: sha256sum not found; checksum verification skipped" >&2
    fi
    mkdir -p "$TARGET_DIR"
    install -m 0755 "${tmp_dir}/${filename}" "$TARGET_BIN"
}

main() {
    [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]] && { usage; return 0; }
    [[ $# -le 2 ]] || { usage >&2; return 1; }
    local min_version best_version target_version system_bin system_version
    min_version="$(normalize_version "${1:-${DEFAULT_MIN_VERSION}}")" || {
        echo "Error: invalid minimum ForgeKit version" >&2; return 1;
    }
    best_version="$(normalize_version "${2:-${DEFAULT_BEST_VERSION}}")" || {
        echo "Error: invalid preferred ForgeKit version" >&2; return 1;
    }
    version_ge "$best_version" "$min_version" || {
        echo "Error: preferred ForgeKit version is below minimum" >&2; return 1;
    }

    if [[ -x "$TARGET_BIN" ]] && target_version="$(read_forgekit_version "$TARGET_BIN")" &&
        version_ge "$target_version" "$min_version"; then
        printf '%s\n' "$TARGET_BIN"
        return 0
    fi
    if command -v forgekit >/dev/null 2>&1; then
        system_bin="$(command -v forgekit)"
        if system_version="$(read_forgekit_version "$system_bin")" &&
            version_ge "$system_version" "$min_version"; then
            mkdir -p "$TARGET_DIR"
            install -m 0755 "$system_bin" "$TARGET_BIN"
            printf '%s\n' "$TARGET_BIN"
            return 0
        fi
    fi
    download_release "$best_version"
    target_version="$(read_forgekit_version "$TARGET_BIN")" || {
        echo "Error: installed ForgeKit version could not be read" >&2; return 1;
    }
    version_ge "$target_version" "$min_version" || {
        echo "Error: installed ForgeKit is below minimum" >&2; return 1;
    }
    printf '%s\n' "$TARGET_BIN"
}

main "$@"
