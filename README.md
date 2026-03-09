# ssh-agent-proxy

A single-binary SSH agent proxy that multiplexes multiple upstream SSH agents behind one Unix socket.

## Warning

This is a vibe-coded project, which is not reviewed by human currently. Use at your own risk!

## Features

- Aggregate keys from multiple SSH agents into a single socket
- Per-upstream key caching (`none`, `duration`, `forever`)
- Automatic cache invalidation when an unknown key is requested
- Manual cache invalidation via `SIGUSR1`
- Peer credential logging (PID, UID, GID, command line)
- Service management for macOS (launchd) and Linux (systemd user service)

## Build

```bash
go build -o ssh-agent-proxy .
```

## Configuration

Default config path: `~/.config/ssh-agent-proxy/config.json` (auto-created on first run).

```json
{
  "listen": "/tmp/ssh-agent-proxy.sock",
  "upstreams": [
    {"name": "system", "socket": "$SSH_AUTH_SOCK", "cache": "none"},
    {"name": "hw-key", "socket": "~/.gnupg/S.gpg-agent.ssh", "cache": "duration", "cache_duration": "5m"}
  ],
  "log": {"enabled": true, "file": "/tmp/ssh-agent-proxy.log", "level": "info"}
}
```

### Cache policies

| Policy     | Behavior                                                        |
|------------|-----------------------------------------------------------------|
| `none`     | Always queries the upstream agent                               |
| `duration` | Caches key list for `cache_duration` (Go duration string, e.g. `5m`) |
| `forever`  | Caches until explicitly invalidated                             |

## Usage

```
ssh-agent-proxy service <command>

Commands:
  service run [flags]    Run the proxy agent (foreground)
  service install        Install system service (launchd/systemd)
  service uninstall      Remove system service
  service start          Start the service
  service stop           Stop the service
```

### Flags for `service run`

| Flag       | Description                    |
|------------|--------------------------------|
| `-config`  | Config file path               |
| `-listen`  | Override listen socket path    |
| `-log`     | Override log file path         |
| `-debug`   | Enable debug-level logging     |
| `-version` | Print version and exit         |

### Running directly

```bash
ssh-agent-proxy service run -config ~/.config/ssh-agent-proxy/config.json
```

Then point clients at the proxy socket:

```bash
export SSH_AUTH_SOCK=/tmp/ssh-agent-proxy.sock
ssh-add -l
```

### Installing as a service

**macOS (launchd):**

```bash
ssh-agent-proxy service install
ssh-agent-proxy service install -config /path/to/config.json
```

**Linux (systemd user service):**

```bash
ssh-agent-proxy service install
ssh-agent-proxy service install -config /path/to/config.json
```

Manage the service:

```bash
ssh-agent-proxy service start
ssh-agent-proxy service stop
ssh-agent-proxy service uninstall
```

### Invalidating caches at runtime

```bash
pkill -USR1 ssh-agent-proxy
```

## License

MIT License. See [LICENSE](LICENSE) for details.
