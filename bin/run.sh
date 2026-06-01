#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "URL is required. Usage: make run URL=https://example.com" >&2
  exit 1
fi

go run ./cmd/hexlet-go-crawler "$@"
