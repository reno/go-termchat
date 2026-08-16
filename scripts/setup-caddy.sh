#!/bin/bash
set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

CADDY_DIR="${CADDY_DIR:-$HOME/caddy}"
UPSTREAM="${TERMCHAT_UPSTREAM:-127.0.0.1:8080}"

echo "🔐 Setting up Caddy with nip.io"
echo ""

if ! docker info > /dev/null 2>&1; then
    echo -e "${RED}❌ Cannot talk to the Docker daemon as $(id -un).${NC}"
    echo "   Run 'newgrp docker', or log out and back in."
    exit 1
fi

PUBLIC_IP="${PUBLIC_IP:-}"

if [ -z "$PUBLIC_IP" ]; then
    PUBLIC_IP=$(curl -fsS --max-time 10 -H "Authorization: Bearer Oracle" \
        http://169.254.169.254/opc/v2/vnics/ 2>/dev/null \
        | tr ',' '\n' | grep '"publicIp"' | head -1 | cut -d'"' -f4 || true)
fi

if [ -z "$PUBLIC_IP" ]; then
    PUBLIC_IP=$(curl -fsS --max-time 10 https://ifconfig.me 2>/dev/null || true)
fi

if [[ ! "$PUBLIC_IP" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]]; then
    echo -e "${RED}❌ Could not determine the public IPv4 address.${NC}"
    echo "   Re-run with it explicit:  PUBLIC_IP=1.2.3.4 $0"
    exit 1
fi

NIP_HOST="${PUBLIC_IP}.nip.io"

echo "🌐 Public IP: $PUBLIC_IP"
echo "🌐 Hostname:  $NIP_HOST"
echo ""

if ! getent hosts "$NIP_HOST" > /dev/null 2>&1; then
    echo -e "${YELLOW}⚠️  $NIP_HOST does not resolve from this host.${NC}"
    echo "   Continuing anyway; Let's Encrypt resolves it independently."
    echo ""
fi

echo "📝 Writing $CADDY_DIR ..."
mkdir -p "$CADDY_DIR"
cd "$CADDY_DIR"

cat > Caddyfile <<EOF
$NIP_HOST {
    reverse_proxy $UPSTREAM
}
EOF

cat > docker-compose.yml <<'EOF'
services:
  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    network_mode: host
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro,z
      - caddy_data:/data
      - caddy_config:/config

volumes:
  caddy_data:
  caddy_config:
EOF

echo "🚀 Starting Caddy..."
docker compose up -d --force-recreate

echo ""
echo "⏳ Waiting for the certificate (up to 90s)..."

for _ in $(seq 1 45); do
    CODE=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 \
        "https://$NIP_HOST/healthz" 2>/dev/null || echo 000)

    if [ "$CODE" = "200" ]; then
        echo -e "${GREEN}✅ HTTPS is live: https://$NIP_HOST${NC}"
        echo ""
        echo "📋 Next steps:"
        echo ""
        echo "1. Point the frontend at it:"
        echo "   gh secret set VITE_API_BASE_URL --body https://$NIP_HOST"
        echo ""
        echo "2. Allow it as a CORS origin:"
        echo "   sed -i 's|^TERMCHAT_CORS_ORIGINS=.*|TERMCHAT_CORS_ORIGINS=https://$NIP_HOST|' ~/termchat/.env"
        echo "   sudo systemctl restart termchat"
        echo ""
        echo "The instance also serves the PWA itself, so https://$NIP_HOST is the whole app."
        exit 0
    fi

    if [ "$CODE" != "000" ]; then
        echo -e "${YELLOW}⚠️  TLS works but termchat answered HTTP $CODE.${NC}"
        echo "   The certificate is fine; the upstream at $UPSTREAM is not."
        echo "   Check: sudo systemctl status termchat"
        exit 1
    fi

    sleep 2
done

echo -e "${YELLOW}⚠️  No certificate after 90s.${NC}"
echo ""
echo "The usual cause is ports 80 and 443 not being open in the OCI Security List."
echo "Let's Encrypt must reach port 80 from the internet for the HTTP-01 challenge,"
echo "and firewalld alone does not cover that — OCI filters upstream of the instance."
echo ""
echo "  OCI Console -> Networking -> Virtual Cloud Networks -> your VCN"
echo "  -> Subnets -> your subnet -> Security List -> Add Ingress Rules"
echo "  Source 0.0.0.0/0, IP Protocol TCP, destination ports 80 and 443"
echo ""
echo "Then watch the handshake:"
echo "  docker compose -f $CADDY_DIR/docker-compose.yml logs -f caddy"
exit 1
