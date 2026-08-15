#!/bin/bash
set -euo pipefail

echo "🚀 Setting up termchat on Oracle Cloud (Oracle Linux)"
echo ""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

if [ "$EUID" -eq 0 ]; then
    echo -e "${RED}❌ Please do not run as root. Run as the opc user.${NC}"
    exit 1
fi

if [ ! -r /etc/os-release ]; then
    echo -e "${RED}❌ Cannot read /etc/os-release; unsupported image.${NC}"
    exit 1
fi

. /etc/os-release

case "${ID:-}:${ID_LIKE:-}" in
    ol:*|*rhel*|*fedora*) ;;
    *)
        echo -e "${RED}❌ This script targets Oracle Linux / RHEL-family images.${NC}"
        echo -e "${RED}   Detected: ${PRETTY_NAME:-unknown}${NC}"
        echo "   On an Ubuntu image the package manager and firewall differ."
        exit 1
        ;;
esac

APP_USER="$(id -un)"
echo "📦 Image: ${PRETTY_NAME:-unknown} (user: $APP_USER)"
echo ""

echo "📦 Updating system packages..."
sudo dnf -y upgrade --refresh

echo "🔧 Installing dnf plugins..."
sudo dnf -y install dnf-plugins-core

echo "🐳 Installing Docker Engine..."
if command -v docker &> /dev/null; then
    echo -e "${GREEN}✅ Docker already installed${NC}"
else
    sudo dnf config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
    sudo dnf -y install --allowerasing docker-ce docker-ce-cli containerd.io docker-compose-plugin
    echo -e "${GREEN}✅ Docker installed${NC}"
fi

sudo systemctl enable --now docker
sudo usermod -aG docker "$APP_USER"

echo "🔐 Installing Caddy (for HTTPS)..."
if command -v caddy &> /dev/null; then
    echo -e "${GREEN}✅ Caddy already installed${NC}"
elif sudo dnf -y install 'dnf-command(copr)' \
    && sudo dnf -y copr enable @caddy/caddy \
    && sudo dnf -y install caddy; then
    echo -e "${GREEN}✅ Caddy installed${NC}"
else
    echo -e "${YELLOW}⚠️  Caddy install failed; continuing without it.${NC}"
    echo -e "${YELLOW}   HTTPS is a follow-up step, not required to boot termchat.${NC}"
fi

echo "🔥 Configuring firewalld..."
sudo dnf -y install firewalld
sudo systemctl enable --now firewalld
sudo firewall-cmd --permanent --add-service=ssh
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --reload
echo -e "${GREEN}✅ Firewall configured${NC}"

echo "📁 Creating application directory..."
mkdir -p ~/termchat
cd ~/termchat

echo "🔑 Generating session key..."
SESSION_KEY=$(openssl rand -hex 32)

echo "📝 Creating docker-compose.yml..."
cat > docker-compose.yml << 'EOF'
services:
  termchat:
    image: ghcr.io/reno/go-termchat:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      - TERMCHAT_ENV=production
      - TERMCHAT_ADDR=:8080
      - TERMCHAT_SESSION_KEY=${TERMCHAT_SESSION_KEY}
      - TERMCHAT_CORS_ORIGINS=${TERMCHAT_CORS_ORIGINS}
      - TERMCHAT_SESSION_TTL=30m
      - TERMCHAT_MESSAGES_PER_CHANNEL=200
      - TERMCHAT_MESSAGE_TTL=1h
      - TERMCHAT_SWEEP_INTERVAL=1m
      - TERMCHAT_TYPING_TTL=5s
      - TERMCHAT_KEEPALIVE=20s
    healthcheck:
      test: ["CMD", "/termchat", "-healthcheck"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
EOF

echo "📝 Creating .env file..."
echo "TERMCHAT_SESSION_KEY=$SESSION_KEY" > .env
echo "TERMCHAT_CORS_ORIGINS=https://termchat.pages.dev" >> .env
chmod 600 .env

echo ""
echo -e "${YELLOW}⚠️  IMPORTANT: Update TERMCHAT_CORS_ORIGINS in .env with your actual Cloudflare Pages URL${NC}"
echo ""

echo "⚙️  Creating systemd service..."
sudo tee /etc/systemd/system/termchat.service > /dev/null <<EOF
[Unit]
Description=termchat
Requires=docker.service
After=docker.service network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=$HOME/termchat
ExecStart=/usr/bin/docker compose up -d
ExecStop=/usr/bin/docker compose down
User=$APP_USER

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable termchat

echo ""
echo -e "${GREEN}✅ Setup complete!${NC}"
echo ""
echo -e "${YELLOW}⚠️  Log out and back in before running docker without sudo.${NC}"
echo "   The $APP_USER user was added to the docker group, which only"
echo "   applies to new sessions. GitHub Actions opens a fresh session,"
echo "   so CI deploys are unaffected."
echo ""
echo "📋 Next steps:"
echo ""
echo "1. Edit .env to set your Cloudflare Pages URL:"
echo "   nano ~/termchat/.env"
echo ""
echo "2. Start the service:"
echo "   sudo systemctl start termchat"
echo ""
echo "3. Check status:"
echo "   docker compose ps"
echo "   curl http://localhost:8080/healthz"
echo ""
echo "4. (Optional) Configure Caddy for HTTPS:"
echo "   See: docs/cloudflare-deployment.md#configure-caddy"
echo ""
echo "🔐 Your generated session key is in ~/termchat/.env"
echo "   Keep it secret!"
echo ""
