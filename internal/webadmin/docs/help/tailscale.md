# Tailscale Setup

Tailscale provides secure, zero-config networking for your gateway. This is the recommended way to run coven-gateway in production.

## Why Tailscale?

- **Automatic TLS**: Certificates provisioned and renewed automatically
- **Secure by default**: Only your devices can access the gateway
- **No port forwarding**: Works behind NAT, firewalls, anywhere
- **WebAuthn support**: Passkey authentication requires HTTPS

## Quick Setup

### 1. Get an Auth Key

1. Go to [Tailscale Admin Console](https://login.tailscale.com/admin/settings/keys)
2. Generate an auth key (reusable recommended for servers)
3. Copy the key (starts with `tskey-auth-`)

### 2. Configure the Gateway

```yaml
tailscale:
  enabled: true
  hostname: "coven-gateway"
  auth_key: "${TS_AUTHKEY}"
  https: true

# Server addresses not needed with Tailscale
# server:
#   grpc_addr: ...
#   http_addr: ...
```

### 3. Set the Auth Key

```bash
export TS_AUTHKEY="tskey-auth-xxxxx"
./bin/coven-gateway serve
```

### 4. Access Your Gateway

Your gateway is now available at:
- **Admin UI**: `https://coven-gateway.your-tailnet.ts.net/admin/`
- **gRPC**: `coven-gateway.your-tailnet.ts.net:443`

## Configuration Options

### Basic Tailscale

```yaml
tailscale:
  enabled: true
  hostname: "coven-gateway"
  auth_key: "${TS_AUTHKEY}"
  https: true
```

Your gateway is accessible only to devices on your tailnet.

### Public Access with Funnel

Tailscale Funnel exposes your gateway to the public internet via Tailscale's infrastructure:

```yaml
tailscale:
  enabled: true
  hostname: "coven-gateway"
  auth_key: "${TS_AUTHKEY}"
  https: true
  funnel: true
```

With Funnel enabled:
- HTTP API is publicly accessible at `https://coven-gateway.your-tailnet.ts.net`
- gRPC remains tailnet-only (secure)
- No firewall changes needed

**Enable Funnel in Tailscale Admin:**

1. Go to [DNS settings](https://login.tailscale.com/admin/dns)
2. Enable HTTPS certificates
3. Go to [Access Controls](https://login.tailscale.com/admin/acls)
4. Add Funnel permissions for your node

### Ephemeral Nodes

For temporary or auto-scaling deployments:

```yaml
tailscale:
  enabled: true
  hostname: "coven-gateway"
  auth_key: "${TS_AUTHKEY}"
  ephemeral: true
```

The node is removed from your tailnet when the gateway shuts down.

### Custom State Directory

```yaml
tailscale:
  enabled: true
  hostname: "coven-gateway"
  auth_key: "${TS_AUTHKEY}"
  state_dir: "/var/lib/coven/tailscale"
```

Important for:
- Docker deployments (mount as volume)
- Multiple gateways on same machine

## Auth Key Types

| Type | Use Case |
|------|----------|
| **One-off** | Single device, expires after use |
| **Reusable** | Servers, containers, automation |
| **Ephemeral + Reusable** | Auto-scaling, temporary instances |

Generate keys at: https://login.tailscale.com/admin/settings/keys

## Docker with Tailscale

```yaml
# docker-compose.yml
services:
  coven-gateway:
    image: coven-gateway
    environment:
      - TS_AUTHKEY=${TS_AUTHKEY}
    volumes:
      - ./config.yaml:/app/config.yaml:ro
      - ./data:/app/data
      - ./tailscale:/app/tailscale
```

```yaml
# config.yaml
tailscale:
  enabled: true
  hostname: "coven-gateway"
  auth_key: "${TS_AUTHKEY}"
  state_dir: "/app/tailscale"
  https: true

database:
  path: "/app/data/gateway.db"
```

## Connecting Agents via Tailscale

Agents can connect using the Tailscale hostname:

```bash
coven agent --server coven-gateway.your-tailnet.ts.net:443
```

Or configure in the agent's config file.

## Troubleshooting

### "auth key not valid"

- Key may have expired or been used (if one-off)
- Generate a new reusable key

### "certificate not ready"

- Tailscale needs to provision certificates
- Wait 30-60 seconds and retry
- Ensure HTTPS certificates are enabled in Tailscale DNS settings

### "funnel not enabled"

- Funnel must be enabled in your tailnet's ACL policy
- See [Funnel documentation](https://tailscale.com/kb/1223/funnel)

### Connection refused

- Verify the gateway is running: `./bin/coven-gateway health`
- Check Tailscale status: `tailscale status`
- Ensure your device is on the same tailnet
