# Troubleshooting

Common issues and solutions for coven-gateway.

## Connection Issues

### "connection refused" when connecting agent

**Symptoms**: Agent can't connect to gateway

**Solutions**:
1. Verify gateway is running: `./bin/coven-gateway health`
2. Check the gRPC address in config matches what agent is using
3. If using Tailscale, ensure both gateway and agent are on the same tailnet
4. Check firewall isn't blocking the port

### Agent connects but immediately disconnects

**Symptoms**: Agent shows "connected" then "disconnected" in logs

**Causes**:
- Agent registration mode is `disabled` and agent isn't pre-registered
- Invalid agent SSH key fingerprint
- Heartbeat timeout too short

**Solutions**:
1. Check `auth.agent_auto_registration` setting
2. Verify agent's SSH key is registered (if using `disabled` mode)
3. Increase `agents.heartbeat_timeout` if network is slow

### WebAuthn/Passkey not working

**Symptoms**: "WebAuthn requires HTTPS" error

**Solution**: Enable Tailscale with HTTPS:

```yaml
tailscale:
  enabled: true
  https: true
```

WebAuthn requires a secure context (HTTPS).

## Database Issues

### "database is locked"

**Symptoms**: Operations fail with SQLite lock errors

**Causes**:
- Multiple gateway instances using same database
- Database on network filesystem (NFS, SMB)

**Solutions**:
1. Ensure only one gateway instance per database
2. Use local filesystem for database, not network mounts
3. If using Docker, ensure volume is mounted correctly

### "no such table"

**Symptoms**: Queries fail with missing table errors

**Solution**: Run bootstrap to initialize the database:

```bash
./bin/coven-gateway bootstrap --name "Admin"
```

## Tailscale Issues

### "auth key not valid"

**Causes**:
- Key expired
- One-off key already used
- Key revoked in admin console

**Solution**: Generate a new auth key at https://login.tailscale.com/admin/settings/keys

### "certificate not ready"

**Symptoms**: HTTPS fails shortly after startup

**Cause**: Tailscale needs time to provision certificates

**Solutions**:
1. Wait 30-60 seconds and retry
2. Ensure HTTPS certificates are enabled in Tailscale DNS settings
3. Check `tailscale cert` output for errors

### "funnel: not enabled"

**Cause**: Funnel not configured in tailnet ACL

**Solution**:
1. Go to https://login.tailscale.com/admin/acls
2. Add Funnel permissions:
```json
{
  "nodeAttrs": [
    {
      "target": ["tag:server"],
      "attr": ["funnel"]
    }
  ]
}
```

## Authentication Issues

### "invalid token"

**Causes**:
- Token expired
- JWT secret changed
- Token from different gateway

**Solutions**:
1. Re-authenticate via admin UI
2. Check `auth.jwt_secret` hasn't changed
3. Run `bootstrap` to generate new token

### "CSRF token invalid"

**Symptoms**: Form submissions fail

**Causes**:
- Session expired
- Browser blocked cookies
- Multiple tabs with stale tokens

**Solution**: Refresh the page and try again

## Startup Issues

### "address already in use"

**Cause**: Another process using the configured port

**Solutions**:
1. Find the process: `lsof -i :8080`
2. Change the port in config
3. Stop the conflicting process

### "permission denied" for database

**Causes**:
- Database directory doesn't exist
- Insufficient permissions
- SELinux/AppArmor blocking access

**Solutions**:
1. Create parent directory: `mkdir -p ~/.local/share/coven`
2. Check permissions: `ls -la ~/.local/share/coven`
3. Check SELinux: `audit2why < /var/log/audit/audit.log`

## Performance Issues

### Slow message responses

**Possible causes**:
1. Agent is slow (check agent logs)
2. Network latency (especially if agent is remote)
3. Large context/conversation history

**Diagnostics**:
- Check agent status in Settings > Agents
- Enable debug logging: `logging.level: "debug"`
- Monitor metrics endpoint: `/metrics`

### High memory usage

**Possible causes**:
1. Large number of active threads
2. Memory leak (report as bug)
3. Large message history

**Solutions**:
1. Archive old threads
2. Restart gateway (if memory leak suspected)
3. Check for updates

## Logging

### Enable debug logging

```yaml
logging:
  level: "debug"
  format: "text"
```

### View structured logs

```yaml
logging:
  level: "info"
  format: "json"
```

Then filter with `jq`:

```bash
./bin/coven-gateway serve 2>&1 | jq 'select(.level == "ERROR")'
```

## Getting Help

If you're still stuck:

1. Check logs with debug level enabled
2. Search [GitHub Issues](https://github.com/2389-research/coven-gateway/issues)
3. Open a new issue with:
   - Gateway version
   - Config (redact secrets!)
   - Error messages
   - Steps to reproduce
