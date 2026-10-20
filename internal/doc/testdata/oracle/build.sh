#!/bin/sh
# Builds oracle.js, the IIFE bundle of src/entry.js that go test runs in goja.
# The Go helper (oracle_test.go) patches one call when it loads the bundle:
# goja's String.prototype.split leaves empty strings between the tokens of
# prosemirror-model's content-expression tokenizer, which V8 does not.
#
#   npm ci && ./build.sh && node gen-fixtures.mjs
set -e
cd "$(dirname "$0")"
ESBUILD=${ESBUILD:-esbuild}
command -v "$ESBUILD" >/dev/null 2>&1 || ESBUILD=/home/dev/go/bin/esbuild
"$ESBUILD" src/entry.js --bundle --format=iife --target=es2017 --charset=utf8 --legal-comments=none --outfile=oracle.js --log-level=warning
grep -q -F '.split(/\s*(?=\b|\W|$)/)' oracle.js || { echo "oracle.js: the tokenizer's split call changed; update the patch in oracle_test.go" >&2; exit 1; }
ls -l oracle.js
