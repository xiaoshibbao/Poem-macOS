#!/usr/bin/env bash
# Builds dist/Poem.app (universal: arm64 + x86_64) and dist/Poem-<version>-macOS.zip.
# Requires macOS with Xcode command line tools, Go, Node.js + pnpm and Zig 0.13.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="$(node -p "require('./frontend/package.json').version")"
ZIG="${ZIG:-zig}"

echo "==> Frontend"
(cd frontend && pnpm install --frozen-lockfile && pnpm run build)

echo "==> poem-helper.exe (runs inside Wine)"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o assets/poem-helper.exe ./cmd/poem-helper

echo "==> LuaEntry.dll"
"$ZIG" cc -O2 -s -target x86_64-windows-gnu -shared \
  native/lua-entry/lua_entry.c \
  native/lua-entry/minhook/src/buffer.c native/lua-entry/minhook/src/hook.c \
  native/lua-entry/minhook/src/trampoline.c native/lua-entry/minhook/src/hde/hde64.c \
  -o assets/LuaEntry.dll -luser32
rm -f assets/LuaEntry.lib assets/LuaEntry.pdb

echo "==> Tests"
go test ./internal/...

echo "==> Launcher"
mkdir -p dist/build
for arch in arm64 amd64; do
  clang_arch="$arch"; [ "$arch" = amd64 ] && clang_arch=x86_64
  CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" CC="clang -arch $clang_arch" \
    MACOSX_DEPLOYMENT_TARGET=11.0 CGO_CFLAGS="-mmacosx-version-min=11.0" CGO_LDFLAGS="-mmacosx-version-min=11.0" \
    go build -buildvcs=false -tags production -trimpath -ldflags '-s -w' -o "dist/build/Poem-$arch" .
done

echo "==> Poem.app"
APP=dist/Poem.app
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
lipo -create -output "$APP/Contents/MacOS/Poem" dist/build/Poem-arm64 dist/build/Poem-amd64
sed "s/@VERSION@/$VERSION/g" build/darwin/Info.plist > "$APP/Contents/Info.plist"

ICONSET=dist/build/Poem.iconset
rm -rf "$ICONSET" && mkdir -p "$ICONSET"
for size in 16 32 128 256 512; do
  sips -z $size $size assets/appicon.png --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
  sips -z $((size * 2)) $((size * 2)) assets/appicon.png --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/Poem.icns"

# Ad-hoc signature: required on Apple Silicon; not notarised.
codesign --force --deep --sign - "$APP"

cp README.md LICENSE dist/
mkdir -p dist/third-party
cp native/lua-entry/minhook/LICENSE.txt dist/third-party/MinHook-LICENSE.txt
(cd dist && rm -f "Poem-$VERSION-macOS.zip" && ditto -c -k --keepParent Poem.app "Poem-$VERSION-macOS.zip" \
  && zip -q -r "Poem-$VERSION-macOS.zip" README.md LICENSE third-party)
echo "Built dist/Poem.app and dist/Poem-$VERSION-macOS.zip"
