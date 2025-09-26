#!/bin/bash
set -e

# Configuration
SERVER="5.231.56.71"
APP_DIR="/opt/tickit"
BUILD_DIR="./bin"

echo "🔨 Building application..."
# Build for Linux if you're on a different platform
GOOS=linux GOARCH=amd64 go build -o $BUILD_DIR/web ./cmd/web

echo "🌱 Building seed script..."
# Build seed script for database seeding
GOOS=linux GOARCH=amd64 go build -o $BUILD_DIR/seed ./cmd/dev

echo "🗄️ Building migration script..."
# Build migration script for database migrations
GOOS=linux GOARCH=amd64 go build -o $BUILD_DIR/migrate ./cmd/migrate

echo "📦 Building frontend assets..."
# Build frontend (adjust command based on your build system)
go run ./cmd/web/build

echo "📤 Uploading to server..."
rsync -avz --delete \
    --include='/bin/' \
    --include='/bin/**' \
    --include='/web/' \
    --include='/web/**' \
    --include='/templates/' \
    --include='/templates/**' \
    --include='/static/' \
    --include='/static/**' \
    --include='deploy.sh' \
    --exclude='*' \
    ./ $SERVER:$APP_DIR/

echo "🔐 Setting permissions..."
echo "🚀 Starting services..."

ssh $SERVER << 'EOF'
chown -R tickit:tickit /opt/tickit
chmod +x /opt/tickit/bin/web
chmod +x /opt/tickit/bin/seed
chmod +x /opt/tickit/bin/migrate

systemctl daemon-reload
systemctl restart tickit
systemctl restart caddy
systemctl enable tickit
systemctl enable caddy

# Check status
systemctl status tickit --no-pager
systemctl status caddy --no-pager

# Show services running and their ports
echo "📊 Service status and ports:"
ss -tlnp | grep -E ':(80|443|8080|3000)' || echo "No services found on common ports"
ps aux | grep -E '(tickit|caddy)' | grep -v grep
EOF

echo "✅ Deployment complete!"
echo "🌐 Your app should be available at https://tickitdemo.zweiundeins.gmbh"
