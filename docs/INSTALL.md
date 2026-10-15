# Tickit Deployment Guide

This guide covers deploying the Tickit application to a VPS using Caddy as a reverse proxy for optimal performance, HTTP/3 support, and multi-app hosting capability.

## Architecture Overview

```
Internet → Caddy (Port 80/443) → Go App (Port 7105)
```

**Benefits:**
- HTTP/3 support via Caddy
- Automatic HTTPS certificates (Let's Encrypt)
- Brotli compression
- Excellent SSE (Server-Sent Events) support
- Multi-app hosting capability
- Static file serving optimization
- Enhanced security

## Prerequisites

- VPS with Ubuntu/Debian
- Domain name pointing to your VPS
- Root or sudo access

## Step 1: Application Configuration

### Modify Go Application for Production

Update your Go application to remove built-in HTTPS/HTTP3 handling and bind to localhost only:

```go
// In cmd/web/main.go - modify the server setup:
httpSrv := &http.Server{
    Addr:     "127.0.0.1:" + config.Global.HttpPort, // Bind to localhost only
    Handler:  router,
    ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
}
```

### Update Configuration

Set your HTTP port to 7105 in your config:

```go
// Set HttpPort to "7105" and remove HttpsPort configuration
```

## Step 2: VPS Setup

### Install Dependencies

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Caddy with Brotli support
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update

# Install Caddy with xcaddy to add Brotli encoder
sudo apt install -y golang-go libbrotli-dev
go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
sudo CGO_ENABLED=1 ~/go/bin/xcaddy build --with github.com/dunglas/caddy-cbrotli --output /usr/bin/caddy

# Or install standard Caddy if Brotli is already included
# sudo apt install caddy

# Install other dependencies
sudo apt install -y git build-essential rsync
```

### Create Application Directory

```bash
sudo mkdir -p /opt/tickit
sudo chown $USER:$USER /opt/tickit
```

### Create System User

```bash
sudo useradd --system --shell /bin/false --home /opt/tickit tickit
sudo chown -R tickit:tickit /opt/tickit
sudo mkdir -p /opt/tickit/data /opt/tickit/upload
sudo chown -R tickit:tickit /opt/tickit/data /opt/tickit/upload
```

## Step 3: Caddy Configuration

The host's Caddyfile imports every file in `/etc/caddy/sites/`, and the site block is [`deploy/tickitdemo.caddy`](../deploy/tickitdemo.caddy): it proxies to the app on `127.0.0.1:7105`, serves `/static/` and `/uploads/` from `/opt/tickit`, sets the security headers, and writes its access log as JSON to `/var/log/caddy/tickitdemo-access.log`. It goes in by hand, since Caddy asks for the certificate as it loads the block. As root, from a copy of `deploy/`:

```bash
install -m 0644 tickitdemo.caddy /etc/caddy/sites/
runuser -u caddy -- env HOME=/var/lib/caddy caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
systemctl reload caddy
```

Validate as Caddy's user: `caddy validate`, run as root, creates the log file owned by root, and the reload then fails.

## Step 4: Systemd Service

Create `/etc/systemd/system/tickit.service`:

```ini
[Unit]
Description=Tickit Application
After=network.target
Wants=network.target

[Service]
Type=simple
User=tickit
Group=tickit
WorkingDirectory=/opt/tickit
ExecStart=/opt/tickit/bin/web
Restart=always
RestartSec=5
Environment=GO_ENV=production

# Security settings
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/opt/tickit/data /opt/tickit/upload
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

On the shared host, [`deploy/systemd/tickit.service.d/listen.conf`](../deploy/systemd/tickit.service.d/listen.conf) keeps the unit on loopback; `deploy/setup-host.sh` installs it ([Drop-ins on the shared host](#drop-ins-on-the-shared-host)).

## Step 5: Environment Configuration

Create `/opt/tickit/.env`:

```bash
GO_ENV=production
SESSION_SECRET=your-very-secure-session-secret-change-this
DATABASE_URL=sqlite:///opt/tickit/data/events.db
PORT=7105
HOST=127.0.0.1

# Add other production environment variables as needed
```

**Security Note:** Generate a strong session secret:
```bash
openssl rand -base64 32
```

## Step 6: Deployment Script

Create `deploy.sh` in your local project:

```bash
#!/bin/bash
set -e

# Configuration
SERVER="your-vps-ip-or-hostname"
APP_DIR="/opt/tickit"
BUILD_DIR="./bin"

echo "🔨 Building application..."
# Build for Linux if you're on a different platform
GOOS=linux GOARCH=amd64 go build -o $BUILD_DIR/web ./cmd/web

echo "🌱 Building seed script..."
# Build seed script for database seeding
GOOS=linux GOARCH=amd64 go build -o $BUILD_DIR/seed ./cmd/dev

echo "�️ Building migration script..."
# Build migration script for database migrations
GOOS=linux GOARCH=amd64 go build -o $BUILD_DIR/migrate ./cmd/migrate

echo "�📦 Building frontend assets..."
# Build frontend (adjust command based on your build system)
go run ./cmd/web/build

echo "📤 Uploading to server..."
rsync -avz --delete \
    --exclude='data/' \
    --exclude='node_modules/' \
    --exclude='.git/' \
    --exclude='*.log' \
    ./ $SERVER:$APP_DIR/

echo "🔐 Setting permissions..."
ssh $SERVER << 'EOF'
sudo chown -R tickit:tickit /opt/tickit
sudo chmod +x /opt/tickit/bin/web
sudo chmod +x /opt/tickit/bin/seed
sudo chmod +x /opt/tickit/bin/migrate
EOF

echo "🚀 Starting services..."
ssh $SERVER << 'EOF'
sudo systemctl daemon-reload
sudo systemctl restart tickit
sudo systemctl restart caddy
sudo systemctl enable tickit
sudo systemctl enable caddy

# Check status
sudo systemctl status tickit --no-pager
sudo systemctl status caddy --no-pager
EOF

echo "✅ Deployment complete!"
echo "🌐 Your app should be available at https://tickit.yourdomain.com"
```

Make the script executable:
```bash
chmod +x deploy.sh
```

## Step 7: Initial Deployment

1. **Update the deployment script** with your server details
2. **Build and deploy:**
   ```bash
   ./deploy.sh
   ```

## Step 8: SSL Certificate Setup

Caddy will automatically obtain and renew SSL certificates. On first deployment:

1. Ensure your domain points to your VPS
2. Start Caddy: `sudo systemctl start caddy`
3. Check logs: `sudo journalctl -u caddy -f`

## Monitoring and Maintenance

### Drop-ins on the shared host

The host keeps its own configs for Caddy, Prometheus and Alloy, and each loads a folder of files that the apps install: lith's guide [Metrics and monitoring](https://lith.zweiundeins.gmbh/docs/guides/monitoring#drop-in-files-for-a-shared-host) has the folders and the labels. tickit's are in `deploy/`, named after the site (`tickitdemo`), and the unit's after the unit (`tickit`):

- `tickitdemo.caddy` as `/etc/caddy/sites/tickitdemo.caddy`, by hand ([Step 3](#step-3-caddy-configuration)).
- `systemd/tickit.service.d/listen.conf` as `/etc/systemd/system/tickit.service.d/listen.conf`: the unit may reach loopback only.
- `observability/tickitdemo.targets.yml` as `/etc/prometheus/targets/tickitdemo.yml`: blackbox probes of `/`, a redirect (the host's `http_redirect` module), and `/auth/login` (`http_2xx_tickitdemo`, which wants "TickIt" in the body). Prometheus watches the folder.
- `observability/tickitdemo.alloy` as `/etc/alloy/tickitdemo.alloy`: Caddy's access log as `{job="caddy", site="tickitdemo"}`, through the host's pipeline for access logs.

tickit serves no metrics, so it has no `/etc/prometheus/apps/` target. `deploy/setup-host.sh` installs everything but the Caddy block: the unit's drop-in with a `daemon-reload` (it never restarts `tickit`: restart it to apply a changed one), then the probes and the Alloy file, each only where its service is installed. It reloads Alloy and puts the old file back if Alloy refuses the new one. Run it again after changing them:

```bash
scp -r deploy root@host:/tmp/tickit-deploy
ssh root@host bash /tmp/tickit-deploy/setup-host.sh
```

### Check Service Status

```bash
# Check application status
sudo systemctl status tickit

# Check Caddy status
sudo systemctl status caddy

# View logs
sudo journalctl -u tickit -f
sudo journalctl -u caddy -f
```

### Database Backups

Create a backup script `/opt/tickit/backup.sh`:

```bash
#!/bin/bash
BACKUP_DIR="/opt/tickit/backups"
DATE=$(date +%Y%m%d_%H%M%S)

mkdir -p $BACKUP_DIR

# Backup databases
cp /opt/tickit/data/*.db $BACKUP_DIR/backup_$DATE/
gzip $BACKUP_DIR/backup_$DATE/*.db

# Keep only last 30 backups
find $BACKUP_DIR -name "backup_*" -type d -mtime +30 -exec rm -rf {} \;
```

Add to crontab:
```bash
# Daily backup at 2 AM
0 2 * * * /opt/tickit/backup.sh
```

### Firewall Configuration

```bash
# Enable UFW
sudo ufw enable

# Allow SSH (adjust port if needed)
sudo ufw allow 22

# Allow HTTP and HTTPS
sudo ufw allow 80
sudo ufw allow 443

# Check status
sudo ufw status
```

## Troubleshooting

### Common Issues

1. **Port conflicts:** Ensure port 7105 is not used by other services
2. **Permissions:** Check that the `tickit` user can read/write to data directories
3. **SSL issues:** Verify domain DNS and firewall settings
4. **Service won't start:** Check logs with `journalctl -u tickit -n 50`

### Performance Tuning

For high-traffic sites, consider:

1. **Increase file limits** in the systemd service
2. **Enable HTTP/2 server push** in Caddy for critical resources
3. **Configure database connection pooling** in your Go app
4. **Set up monitoring** with Prometheus/Grafana

## Alternative: Docker Deployment

If you prefer Docker, create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  caddy:
    image: caddy:2-alpine
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"  # HTTP/3
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - ./web/ui:/opt/tickit/web/ui:ro
      - caddy_data:/data
      - caddy_config:/config
    restart: unless-stopped
    
  tickit:
    build: .
    environment:
      - GO_ENV=production
      - HTTP_PORT=8080
      - HOST=0.0.0.0
    volumes:
      - ./data:/app/data
      - ./upload:/app/upload
    restart: unless-stopped
    depends_on:
      - caddy

volumes:
  caddy_data:
  caddy_config:
```

## Features Supported

✅ **HTTP/3** - Automatic via Caddy  
✅ **Brotli Compression** - Built-in with gzip fallback  
✅ **Server-Sent Events** - Full support for long-running connections  
✅ **Automatic HTTPS** - Let's Encrypt integration  
✅ **Hot Reload** - Development workflow preserved  
✅ **Multi-app Hosting** - Easy to add more applications  
✅ **Static File Optimization** - Direct serving via Caddy  
✅ **Security Headers** - Production-ready security configuration  

This setup provides a robust, scalable, and secure deployment for your Tickit application with modern web standards support.