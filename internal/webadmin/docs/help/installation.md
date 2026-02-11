# Installation

This guide covers installing and running coven-gateway for the first time.

## Prerequisites

- Go 1.25+ (for building from source)
- SQLite support (included, no external dependencies)
- A Tailscale account (recommended for secure access)

## Quick Start

### 1. Build the Gateway

```bash
git clone https://github.com/2389-research/coven-gateway.git
cd coven-gateway
make build
```

This produces binaries in `./bin/`:
- `coven-gateway` - Main server
- `coven-admin` - CLI monitoring tool
- `coven-tui` - Interactive terminal client

### 2. Initialize Configuration

```bash
./bin/coven-gateway init
```

This interactive wizard creates your config file at `~/.config/coven/gateway.yaml`.

Alternatively, copy and edit the example:

```bash
cp config.example.yaml ~/.config/coven/gateway.yaml
```

### 3. Bootstrap the Database

```bash
./bin/coven-gateway bootstrap --name "Your Name"
```

This:
- Creates the SQLite database
- Generates a secure JWT secret
- Creates your admin account
- Outputs a login token

### 4. Start the Gateway

```bash
./bin/coven-gateway serve
```

You'll see a startup banner with connection details:

```text
   ___  _____  _  _  ___  _  _
  / __)/  _  \| || || __|| \| |
 ( (__ | |_| || || || _| | \\ |
  \___)\_____|\_/\_/|___||_|\_|

 Config: /home/user/.config/coven/gateway.yaml
 gRPC:   0.0.0.0:50051
 HTTP:   0.0.0.0:8080
 Admin:  http://localhost:8080/admin/
```

### 5. Connect an Agent

In another terminal:

```bash
coven agent
```

The agent registers with the gateway and appears in your admin UI.

## Configuration File Locations

The gateway looks for config in this order:

1. `$COVEN_CONFIG` environment variable
2. `$XDG_CONFIG_HOME/coven/gateway.yaml`
3. `~/.config/coven/gateway.yaml`

## Data Storage

By default, data is stored in:

- **Config**: `~/.config/coven/gateway.yaml`
- **Database**: `~/.local/share/coven/gateway.db`
- **Tailscale state**: `~/.local/share/coven-gateway/tailscale/`

## Next Steps

- [Configuration](configuration) - Full config reference
- [Tailscale Setup](tailscale) - Secure remote access
- [Docker Deployment](docker) - Container deployment
