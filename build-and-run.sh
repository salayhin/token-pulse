#!/bin/bash

set -e

PORT=${1:-3456}

echo "🛑 Killing existing process on port $PORT..."
lsof -ti :$PORT | xargs kill -9 2>/dev/null || echo "  (no process found)"

sleep 1

echo "🔨 Building Go app..."
make build

if [ ! -f ./bin/claude-token-lens ]; then
  echo "❌ Build failed: binary not found"
  exit 1
fi

echo "✅ Build successful"
echo "🚀 Starting app on port $PORT..."
./bin/claude-token-lens serve
