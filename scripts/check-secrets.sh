#!/bin/bash

set -e

is_encrypted() {
  local file="$1"
  if grep -q -E '("sops":|sops:)' "$file"; then
    return 0
  else
    return 1
  fi
}

if [ $# -gt 0 ]; then
  FILE="$1"
  
  if [ ! -f "$FILE" ]; then
    echo "Error: File '$FILE' does not exist."
    exit 1
  fi
  
  if is_encrypted "$FILE"; then
    exit 0
  else
    exit 1
  fi
fi

echo "🕵️  Searching for files matching '*.secrets.*'..."

FILES=$(find . -type f -name "*.secrets.*" -not -path "./.git/*" -not -path "./.github/*")

if [ -z "$FILES" ]; then
  echo "✅ No files found matching '*.secrets.*'. Check passed."
  exit 0
fi

echo "🔍 Found potential secret files:"
echo "$FILES"
echo "-------------------------------------"

UNENCRYPTED_FILES=()

for file in $FILES; do
  if is_encrypted "$file"; then
    echo "✅ '$file' appears to be encrypted with SOPS."
  else
    echo "❌ ERROR: '$file' is NOT encrypted with SOPS."
    UNENCRYPTED_FILES+=("$file")
  fi
done

if [ ${#UNENCRYPTED_FILES[@]} -ne 0 ]; then
  echo "-------------------------------------"
  echo "🔥 Found unencrypted secret files! Please encrypt them with SOPS before committing."
  for file in "${UNENCRYPTED_FILES[@]}"; do
    echo "- $file"
  done
  exit 1
fi

echo "-------------------------------------"
echo "✨ All potential secret files are correctly encrypted. Good job!"
