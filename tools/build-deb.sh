#!/usr/bin/env bash
# Build loimo for Linux and package it as a Debian .deb
# (system libs are used; Depends: libgtk-3-0, libayatana-appindicator3-1).
#
# Requirements:
#   - Go >= 1.26, gcc, pkg-config
#   - dpkg-deb (Debian/Ubuntu/Deepin etc.)
#
# Usage:
#   tools/build-deb.sh [VERSION]      # e.g. tools/build-deb.sh 1.2.2
set -euo pipefail

VERSION="${1:-1.2.1-linux}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
BUILD="$ROOT/build"
PKGDIR="$BUILD/deb-root/loimo_${VERSION}_amd64"
OUT="$DIST/loimo_${VERSION}_amd64.deb"

echo ">> building binary"
mkdir -p "$DIST"
(cd "$ROOT" && go build -trimpath \
  -ldflags "-s -w \
    -X github.com/chipmax/loimo/internal/gateway.Version=$VERSION \
    -X github.com/chipmax/loimo/internal/update.Current=$VERSION" \
  -o "$DIST/loimo-linux" ./cmd/loimo)

echo ">> assembling package tree"
rm -rf "$(dirname "$PKGDIR")"
mkdir -p "$PKGDIR/DEBIAN" \
         "$PKGDIR/usr/bin" \
         "$PKGDIR/usr/share/applications" \
         "$PKGDIR/usr/share/icons/hicolor/256x256/apps" \
         "$PKGDIR/usr/share/doc/loimo" \
         "$PKGDIR/usr/share/metainfo"
cp "$DIST/loimo-linux" "$PKGDIR/usr/bin/loimo"
cp "$ROOT/assets/icon-256.png" "$PKGDIR/usr/share/icons/hicolor/256x256/apps/loimo.png"

# --- desktop entry ----------------------------------------------------------
cat > "$PKGDIR/usr/share/applications/loimo.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=Lối Mở
Comment=本地免费模型网关
Exec=loimo
Icon=loimo
Terminal=false
Categories=Network;Utility;
StartupWMClass=loimo
X-GNOME-Autostart-enabled=false
EOF

# --- control -----------------------------------------------------------------
SIZE_KB=$(( $(stat -c%s "$DIST/loimo-linux") / 1024 + 200 ))
cat > "$PKGDIR/DEBIAN/control" <<EOF
Package: loimo
Version: $VERSION
Architecture: amd64
Maintainer: Lối Mở Linux Port <loimo@localhost>
Installed-Size: $SIZE_KB
Depends: libc6 (>= 2.34), libgtk-3-0 (>= 3.24), libayatana-appindicator3-1 (>= 0.5)
Section: net
Priority: optional
Homepage: https://github.com/chipmax/OpenKi
Description: Local free-model gateway with a tray icon
 Lối Mở wraps the OpenCode Zen free model lane into standard
 OpenAI/Anthropic-compatible APIs on 127.0.0.1, auto-detects and injects
 configuration into installed AI agents (OpenCode, Codex, Claude Code,
 Aider, Qwen Code, etc.), and stays resident in the system tray. The
 management dashboard opens in the default browser.
 .
 Linux features: XDG autostart, libnotify notifications, system proxy
 via environment variables, pgrep-based agent detection.
EOF

# --- postinst ----------------------------------------------------------------
cat > "$PKGDIR/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database /usr/share/applications >/dev/null 2>&1 || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f -t /usr/share/icons/hicolor >/dev/null 2>&1 || true
fi
exit 0
EOF
chmod 755 "$PKGDIR/DEBIAN/postinst"

# --- docs --------------------------------------------------------------------
cat > "$PKGDIR/usr/share/doc/loimo/copyright" <<'EOF'
Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: loimo
Upstream-Contact: https://github.com/chipmax/OpenKi
Source: https://github.com/chipmax/OpenKi

Files: *
Copyright: 2024-2026 LAGcomcom
           2026 chipmax (bản fork loimo)
License: GPL-3.0
 This program is free software: you can redistribute it and/or modify it
 under the terms of the GNU General Public License as published by the Free
 Software Foundation, either version 3 of the License, or (at your option)
 any later version.
 .
 On Debian systems the complete license text is available at
 /usr/share/common-licenses/GPL-3.
EOF

cat > "$PKGDIR/usr/share/doc/loimo/changelog" <<EOF
loimo ($VERSION) unstable; urgency=medium

  * Linux 适配：托盘常驻、浏览器打开管理页、XDG 开机自启、
    libnotify 通知、系统代理环境变量、pgrep 进程检测。
  * 打包为自包含 AppImage 与 Debian .deb。

 -- Lối Mở Linux Port <loimo@localhost>  $(date -R)
EOF

cat > "$PKGDIR/usr/share/metainfo/loimo.metainfo.xml" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>io.github.chipmax.loimo</id>
  <name>Lối Mở</name>
  <summary>本地免费模型网关（托盘常驻）</summary>
  <metadata_license>MIT</metadata_license>
  <project_license>GPL-3.0</project_license>
  <description>
    <p>把 OpenCode Zen 免费模型封装成标准 OpenAI / Anthropic 兼容接口，自动注入已安装的 AI Agent 配置，托盘常驻。</p>
  </description>
  <url type="homepage">https://github.com/chipmax/OpenKi</url>
  <launchable type="desktop-id">loimo.desktop</launchable>
  <provides>
    <binary>loimo</binary>
  </provides>
</component>
EOF

# --- build -------------------------------------------------------------------
echo ">> building $OUT"
dpkg-deb --build --root-owner-group "$PKGDIR" "$OUT"
echo ">> done: $OUT"
