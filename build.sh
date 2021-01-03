#!/bin/bash

set -e
cd "$(dirname $0)"
mkdir -p bin
for d in $(find cmd -mindepth 1 -maxdepth 1 -type d); do
	cd $d
	xn="$(basename "$d")"
	go build
	/bin/mv "$xn" ../../bin
done
