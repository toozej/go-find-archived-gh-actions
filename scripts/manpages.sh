#!/bin/sh
set -e
rm -rf manpages
mkdir manpages
go run ./cmd/go-find-archived-gh-actions/ man | gzip -c -9 >manpages/go-find-archived-gh-actions.1.gz
