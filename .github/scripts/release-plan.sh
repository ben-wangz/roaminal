#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ARTIFACT_KIND="${1:?artifact kind is required: chart or container}"
case "${ARTIFACT_KIND}" in
    chart|container) ;;
    *) echo "unsupported artifact kind: ${ARTIFACT_KIND}" >&2; exit 2 ;;
esac

: "${TAG_NAME:?TAG_NAME is required}"
: "${FORGEKIT_BIN:?FORGEKIT_BIN is required}"
: "${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}"

exec python3 "${SCRIPT_DIR}/release-plan.py" "${PROJECT_ROOT}" "${ARTIFACT_KIND}"
