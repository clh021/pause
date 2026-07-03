#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-Pause}"
APP_ICON_SOURCE="${APP_ICON_SOURCE:-${ROOT_DIR}/assets/branding/app-icon-1024.png}"
LINUX_PLATFORM="${LINUX_PLATFORM:-linux/amd64}"
LINUX_ARCH_LABEL="${LINUX_ARCH_LABEL:-}"
LINUX_OUTPUT_DIR="${LINUX_OUTPUT_DIR:-}"
APP_VERSION_OVERRIDE="${APP_VERSION_OVERRIDE:-}"
WAILS_TAGS="${WAILS_TAGS:-wails}"
USE_CLEAN="${USE_CLEAN:-0}"
VITE_UPDATES_URL="${VITE_UPDATES_URL:-}"
ARTIFACT_VERSION=""

print_help() {
  cat <<'EOF'
Usage:
  ./scripts/build-linux-bundle.sh [options]

Options:
  --platform <linux/amd64|linux/arm64|...>  Build target platform
  --arch-label <label>                       Artifact folder label (default derived from platform)
  --output-dir <path>                        Artifact output dir
  --version <version>                        Version used in output filename
  --icon <path>                              App icon source (png)
  --tags <go_build_tags>                     Build tags (default: wails)
  --clean                                    Enable wails -clean
  --no-clean                                 Disable wails -clean (default)
  -h, --help                                 Show this help

Environment variables:
  APP_ICON_SOURCE, LINUX_PLATFORM, LINUX_ARCH_LABEL, LINUX_OUTPUT_DIR,
  APP_VERSION_OVERRIDE, WAILS_TAGS, USE_CLEAN, VITE_UPDATES_URL
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --platform)
      LINUX_PLATFORM="$2"
      shift 2
      ;;
    --arch-label)
      LINUX_ARCH_LABEL="$2"
      shift 2
      ;;
    --output-dir)
      LINUX_OUTPUT_DIR="$2"
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
    --tags)
      WAILS_TAGS="$2"
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

default_arch_label_from_platform() {
  case "${LINUX_PLATFORM}" in
    linux/amd64) echo "linux-x64" ;;
    linux/arm64) echo "linux-arm64" ;;
    linux/386) echo "linux-x86" ;;
    *) echo "linux-build" ;;
  esac
}

ARTIFACT_VERSION="$(resolve_artifact_version)"

if [[ -z "${LINUX_ARCH_LABEL}" ]]; then
  LINUX_ARCH_LABEL="$(default_arch_label_from_platform)"
fi
if [[ -z "${LINUX_OUTPUT_DIR}" ]]; then
  LINUX_OUTPUT_DIR="${ROOT_DIR}/build/bin/${LINUX_ARCH_LABEL}"
fi

cd "${ROOT_DIR}"

echo "build config:"
echo "  app_name=${APP_NAME}"
echo "  linux_platform=${LINUX_PLATFORM}"
echo "  linux_arch_label=${LINUX_ARCH_LABEL}"
echo "  linux_output_dir=${LINUX_OUTPUT_DIR}"
echo "  artifact_version=${ARTIFACT_VERSION}"
echo "  app_icon_source=${APP_ICON_SOURCE}"
echo "  wails_tags=${WAILS_TAGS}"
echo "  use_clean=${USE_CLEAN}"

if [[ ! -f "${APP_ICON_SOURCE}" ]]; then
  echo "ERROR: app icon source not found: ${APP_ICON_SOURCE}" >&2
  exit 1
fi

mkdir -p "${ROOT_DIR}/build/bin"
mkdir -p "${LINUX_OUTPUT_DIR}"

# Copy frontend dist into embed directory so the binary includes the web UI.
if [[ -d "${ROOT_DIR}/frontend/dist" ]] && [[ -f "${ROOT_DIR}/frontend/dist/index.html" ]]; then
  mkdir -p "${ROOT_DIR}/internal/remoteserver/static"
  cp -r "${ROOT_DIR}/frontend/dist/"* "${ROOT_DIR}/internal/remoteserver/static/"
  echo "frontend dist copied into embed directory"
fi

