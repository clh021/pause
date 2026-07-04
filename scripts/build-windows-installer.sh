#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-Pause}"
APP_ICON_SOURCE="${APP_ICON_SOURCE:-${ROOT_DIR}/assets/branding/app-icon-1024.png}"
APP_ICON_TARGET="${ROOT_DIR}/build/appicon.png"
WINDOWS_ICON_SOURCE="${WINDOWS_ICON_SOURCE:-${ROOT_DIR}/assets/branding/icon.ico}"
WINDOWS_ICON_TARGET="${ROOT_DIR}/build/windows/icon.ico"
WINDOWS_PLATFORM="${WINDOWS_PLATFORM:-windows/amd64}"
WINDOWS_ARCH_LABEL="${WINDOWS_ARCH_LABEL:-}"
WINDOWS_OUTPUT_DIR="${WINDOWS_OUTPUT_DIR:-}"
APP_VERSION_OVERRIDE="${APP_VERSION_OVERRIDE:-}"
WINDOWS_NSIS_TEMPLATE="${WINDOWS_NSIS_TEMPLATE:-${ROOT_DIR}/scripts/windows-installer/project.nsi}"
WINDOWS_WEBVIEW2="${WINDOWS_WEBVIEW2:-download}"
WAILS_TAGS="${WAILS_TAGS:-wails}"
USE_CLEAN="${USE_CLEAN:-0}"
INCLUDE_PORTABLE_EXE="${INCLUDE_PORTABLE_EXE:-0}"
VITE_UPDATES_URL="${VITE_UPDATES_URL:-}"
NIRCMD_URL="${NIRCMD_URL:-https://www.nirsoft.net/utils/nircmd-x64.zip}"
NIRCMD_SHA256="${NIRCMD_SHA256:-}"
NIRCMD_ZIP=""
ARTIFACT_VERSION=""

print_help() {
  cat <<'EOF'
Usage:
  ./scripts/build-windows-installer.sh [options]

Options:
  --platform <windows/amd64|windows/arm64|...>  Build target platform
  --arch-label <label>                          Artifact folder label (default derived from platform)
  --output-dir <path>                           Artifact output dir
  --version <version>                           Version used in output filename
  --icon <path>                                 App icon source (png, used for app resources)
  --windows-ico <path>                          Windows icon source (.ico)
  --webview2 <download|browser|embed|error>     Wails WebView2 mode
  --tags <go_build_tags>                        Build tags (default: wails)
  --nsis-template <path>                        NSIS template path
  --clean                                       Enable wails -clean
  --no-clean                                    Disable wails -clean (default)
  --include-portable-exe                        Keep raw app exe in output directory
  --no-portable-exe                             Drop raw app exe from output directory (default)
  -h, --help                                    Show this help

Environment variables:
  APP_ICON_SOURCE, WINDOWS_ICON_SOURCE, WINDOWS_PLATFORM, WINDOWS_ARCH_LABEL,
  WINDOWS_OUTPUT_DIR, APP_VERSION_OVERRIDE, WINDOWS_WEBVIEW2, WINDOWS_NSIS_TEMPLATE, WAILS_TAGS,
  USE_CLEAN, INCLUDE_PORTABLE_EXE, VITE_UPDATES_URL, NIRCMD_URL, NIRCMD_SHA256
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --platform)
      WINDOWS_PLATFORM="$2"
      shift 2
      ;;
    --arch-label)
      WINDOWS_ARCH_LABEL="$2"
      shift 2
      ;;
    --output-dir)
      WINDOWS_OUTPUT_DIR="$2"
      shift 2
      ;;
    --version)
      APP_VERSION_OVERRIDE="$2"
      shift 2
      ;;
    --icon)
      APP_ICON_SOURCE="$2"
      shift 2
      ;;
    --windows-ico)
      WINDOWS_ICON_SOURCE="$2"
      shift 2
      ;;
    --webview2)
      WINDOWS_WEBVIEW2="$2"
      shift 2
      ;;
    --tags)
      WAILS_TAGS="$2"
      shift 2
      ;;
    --nsis-template)
      WINDOWS_NSIS_TEMPLATE="$2"
      shift 2
      ;;
    --clean)
      USE_CLEAN="1"
      shift
      ;;
    --no-clean)
      USE_CLEAN="0"
      shift
      ;;
    --include-portable-exe)
      INCLUDE_PORTABLE_EXE="1"
      shift
      ;;
    --no-portable-exe)
      INCLUDE_PORTABLE_EXE="0"
      shift
      ;;
    -h|--help)
      print_help
      exit 0
      ;;
    *)
      echo "ERROR: unknown option '$1'" >&2
      print_help >&2
      exit 1
      ;;
  esac
