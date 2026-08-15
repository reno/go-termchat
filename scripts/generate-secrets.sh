#!/bin/bash
set -euo pipefail

# Generate secrets for termchat deployment
# Usage: ./scripts/generate-secrets.sh

echo "🔐 Generating secrets for termchat"
echo ""

# Generate session key
echo "TERMCHAT_SESSION_KEY:"
SESSION_KEY=$(openssl rand -hex 32)
echo "$SESSION_KEY"
echo ""

# Copy to clipboard if available
if command -v pbcopy &> /dev/null; then
    echo "$SESSION_KEY" | pbcopy
    echo "✅ Session key copied to clipboard (macOS)"
    echo ""
elif command -v xclip &> /dev/null; then
    echo "$SESSION_KEY" | xclip -selection clipboard
    echo "✅ Session key copied to clipboard (Linux)"
    echo ""
fi

echo "📋 Use this key in your deployment:"
echo ""
echo "For Fly.io:"
echo "  fly secrets set TERMCHAT_SESSION_KEY=$SESSION_KEY"
echo ""
echo "For .env.local:"
echo "  echo 'TERMCHAT_SESSION_KEY=$SESSION_KEY' >> .env.local"
echo ""
echo "For Docker:"
echo "  docker run -e TERMCHAT_SESSION_KEY=$SESSION_KEY ..."
echo ""
echo "⚠️  Keep this key secret! Do not commit it to git."
echo ""
