#!/bin/bash
set -euo pipefail

# Deploy termchat web client to Cloudflare Pages
# Usage: ./scripts/deploy-pages.sh [branch]

BRANCH="${1:-main}"
PROJECT_NAME="${CLOUDFLARE_PROJECT_NAME:-termchat}"

echo "🚀 Deploying termchat to Cloudflare Pages"
echo "   Project: $PROJECT_NAME"
echo "   Branch: $BRANCH"
echo ""

# Check if wrangler is installed
if ! command -v wrangler &> /dev/null; then
    echo "❌ Error: wrangler CLI not found"
    echo "   Install with: npm install -g wrangler"
    exit 1
fi

# Check if authenticated
if ! wrangler whoami &> /dev/null; then
    echo "❌ Error: Not authenticated with Cloudflare"
    echo "   Run: wrangler login"
    exit 1
fi

# Build the web client
echo "📦 Building web client..."
cd web

if ! command -v pnpm &> /dev/null; then
    echo "❌ Error: pnpm not found"
    echo "   Run: corepack enable pnpm"
    exit 1
fi

pnpm install --frozen-lockfile
pnpm run build

echo ""
echo "✅ Build complete"
echo ""

# Deploy to Cloudflare Pages
echo "🌐 Deploying to Cloudflare Pages..."
wrangler pages deploy dist \
    --project-name="$PROJECT_NAME" \
    --branch="$BRANCH" \
    --commit-dirty=true

echo ""
echo "✅ Deployment complete!"
echo ""
echo "📋 Next steps:"
echo "   1. Set environment variables in Cloudflare Pages dashboard:"
echo "      - VITE_API_BASE_URL: Your backend URL"
echo "   2. Configure custom domain (optional)"
echo "   3. Update backend CORS settings to allow your Pages URL"
echo ""