done

resolve_artifact_version() {
  local version="${APP_VERSION_OVERRIDE}"
  if [[ -z "${version}" && -f "${ROOT_DIR}/VERSION" ]]; then
    version="$(head -n 1 "${ROOT_DIR}/VERSION" | tr -d '[:space:]')"
  fi
  if [[ -z "${version}" && -f "${ROOT_DIR}/wails.json" ]]; then
    version="$(sed -n 's/.*"productVersion":[[:space:]]*"\([^"]*\)".*/\1/p' "${ROOT_DIR}/wails.json" | head -n 1)"
  fi
  version="$(echo "${version}" | tr -d '[:space:]')"
  version="${version#v}"
  if [[ -z "${version}" ]]; then
    version="0.0.0"
  fi
  echo "${version}"
}

ARTIFACT_VERSION="$(resolve_artifact_version)"

resolve_git_branch() {
  local branch=""
  if ! git -C "${ROOT_DIR}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    if [[ "${GITHUB_REF_TYPE:-}" == "branch" && -n "${GITHUB_REF_NAME:-}" ]]; then
      echo "${GITHUB_REF_NAME}"
    else
      echo "unknown"
    fi
    return
  fi
  branch="$(git -C "${ROOT_DIR}" rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
  if [[ -z "${branch}" || "${branch}" == "HEAD" ]]; then
    if [[ "${GITHUB_REF_TYPE:-}" == "branch" && -n "${GITHUB_REF_NAME:-}" ]]; then
      branch="${GITHUB_REF_NAME}"
    else
      branch="detached"
    fi
  fi
  echo "${branch}"
}

resolve_git_commit() {
  if git -C "${ROOT_DIR}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git -C "${ROOT_DIR}" rev-parse HEAD 2>/dev/null || true
  else
    echo "unknown"
  fi
}

resolve_build_time() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

BUILD_GIT_BRANCH="$(resolve_git_branch)"
BUILD_GIT_COMMIT="$(resolve_git_commit)"
BUILD_TIME="$(resolve_build_time)"
BUILD_LDFLAGS="-X pause/internal/meta.Version=${ARTIFACT_VERSION} -X pause/internal/meta.GitBranch=${BUILD_GIT_BRANCH} -X pause/internal/meta.GitCommit=${BUILD_GIT_COMMIT} -X pause/internal/meta.BuildTime=${BUILD_TIME}"

default_arch_label_from_platform() {
  case "${WINDOWS_PLATFORM}" in
    windows/amd64) echo "windows-x64" ;;
    windows/arm64) echo "windows-arm64" ;;
    windows/386) echo "windows-x86" ;;
    *) echo "windows-build" ;;
  esac
}

if [[ -z "${WINDOWS_ARCH_LABEL}" ]]; then
  WINDOWS_ARCH_LABEL="$(default_arch_label_from_platform)"
fi
if [[ -z "${WINDOWS_OUTPUT_DIR}" ]]; then
  WINDOWS_OUTPUT_DIR="${ROOT_DIR}/build/bin/${WINDOWS_ARCH_LABEL}"
fi

cd "${ROOT_DIR}"

if ! command -v nsis >/dev/null 2>&1 && ! command -v makensis >/dev/null 2>&1; then
  echo "ERROR: NSIS not found. Install it first (macOS: brew install nsis)." >&2
  exit 1
fi

echo "build config:"
echo "  app_name=${APP_NAME}"
echo "  windows_platform=${WINDOWS_PLATFORM}"
echo "  windows_arch_label=${WINDOWS_ARCH_LABEL}"
echo "  windows_output_dir=${WINDOWS_OUTPUT_DIR}"
echo "  artifact_version=${ARTIFACT_VERSION}"
echo "  git_branch=${BUILD_GIT_BRANCH}"
echo "  git_commit=${BUILD_GIT_COMMIT}"
echo "  build_time=${BUILD_TIME}"
echo "  app_icon_source=${APP_ICON_SOURCE}"
echo "  windows_icon_source=${WINDOWS_ICON_SOURCE}"
echo "  windows_webview2=${WINDOWS_WEBVIEW2}"
echo "  wails_tags=${WAILS_TAGS}"
echo "  use_clean=${USE_CLEAN}"
echo "  include_portable_exe=${INCLUDE_PORTABLE_EXE}"
echo "  nircmd_url=${NIRCMD_URL}"
echo "  nircmd_sha256=${NIRCMD_SHA256:-<not set>}"

