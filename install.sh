#!/usr/bin/env bash
# install.sh — installs meofishes with its base plugins and themes and adds
# it to the applications menu.
#
#   ./install.sh                 system-wide: /opt/meofishes (asks for sudo)
#   ./install.sh --user          only for you: ~/.local/opt/meofishes
#   ./install.sh --uninstall     remove it (add --user for a user install)
#
# It needs a built binary: ./meofishes next to this script (or BIN=path).
# The base plugins and themes are embedded in the binary; the installer runs
# "meofishes setup" so they are ready on the first start.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
BIN=${BIN:-$HERE/meofishes}
MODE=system
UNINSTALL=0
for a in "$@"; do
  case "$a" in
    --user) MODE=user ;;
    --uninstall) UNINSTALL=1 ;;
    -h|--help) sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $a" >&2; exit 2 ;;
  esac
done

if [[ $MODE == system ]]; then
  PREFIX=/opt/meofishes
  LINK=/usr/local/bin/meofishes
  APPS=/usr/share/applications
  ICONS=/usr/share/icons/hicolor/scalable/apps
  SUDO=$([[ $EUID -eq 0 ]] && echo "" || echo sudo)
else
  PREFIX=$HOME/.local/opt/meofishes
  LINK=$HOME/.local/bin/meofishes
  APPS=$HOME/.local/share/applications
  ICONS=$HOME/.local/share/icons/hicolor/scalable/apps
  SUDO=""
fi

refresh_menus() {
  command -v update-desktop-database >/dev/null && $SUDO update-desktop-database "$APPS" >/dev/null 2>&1 || true
  command -v gtk-update-icon-cache >/dev/null && $SUDO gtk-update-icon-cache -q "$(dirname "$(dirname "$ICONS")")" >/dev/null 2>&1 || true
}

if [[ $UNINSTALL == 1 ]]; then
  $SUDO rm -rf "$PREFIX" "$LINK" "$APPS/meofishes.desktop" "$ICONS/meofishes.svg"
  refresh_menus
  echo "meofishes removed. Your settings, library and plugins stay in ~/.config/meofishes and ~/.local/share/meofishes."
  exit 0
fi

[[ -x $BIN ]] || { echo "binary not found: $BIN (build it first, see README)" >&2; exit 1; }
for dep in mpv; do
  command -v $dep >/dev/null || echo "warning: $dep is not installed — meofishes needs it to play music" >&2
done
command -v yt-dlp >/dev/null || echo "note: yt-dlp is not installed — YouTube/SoundCloud will not work" >&2

echo "installing to $PREFIX"
$SUDO mkdir -p "$PREFIX" "$(dirname "$LINK")" "$APPS" "$ICONS"
$SUDO install -m 755 "$BIN" "$PREFIX/meofishes"
$SUDO install -m 644 "$HERE/packaging/meofishes.svg" "$PREFIX/meofishes.svg"
[[ -d $HERE/docs ]] && $SUDO cp -r "$HERE/docs" "$PREFIX/"
$SUDO ln -sf "$PREFIX/meofishes" "$LINK"
$SUDO install -m 644 "$HERE/packaging/meofishes.svg" "$ICONS/meofishes.svg"
sed "s#@BIN@#$PREFIX/meofishes#g" "$HERE/packaging/meofishes.desktop" | $SUDO tee "$APPS/meofishes.desktop" >/dev/null
refresh_menus

# base plugins and themes for the user who runs the installer
if [[ $EUID -ne 0 ]]; then
  "$PREFIX/meofishes" setup >/dev/null 2>&1 && echo "base plugins and themes installed"
fi
echo "done — meofishes is in your applications menu (or run: meofishes)"
