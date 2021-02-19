#!/bin/bash

set -e
cd "$(dirname $0)"
for arch in 386 amd64; do
	mkdir -p bin/$arch
	for d in $(find cmd -mindepth 1 -maxdepth 1 -type d); do
		pushd $d >/dev/null
		xn="$(basename "$d")"
		GOARCH=$arch go build
		/bin/mv "$xn" ../../bin/$arch
		popd >/dev/null
	done
done