if [[ ! -f "${APP_ICON_SOURCE}" ]]; then
  echo "ERROR: app icon source not found: ${APP_ICON_SOURCE}" >&2
  exit 1
fi
if [[ ! -f "${WINDOWS_ICON_SOURCE}" ]]; then
  echo "ERROR: windows icon source not found: ${WINDOWS_ICON_SOURCE}" >&2
  exit 1
fi

mkdir -p "${ROOT_DIR}/build/bin"
mkdir -p "${WINDOWS_OUTPUT_DIR}"
mkdir -p "$(dirname "${APP_ICON_TARGET}")"
mkdir -p "$(dirname "${WINDOWS_ICON_TARGET}")"
cp "${APP_ICON_SOURCE}" "${APP_ICON_TARGET}"
cp "${WINDOWS_ICON_SOURCE}" "${WINDOWS_ICON_TARGET}"

# Copy frontend dist into embed directory so the binary includes the web UI.
if [[ -d "${ROOT_DIR}/frontend/dist" ]] && [[ -f "${ROOT_DIR}/frontend/dist/index.html" ]]; then
  mkdir -p "${ROOT_DIR}/internal/remoteserver/static"
  cp -r "${ROOT_DIR}/frontend/dist/"* "${ROOT_DIR}/internal/remoteserver/static/"
  echo "frontend dist copied into embed directory"
fi

if [[ -f "${WINDOWS_NSIS_TEMPLATE}" ]]; then
  mkdir -p "${ROOT_DIR}/build/windows/installer"
  cp "${WINDOWS_NSIS_TEMPLATE}" "${ROOT_DIR}/build/windows/installer/project.nsi"
  echo "using NSIS template: ${WINDOWS_NSIS_TEMPLATE}"
else
  echo "WARNING: NSIS template not found, wails default template will be used: ${WINDOWS_NSIS_TEMPLATE}" >&2
fi

# Download nircmd.exe (silent screenshot tool, avoids GDI flash on Windows 11).
# NirSoft only ships a single x64 binary; ARM64 Windows runs it under emulation.
NIRCMD_DIR="${ROOT_DIR}/internal/remoteserver/nircmd"
NIRCMD_BIN="${NIRCMD_DIR}/nircmd.exe"
mkdir -p "${NIRCMD_DIR}"

# Helper: verify a file against NIRCMD_SHA256 if the env var is set.
verify_nircmd_sha256() {
  local file="$1"
  if [[ -z "${NIRCMD_SHA256:-}" ]]; then
    return 0
  fi
  if ! command -v sha256sum >/dev/null 2>&1; then
    echo "ERROR: sha256sum not available, cannot verify nircmd.exe" >&2
    return 1
  fi
  local computed
  computed="$(sha256sum "${file}" | cut -d' ' -f1)"
  if [[ "${computed}" != "${NIRCMD_SHA256}" ]]; then
    echo "ERROR: SHA-256 mismatch for ${file} (expected ${NIRCMD_SHA256}, got ${computed})" >&2
    return 1
  fi
  echo "SHA-256 checksum verified for ${file}"
  return 0
}

if [[ -f "${NIRCMD_BIN}" ]]; then
  echo "nircmd.exe already present at ${NIRCMD_BIN}"
  if ! verify_nircmd_sha256 "${NIRCMD_BIN}"; then
    echo "WARNING: existing nircmd.exe failed SHA-256 check, re-downloading ..."
    rm -f "${NIRCMD_BIN}"
  fi
fi

