#!/usr/bin/env sh

set -eu

# CI providers usually export one of these variables.
if [ -n "${GITHUB_ACTIONS:-}" ]; then
  echo "ci"
  exit 0
fi

echo "local"
exit 1
