#!/bin/bash
# Render Kitter's pages and dialogs to PNGs without a display or root:
# downloads Pango, cairo, fontconfig and fonts as Debian packages into a
# cache, unpacks them, and points the headless tester at them.
# Usage: scripts/ui-shots.sh [out-dir]   (Debian/Ubuntu with apt-get)
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
out="${1:-$root/native/build/shots}"
cache="${KITTER_SHOTS_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/kitter-ui-shots}"
sys="$cache/root"
if [[ ! -e "$sys/.done" ]]; then
  mkdir -p "$cache/debs" "$sys"
  roots="libpangocairo-1.0-0 libpangoft2-1.0-0 fonts-noto-cjk fonts-inter fontconfig-config"
  pkgs=$(apt-cache depends --recurse --no-recommends --no-suggests --no-conflicts \
    --no-breaks --no-replaces --no-enhances $roots | grep '^\w' | sort -u)
  need=()
  for p in $pkgs; do dpkg -s "$p" >/dev/null 2>&1 || need+=("$p"); done
  if ((${#need[@]})); then
    (cd "$cache/debs" && apt-get download "${need[@]}")
  fi
  for deb in "$cache"/debs/*.deb; do [[ -e "$deb" ]] && dpkg -x "$deb" "$sys"; done
  touch "$sys/.done"
fi
cat > "$cache/fonts.conf" <<CONF
<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "fonts.dtd">
<fontconfig>
  <dir>/usr/share/fonts</dir>
  <dir>$sys/usr/share/fonts</dir>
  <cachedir>$cache/fc-cache</cachedir>
  <!-- Stand in for macOS's SF Pro / PingFang so layouts read alike. -->
  <alias binding="strong"><family>system-ui</family><prefer><family>Inter</family><family>Noto Sans CJK SC</family></prefer></alias>
  <alias binding="strong"><family>sans-serif</family><prefer><family>Inter</family><family>Noto Sans CJK SC</family></prefer></alias>
  <include ignore_missing="yes">/etc/fonts/conf.d</include>
  <include ignore_missing="yes">$sys/etc/fonts/conf.d</include>
</fontconfig>
CONF
rm -rf "$out"
cd "$root/native"
# One process per appearance: the bundled mono font only draws in the
# first Tester a process creates.
for mode in light dark; do
  LD_LIBRARY_PATH="$sys/usr/lib/$(uname -m)-linux-gnu${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}" \
  FONTCONFIG_FILE="$cache/fonts.conf" KITTER_SHOTS="$out" \
    go test ./app -run "^TestScreenshots\$/^$mode\$" -count=1
done
echo "screenshots: $out"
