#!/bin/bash
set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

CADDY_DIR="${CADDY_DIR:-$HOME/caddy}"
UPSTREAM="${TERMCHAT_UPSTREAM:-127.0.0.1:8080}"

echo "🔐 Setting up Caddy"
echo "   Override the hostname with:  DOMAIN=chat.example.com $0"
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

SITE_HOST="${DOMAIN:-${PUBLIC_IP}.nip.io}"

echo "🌐 Public IP: $PUBLIC_IP"
echo "🌐 Hostname:  $SITE_HOST"
echo ""

RESOLVED=$(getent hosts "$SITE_HOST" 2>/dev/null | awk '{print $1}' | head -1 || true)

if [ -z "$RESOLVED" ]; then
    echo -e "${YELLOW}⚠️  $SITE_HOST does not resolve from this host.${NC}"
    echo "   If you just created the DNS record, give it a minute."
    echo ""
elif [ "$RESOLVED" != "$PUBLIC_IP" ]; then
    echo -e "${RED}❌ $SITE_HOST resolves to $RESOLVED, not $PUBLIC_IP.${NC}"
    echo ""
    echo "   With Cloudflare this almost always means the record is proxied"
    echo "   (orange cloud) and you are seeing a Cloudflare edge address."
    echo "   Set the record to DNS only (grey cloud): Caddy needs to answer"
    echo "   the HTTP-01 challenge on your origin directly."
    echo ""
    if [ "${SKIP_DNS_CHECK:-0}" != "1" ]; then
        echo "   Override if this is deliberate:  SKIP_DNS_CHECK=1 $0"
        exit 1
    fi
    echo -e "${YELLOW}   SKIP_DNS_CHECK=1 is set; continuing anyway.${NC}"
    echo ""
fi

echo "📝 Writing $CADDY_DIR ..."
mkdir -p "$CADDY_DIR"
cd "$CADDY_DIR"

cat > Caddyfile <<EOF
$SITE_HOST {
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
        "https://$SITE_HOST/healthz" 2>/dev/null || echo 000)

    if [ "$CODE" = "200" ]; then
        echo -e "${GREEN}✅ HTTPS is live: https://$SITE_HOST${NC}"
        echo ""
        echo "The binary embeds the PWA, so that URL is the whole app — served"
        echo "same-origin, which is why TERMCHAT_CORS_ORIGINS stays empty."
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
