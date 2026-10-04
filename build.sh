#!/bin/sh
# Cross-compiles static google-multi-auth binaries for every supported
# platform into bin/, then packages one zip per platform into dist/ for a
# GitHub Release, plus a SHA256SUMS file. Only the maintainer needs Go
# installed to run this -- end users download a zip from Releases.
#
# Each zip unpacks to a google-multi-auth/ folder holding the setup script
# for that platform, README.md, LICENSE and bin/<binary>, so the setup
# scripts find the binary exactly as they do in a source checkout.
set -e

ROOT=$(pwd)
rm -rf bin dist
mkdir -p bin dist

build() {
	os="$1"
	arch="$2"
	name="google-multi-auth-${os}-${arch}"
	out="bin/${name}"
	setup="setup"
	if [ "$os" = "windows" ]; then
		out="${out}.exe"
		setup="setup.bat"
	fi
	echo "Building ${os}/${arch} -> ${out}"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w" -o "$out" ./cmd/google-multi-auth

	stage=$(mktemp -d)
	mkdir -p "${stage}/google-multi-auth/bin"
	cp "$setup" README.md LICENSE "${stage}/google-multi-auth/"
	cp "$out" "${stage}/google-multi-auth/bin/"
	(cd "$stage" && zip -qr -X "${ROOT}/dist/${name}.zip" google-multi-auth)
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
mkdir -p "${stage}/google-multi-auth/bin"
cp setup README.md LICENSE "${stage}/google-multi-auth/"
cp bin/google-multi-auth-darwin-arm64 bin/google-multi-auth-darwin-amd64 "${stage}/google-multi-auth/bin/"
(cd "$stage" && zip -qr -X "${ROOT}/dist/google-multi-auth-macos.zip" google-multi-auth)
rm -rf "$stage"

(cd dist && shasum -a 256 *.zip > SHA256SUMS)

echo "Done. Binaries are in bin/, release zips and SHA256SUMS in dist/."
