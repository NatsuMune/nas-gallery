#!/usr/bin/env bash
set -euo pipefail

# Build Synology DSM 7 .spk packages for nas-gallery.
#
# Usage:
#   ./packaging/synology/build-spk.sh [-v 1.0.0-0001] [-o dist] [x86_64|armv8|all]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

VERSION="1.0.0-0001"
OUT_DIR="${REPO_ROOT}/dist"
TARGET="all"

while getopts "v:o:h" opt; do
  case "${opt}" in
    v) VERSION="${OPTARG}" ;;
    o) OUT_DIR="${OPTARG}" ;;
    h)
      sed -n '3,7p' "$0"
      exit 0
      ;;
    *) exit 1 ;;
  esac
done
shift $((OPTIND - 1))
if [ $# -ge 1 ]; then
  TARGET="$1"
fi

# Strip optional leading 'v' and append -0001 build number if no hyphen present.
VERSION="${VERSION#v}"
if [[ "${VERSION}" != *-* ]]; then
  VERSION="${VERSION}-0001"
fi

mkdir -p "${OUT_DIR}"

md5_of() {
  if command -v md5sum >/dev/null 2>&1; then
    md5sum "$1" | awk '{print $1}'
  else
    md5 -q "$1"
  fi
}

build_arch() {
  local spk_arch="$1"
  local go_arch="$2"
  local stage
  stage="$(mktemp -d)"
  trap "rm -rf '${stage}'" RETURN

  local pkg_root="${stage}/pkg"
  local spk_root="${stage}/spk"
  mkdir -p "${pkg_root}/bin" "${spk_root}/conf" "${spk_root}/scripts" "${spk_root}/WIZARD_UIFILES"

  echo "Building nas-gallery (${go_arch} -> DSM ${spk_arch})..."
  (
    cd "${REPO_ROOT}/server"
    CGO_ENABLED=0 GOOS=linux GOARCH="${go_arch}" \
      go build -trimpath -ldflags="-s -w" -o "${pkg_root}/bin/nas-gallery" .
  )
  chmod 0755 "${pkg_root}/bin/nas-gallery"

  local extract_kb
  extract_kb="$(du -sk "${pkg_root}" | awk '{print $1}')"

  COPYFILE_DISABLE=1 tar -C "${pkg_root}" --format=ustar -czf "${spk_root}/package.tgz" bin

  local checksum
  checksum="$(md5_of "${spk_root}/package.tgz")"

  sed \
    -e "s|@VERSION@|${VERSION}|g" \
    -e "s|@ARCH@|${spk_arch}|g" \
    -e "s|@CHECKSUM@|${checksum}|g" \
    -e "s|@EXTRACTSIZE@|${extract_kb}|g" \
    "${SCRIPT_DIR}/INFO.in" > "${spk_root}/INFO"

  cp "${SCRIPT_DIR}/conf/privilege" "${spk_root}/conf/privilege"
  cp "${SCRIPT_DIR}/WIZARD_UIFILES/install_uifile" "${spk_root}/WIZARD_UIFILES/install_uifile"
  cp "${SCRIPT_DIR}/WIZARD_UIFILES/uninstall_uifile" "${spk_root}/WIZARD_UIFILES/uninstall_uifile"
  cp "${SCRIPT_DIR}/scripts/"* "${spk_root}/scripts/"
  chmod 0755 "${spk_root}/scripts/"*
  cp "${SCRIPT_DIR}/PACKAGE_ICON.PNG" "${spk_root}/PACKAGE_ICON.PNG"
  cp "${SCRIPT_DIR}/PACKAGE_ICON_256.PNG" "${spk_root}/PACKAGE_ICON_256.PNG"
  cp "${REPO_ROOT}/LICENSE" "${spk_root}/LICENSE"

  local out_file="${OUT_DIR}/nas-gallery-${VERSION}-${spk_arch}.spk"
  COPYFILE_DISABLE=1 tar -C "${spk_root}" --format=ustar -cf "${out_file}" \
    INFO LICENSE PACKAGE_ICON.PNG PACKAGE_ICON_256.PNG package.tgz conf scripts WIZARD_UIFILES

  echo "Built ${out_file}"
}

case "${TARGET}" in
  x86_64) build_arch x86_64 amd64 ;;
  armv8)  build_arch armv8 arm64 ;;
  all)
    build_arch x86_64 amd64
    build_arch armv8 arm64
    ;;
  *)
    echo "Unknown target: ${TARGET} (expected x86_64, armv8, or all)" >&2
    exit 1
    ;;
esac
