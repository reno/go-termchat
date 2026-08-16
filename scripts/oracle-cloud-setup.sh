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

log() {
    echo ""
    echo "[$(date -u +%H:%M:%S)] $*"
}

dnf_do() {
    sudo dnf -y "$@" < /dev/null
}

log "💾 Checking swap..."
MEM_KB=$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)
SWAP_KB=$(awk '/^SwapTotal:/ {print $2}' /proc/meminfo)
if [ "$SWAP_KB" -eq 0 ] && [ "$MEM_KB" -lt 2097152 ]; then
    echo "   $((MEM_KB / 1024)) MB RAM, no swap: creating a 2G swapfile so dnf is not OOM-killed"
    if [ ! -f /swapfile ]; then
        sudo fallocate -l 2G /swapfile || sudo dd if=/dev/zero of=/swapfile bs=1M count=2048 status=none
    fi
    sudo chmod 600 /swapfile
    sudo mkswap /swapfile > /dev/null
    sudo swapon /swapfile
    if ! grep -q '^/swapfile ' /etc/fstab; then
        echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab > /dev/null
    fi
    echo -e "${GREEN}✅ Swap enabled${NC}"
else
    echo -e "${GREEN}✅ $((MEM_KB / 1024)) MB RAM, $((SWAP_KB / 1024)) MB swap${NC}"
fi

log "📦 Refreshing package metadata..."
dnf_do makecache

if [ "${TERMCHAT_SYSTEM_UPGRADE:-0}" = "1" ]; then
    log "📦 Upgrading system packages (30+ minutes on a micro shape)..."
    dnf_do upgrade --refresh
else
    echo -e "${YELLOW}⚠️  Skipping full system upgrade. Re-run with TERMCHAT_SYSTEM_UPGRADE=1 to include it.${NC}"
fi

log "🔧 Installing dnf plugins..."
dnf_do install dnf-plugins-core

log "🐳 Installing Docker Engine..."
if command -v docker &> /dev/null; then
    echo -e "${GREEN}✅ Docker already installed${NC}"
else
    dnf_do config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
    dnf_do install --allowerasing docker-ce docker-ce-cli containerd.io docker-compose-plugin
    echo -e "${GREEN}✅ Docker installed${NC}"
fi

log "🐳 Enabling Docker..."
sudo systemctl enable --now docker
sudo usermod -aG docker "$APP_USER"

log "🔥 Configuring firewalld..."
dnf_do install firewalld
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
echo "TERMCHAT_CORS_ORIGINS=" >> .env
chmod 600 .env

echo ""
echo -e "${YELLOW}⚠️  TERMCHAT_CORS_ORIGINS is empty on purpose.${NC}"
echo "   The binary embeds the PWA, so the app is served same-origin and CORS"
echo "   is never exercised. Only set it if you host the frontend elsewhere."
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
echo "1. Start the service:"
echo "   sudo systemctl start termchat"
echo ""
echo "2. Check status:"
echo "   docker compose ps"
echo "   curl http://localhost:8080/healthz"
echo ""
echo "3. Put HTTPS in front of it:"
echo "   bash scripts/setup-caddy.sh"
echo ""
echo "🔐 Your generated session key is in ~/termchat/.env"
echo "   Keep it secret!"
echo ""
