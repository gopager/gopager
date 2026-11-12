#!/bin/sh
# Builds the first-party plugins into <outdir>/<id>/<version>/ with a manifest.json whose
# artifact digest is the SHA-256 of the built executable.
#
# Usage: plugins/build.sh <outdir>
set -eu

version=1.0.0
plugins="pushover webhook"

if [ "$#" -ne 1 ]; then
	echo "usage: $0 <outdir>" >&2
	exit 2
fi
root=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$1"
outdir=$(cd "$1" && pwd)

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	else
		shasum -a 256 "$1" | cut -d ' ' -f 1
	fi
}

for id in $plugins; do
	target="$outdir/$id/$version"
	mkdir -p "$target"
	(cd "$root/$id" && CGO_ENABLED=0 go build -trimpath -o "$target/$id" .)
	digest=$(sha256_of "$target/$id")
	sed "s/@DIGEST@/$digest/" "$root/$id/manifest.template.json" >"$target/manifest.json"
done
