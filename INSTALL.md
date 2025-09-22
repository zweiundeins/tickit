# Tickit Deployment Guide

This guide covers deploying the Tickit application to a VPS using Caddy as a reverse proxy for optimal performance, HTTP/3 support, and multi-app hosting capability.

## Architecture Overview

```
Internet → Caddy (Port 80/443) → Go App (Port 8080)
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

Set your HTTP port to 8080 in your config:

```go
// Set HttpPort to "8080" and remove HttpsPort configuration
```

## Step 2: VPS Setup

### Install Dependencies

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Caddy
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update
sudo apt install caddy

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

Create `/etc/caddy/Caddyfile`:

```caddyfile
tickit.yourdomain.com {
    # Reverse proxy to Go application
    reverse_proxy 127.0.0.1:8080
    
    # Enable HTTP/3 and modern protocols
    protocols h1 h2 h3
    
    # Serve static files directly from Caddy for better performance
    handle /static/* {
        root * /opt/tickit/web/ui
        file_server
    }
    
    handle /uploads/* {
        root * /opt/tickit
        file_server
    }
    
    # Enable compression (Brotli preferred for SSE, with zstd and gzip fallbacks)
    # Brotli's compression window spans entire request - excellent for long-running SSE
    encode brotli zstd gzip
    
    # Security headers
    header {
        Strict-Transport-Security "max-age=63072000; includeSubDomains; preload"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "SAMEORIGIN"
        Referrer-Policy "strict-origin-when-cross-origin"
        X-XSS-Protection "1; mode=block"
    }
    
    # Access logs
    log {
        output file /var/log/caddy/tickit.log {
            roll_size 100mb
            roll_keep 5
            roll_keep_for 720h
        }
        format console
    }
}

# Example for additional apps on the same server:
# anotherapp.yourdomain.com {
#     reverse_proxy 127.0.0.1:8081
# }
```

### Create Log Directory

```bash
sudo mkdir -p /var/log/caddy
sudo chown caddy:caddy /var/log/caddy
```

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

## Step 5: Environment Configuration

Create `/opt/tickit/.env`:

```bash
GO_ENV=production
SESSION_SECRET=your-very-secure-session-secret-change-this
DATABASE_URL=sqlite:///opt/tickit/data/events.db
HTTP_PORT=8080
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

echo "📦 Building frontend assets..."
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

1. **Port conflicts:** Ensure port 8080 is not used by other services
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