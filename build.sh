#!/bin/sh
# Cross-compiles static inbox-bridge binaries for every supported
# platform into bin/, then packages one zip per platform into dist/ for a
# GitHub Release, plus a SHA256SUMS file. Only the maintainer needs Go
# installed to run this -- end users download a zip from Releases.
#
# Each zip unpacks to an inbox-bridge/ folder holding the setup script
# for that platform, README.md, LICENSE and bin/<binary>, so the setup
# scripts find the binary exactly as they do in a source checkout.
set -e

ROOT=$(pwd)
rm -rf bin dist
mkdir -p bin dist

build() {
	os="$1"
	arch="$2"
	name="inbox-bridge-${os}-${arch}"
	out="bin/${name}"
	setup="setup"
	if [ "$os" = "windows" ]; then
		out="${out}.exe"
		setup="setup.bat"
	fi
	echo "Building ${os}/${arch} -> ${out}"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w" -o "$out" ./cmd/inbox-bridge

	stage=$(mktemp -d)
	mkdir -p "${stage}/inbox-bridge/bin"
	cp "$setup" README.md LICENSE "${stage}/inbox-bridge/"
	cp "$out" "${stage}/inbox-bridge/bin/"
	(cd "$stage" && zip -qr -X "${ROOT}/dist/${name}.zip" inbox-bridge)
	rm -rf "$stage"
}

build darwin arm64
build darwin amd64
build linux amd64
build linux arm64
build windows amd64

# One zip for every Mac, holding both binaries. ./setup picks the right one
# from uname, so a reader never has to know which chip their Mac has. This is
# the file the website's download button serves on macOS.
stage=$(mktemp -d)
mkdir -p "${stage}/inbox-bridge/bin"
cp setup README.md LICENSE "${stage}/inbox-bridge/"
cp bin/inbox-bridge-darwin-arm64 bin/inbox-bridge-darwin-amd64 "${stage}/inbox-bridge/bin/"
(cd "$stage" && zip -qr -X "${ROOT}/dist/inbox-bridge-macos.zip" inbox-bridge)
rm -rf "$stage"

# Claude Desktop extension (.mcpb, a zip with manifest.json at its root).
# Double-clicking it opens Claude Desktop's install dialog, which asks for the
# Google Client ID and secret declared in manifest.json. One file serves every
# Mac and Windows: a universal macOS binary (lipo) plus the Windows .exe,
# both named server/inbox-bridge so the manifest needs one command.
# lipo ships with macOS (Xcode command line tools), so build this on a Mac.
stage=$(mktemp -d)
mkdir -p "${stage}/server"
cp manifest.json LICENSE README.md "${stage}/"
lipo -create -output "${stage}/server/inbox-bridge" \
	bin/inbox-bridge-darwin-arm64 bin/inbox-bridge-darwin-amd64
cp bin/inbox-bridge-windows-amd64.exe "${stage}/server/inbox-bridge.exe"
(cd "$stage" && zip -qr -X "${ROOT}/dist/inbox-bridge.mcpb" .)
rm -rf "$stage"

(cd dist && shasum -a 256 *.zip *.mcpb > SHA256SUMS)

echo "Done. Binaries are in bin/, release zips and SHA256SUMS in dist/."
