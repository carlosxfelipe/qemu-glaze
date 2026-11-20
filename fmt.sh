#!/usr/bin/env bash
set -e

# 1. Format Go code
if command -v go >/dev/null 2>&1; then
  echo "💧 Formatting Go code..."
  go fmt ./...
else
  echo "⚠️  'go' not found, skipping Go formatting."
fi

# 2. Format web and Markdown files with Prettier (.js, .css, .html, .md)
if command -v prettier >/dev/null 2>&1; then
  echo "✨ Formatting files (.js, .css, .html, .md with prettier)..."
  find . \( -name "*.js" -o -name "*.css" -o -name "*.html" -o -name "*.md" \) -not -path "*/.*" -not -path "*/node_modules/*" -exec prettier --write {} +
elif command -v npx >/dev/null 2>&1; then
  echo "✨ Formatting files (.js, .css, .html, .md with npx prettier)..."
  find . \( -name "*.js" -o -name "*.css" -o -name "*.html" -o -name "*.md" \) -not -path "*/.*" -not -path "*/node_modules/*" -exec npx --yes prettier --write {} +
else
  echo "⚠️  Neither 'prettier' nor 'npx' found. Skipping Prettier formatting."
fi

# 3. Format Shell scripts (prefers native shfmt from brew, falls back to uvx)
if command -v shfmt >/dev/null 2>&1; then
  echo "📜 Formatting shell scripts (shfmt)..."
  shopt -s globstar 2>/dev/null || true
  if shopt -q globstar 2>/dev/null; then
    shfmt -i 2 -w **/*.sh
  else
    find . -name "*.sh" -not -path "*/.*" -exec shfmt -i 2 -w {} +
  fi
elif command -v uvx >/dev/null 2>&1; then
  echo "📜 Formatting shell scripts (uvx shfmt-py)..."
  shopt -s globstar 2>/dev/null || true
  if shopt -q globstar 2>/dev/null; then
    uvx --from shfmt-py shfmt -i 2 -w **/*.sh
  else
    find . -name "*.sh" -not -path "*/.*" -exec uvx --from shfmt-py shfmt -i 2 -w {} +
  fi
else
  echo "⚠️  Neither 'shfmt' nor 'uvx' found. Install with: brew install shfmt"
fi

echo "✅ Done!"