STAMP_FILE="$(mktemp /tmp/pause-linux-build-stamp-XXXXXX)"
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
  -platform "${LINUX_PLATFORM}"
  -tags "${WAILS_TAGS}"
)
if [[ "${USE_CLEAN}" == "1" ]]; then
  WAILS_ARGS+=(-clean)
fi

echo "[1/3] Building ${APP_NAME} Linux bundle (${LINUX_PLATFORM})"
if [[ -n "${VITE_UPDATES_URL}" ]]; then
  VITE_UPDATES_URL="${VITE_UPDATES_URL}" "${WAILS_CMD[@]}" "${WAILS_ARGS[@]}"
else
  "${WAILS_CMD[@]}" "${WAILS_ARGS[@]}"
fi

SOURCE_BINARY="${ROOT_DIR}/build/bin/${APP_NAME}"
if [[ ! -f "${SOURCE_BINARY}" || ! "${SOURCE_BINARY}" -nt "${STAMP_FILE}" ]]; then
  SOURCE_BINARY=""
  while IFS= read -r -d '' file; do
    SOURCE_BINARY="${file}"
    break
  done < <(
    find "${ROOT_DIR}/build/bin" \
      -maxdepth 1 \
      -type f \
      -perm -u+x \
      -newer "${STAMP_FILE}" \
      ! -name "*.exe" \
      ! -name "*.msi" \
      ! -name "*.zip" \
      ! -name "*.blockmap" \
      ! -name "*.msix" \
      ! -name "*.appx" \
      -print0
  )
fi

if [[ ! -f "${SOURCE_BINARY}" ]]; then
  echo "ERROR: expected Linux binary missing after build: ${ROOT_DIR}/build/bin/${APP_NAME}" >&2
  exit 1
fi

BUNDLE_NAME="${APP_NAME}-v${ARTIFACT_VERSION}-${LINUX_ARCH_LABEL}"
STAGING_DIR="${LINUX_OUTPUT_DIR}/${BUNDLE_NAME}"
ARCHIVE_PATH="${LINUX_OUTPUT_DIR}/${BUNDLE_NAME}.tar.gz"

echo "[2/3] Preparing portable bundle in ${STAGING_DIR}"
rm -rf "${STAGING_DIR}"
mkdir -p "${STAGING_DIR}"

cp "${SOURCE_BINARY}" "${STAGING_DIR}/${APP_NAME}"
chmod +x "${STAGING_DIR}/${APP_NAME}"
cp "${APP_ICON_SOURCE}" "${STAGING_DIR}/pause.png"

cat > "${STAGING_DIR}/Pause.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Version=1.0
Name=Pause
Comment=Pause break reminder
Exec=sh -c 'APPDIR="$(CDPATH= cd -- "$(dirname -- "$1")" && pwd)"; exec "$APPDIR/Pause"' pause %k
Icon=pause
Terminal=false
Categories=Utility;
StartupNotify=true
EOF

cat > "${STAGING_DIR}/README-linux.txt" <<'EOF'
Pause Linux portable bundle
===========================

Quick start
-----------
1. Extract this archive.
2. Run: ./Pause

Arch Linux / KDE runtime dependencies
-------------------------------------
If launch fails because shared libraries are missing, install:

  sudo pacman -S --needed gtk3 webkit2gtk

Desktop launcher
----------------
- `Pause.desktop` works when it stays next to the `Pause` binary.
- On KDE you can mark the desktop file as executable/trusted, then launch it directly.
- If you copy `Pause.desktop` somewhere else, edit its `Exec=` and `Icon=` entries to absolute paths first.
EOF

echo "[3/3] Creating archive ${ARCHIVE_PATH}"
rm -f "${ARCHIVE_PATH}"
tar -C "${LINUX_OUTPUT_DIR}" -czf "${ARCHIVE_PATH}" "${BUNDLE_NAME}"

if [[ ! -f "${ARCHIVE_PATH}" ]]; then
  echo "ERROR: expected Linux archive missing: ${ARCHIVE_PATH}" >&2
  exit 1
fi

echo "Done. Linux artifacts:"
echo "  ${LINUX_OUTPUT_DIR}"
