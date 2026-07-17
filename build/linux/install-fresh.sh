#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PREFIX="${PREFIX:-/usr/local}"
DESTDIR="${DESTDIR:-}"

if [[ "${1:-}" == "--user" ]]; then
  PREFIX="$HOME/.local"
  SUDO=()
else
  if [[ -w "${DESTDIR}${PREFIX}" ]]; then
    SUDO=()
  else
    SUDO=(sudo)
  fi
fi

BIN_PATH="${DESTDIR}${PREFIX}/bin/aerion"
DESKTOP_PATH="${DESTDIR}${PREFIX}/share/applications/io.github.hkdb.Aerion.desktop"
ICON_PATH="${DESTDIR}${PREFIX}/share/icons/hicolor/256x256/apps/io.github.hkdb.Aerion.png"

echo "Installing Aerion from current source"
echo "Prefix: ${DESTDIR}${PREFIX}"

cd "$ROOT_DIR"

echo "Removing old launch files"
"${SUDO[@]}" rm -f "$BIN_PATH" "$DESKTOP_PATH" "$ICON_PATH"

echo "Building current source"
make build

echo "Installing binary, desktop entry, and icon"
"${SUDO[@]}" install -Dm755 build/bin/aerion "$BIN_PATH"
"${SUDO[@]}" install -Dm644 build/appicon.png "$ICON_PATH"

tmp_desktop="$(mktemp)"
sed "s|^Exec=.*|Exec=${PREFIX}/bin/aerion %U|; s|^TryExec=.*|TryExec=${PREFIX}/bin/aerion|; s|^Exec=aerion mailto:|Exec=${PREFIX}/bin/aerion mailto:|" \
  build/linux/aerion.desktop > "$tmp_desktop"
"${SUDO[@]}" install -Dm644 "$tmp_desktop" "$DESKTOP_PATH"
rm -f "$tmp_desktop"

echo "Refreshing desktop/icon caches"
gtk-update-icon-cache -f -t "${DESTDIR}${PREFIX}/share/icons/hicolor" >/dev/null 2>&1 || true
update-desktop-database "${DESTDIR}${PREFIX}/share/applications" >/dev/null 2>&1 || true
if command -v kbuildsycoca6 >/dev/null 2>&1; then
  kbuildsycoca6 >/dev/null 2>&1 || true
fi

echo "Installed:"
echo "  $BIN_PATH"
echo "  $DESKTOP_PATH"
echo "  $ICON_PATH"