if [[ ! -f "${NIRCMD_BIN}" ]]; then
  NIRCMD_ZIP="$(mktemp /tmp/nircmd-XXXXXX.zip)"
  trap 'rm -f "${NIRCMD_ZIP}"' EXIT
  echo "downloading nircmd.exe from ${NIRCMD_URL} ..."
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "${NIRCMD_ZIP}" "${NIRCMD_URL}"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "${NIRCMD_ZIP}" "${NIRCMD_URL}"
  else
    echo "ERROR: need curl or wget to download nircmd.exe" >&2
    exit 1
  fi

  if ! verify_nircmd_sha256 "${NIRCMD_ZIP}"; then
    exit 1
  fi

  if ! command -v unzip >/dev/null 2>&1; then
    echo "ERROR: need unzip to extract nircmd.exe" >&2
    exit 1
  fi
  unzip -o -j "${NIRCMD_ZIP}" "nircmd.exe" -d "${NIRCMD_DIR}"
  trap - EXIT
  rm -f "${NIRCMD_ZIP}"
  NIRCMD_ZIP=""
  echo "nircmd.exe downloaded to ${NIRCMD_BIN}"
fi

STAMP_FILE="$(mktemp /tmp/pause-win-build-stamp-XXXXXX)"
cleanup_stamp() {
  rm -f "${STAMP_FILE}"
}
trap cleanup_stamp EXIT
touch "${STAMP_FILE}"

if command -v wails >/dev/null 2>&1; then
  WAILS_CMD=(wails)
else
  echo "wails not found in PATH, using 'go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.2'"
  WAILS_CMD=(go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.2)
fi

WAILS_ARGS=(
  build
  -platform "${WINDOWS_PLATFORM}"
  -tags "${WAILS_TAGS},nircmdembed"
  -ldflags "${BUILD_LDFLAGS}"
  -nsis
  -webview2 "${WINDOWS_WEBVIEW2}"
)
if [[ "${USE_CLEAN}" == "1" ]]; then
  WAILS_ARGS+=(-clean)
fi

echo "[1/2] Building ${APP_NAME} Windows installer (${WINDOWS_PLATFORM})"
if [[ -n "${VITE_UPDATES_URL}" ]]; then
  VITE_UPDATES_URL="${VITE_UPDATES_URL}" "${WAILS_CMD[@]}" "${WAILS_ARGS[@]}"
else
  "${WAILS_CMD[@]}" "${WAILS_ARGS[@]}"
fi

echo "[2/2] Collecting Windows artifacts into ${WINDOWS_OUTPUT_DIR}"
# Wails -clean can remove build/bin (including the custom output dir), so ensure it exists again.
mkdir -p "${WINDOWS_OUTPUT_DIR}"

GENERATED_FILES=()
while IFS= read -r -d '' file; do
  GENERATED_FILES+=("${file}")
done < <(
  find "${ROOT_DIR}/build/bin" \
    -maxdepth 1 \
    -type f \
    \( -name "*.exe" -o -name "*.msi" -o -name "*.zip" -o -name "*.blockmap" -o -name "*.msix" -o -name "*.appx" \) \
    -newer "${STAMP_FILE}" \
    -print0
)

if [[ "${#GENERATED_FILES[@]}" -eq 0 ]]; then
  echo "WARNING: no new installer artifacts detected in build/bin." >&2
  echo "You can still inspect ${ROOT_DIR}/build/bin manually." >&2
  exit 0
fi

for file in "${GENERATED_FILES[@]}"; do
  base="$(basename "${file}")"
  if [[ "${INCLUDE_PORTABLE_EXE}" != "1" && "${base}" == "${APP_NAME}.exe" ]]; then
    rm -f "${file}"
    echo "dropped non-installer artifact: ${base}"
    continue
  fi
  if [[ "${base}" == *"-installer.exe" ]]; then
    base="${APP_NAME}-v${ARTIFACT_VERSION}-${WINDOWS_ARCH_LABEL}-setup.exe"
  fi
  target="${WINDOWS_OUTPUT_DIR}/${base}"
  mv -f "${file}" "${target}"
  echo "moved: ${target}"
done

EXPECTED_INSTALLER="${WINDOWS_OUTPUT_DIR}/${APP_NAME}-v${ARTIFACT_VERSION}-${WINDOWS_ARCH_LABEL}-setup.exe"
if [[ ! -f "${EXPECTED_INSTALLER}" ]]; then
  echo "ERROR: expected installer artifact missing: ${EXPECTED_INSTALLER}" >&2
  exit 1
fi

echo "Done. Windows artifacts:"
echo "  ${WINDOWS_OUTPUT_DIR}"
