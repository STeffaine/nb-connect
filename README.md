# nb-connect

`nbcon` discovers NetBox IPAM Services and stores them in a local cache for a terminal-friendly infrastructure connection workflow.

## Current scope

`nbcon` synchronizes NetBox application services into a local cache, enriches them with parent target metadata, and launches configured SSH services. It uses the cache by default, so connection selection works offline after a successful sync.

## Setup

Create the public configuration at `~/.config/nb-connect/config.yaml` (or the platform equivalent) from [config.example.yaml](config.example.yaml).

Create `~/.config/nb-connect/credentials.yaml` from [credentials.example.yaml](credentials.example.yaml), replace the placeholder token, and apply restrictive permissions:

```sh
cp credentials.example.yaml ~/.config/nb-connect/credentials.yaml
chmod 600 ~/.config/nb-connect/credentials.yaml
go run ./cmd/nbcon sync
go run ./cmd/nbcon list
```

Use `--config`, `--credentials`, and `--cache` to override the default paths, which makes automation and testing straightforward.

## Connecting

The plain command opens a built-in terminal selector over cached services. No additional host package is required. Press `1` through `9` to connect to a numbered result, `f` to search, `s` to synchronize from NetBox, use arrows or `j`/`k` to move, Enter to connect, and Esc to cancel.

```sh
nbcon
nbcon connect
```

Use explicit selectors for scripts:

```sh
nbcon connect --target router-01 --service sshd
```

`ssh`, `sshd`, and `telnet` services are currently supported. SSH uses `ssh.default_user` and the service endpoint from NetBox, then executes the local `ssh` client. Local SSH configuration remains responsible for keys, host aliases, and additional options. Telnet services execute the local `telnet` client as `telnet <host> <port>` and do not use SSH configuration.

To assign a private key to a user, add it under `ssh.keys`:

```yaml
ssh:
	default_user: ansible
	keys:
		ansible:
			identity_file: ~/.ssh/id_ansible
```

`nbcon` expands `~/...` and passes the configured key to `ssh -i` for the matching user.

Use `--dry-run` to print the connection invocation without opening a session. Services with more than one endpoint require an explicit endpoint.

```sh
nbcon connect --target router-01 --service sshd --dry-run
nbcon connect --target router-01 --service sshd --endpoint 192.0.2.10:22
nbcon connect --target switch-01 --service telnet --dry-run
```

Use `--refresh` to synchronize with NetBox before reading the cache:

```sh
nbcon connect --refresh
```

### Ping checks

In the service selector, press `p` to ping the selected endpoint without leaving the TUI. Configure the number of probes with `ping.count`; it defaults to `4`:

```yaml
ping:
	count: 2
```

### API debugging

Use `sync --debug-api` to write each NetBox request URL, response status, and raw response body to standard error. Authorization headers and tokens are never printed.

```sh
go run ./cmd/nbcon sync --debug-api
```

Raw API responses may contain infrastructure names and addresses. Use this option only in a trusted terminal and do not paste its output into public channels.

## Design Decisions

- NetBox is queried through `GET /api/status/` for token validation and `GET /api/ipam/services/` for discovery. The client follows the paginated `results` response and supports modern unified service parents as well as older device/VM fields.
- The client follows NetBox pagination and preserves the service protocol, ports, IP addresses, device or VM, and tags.
- Service selection reads the local cache by default; `connect --refresh` explicitly updates it from NetBox.
- Cache writes are atomic and use restrictive `0600` permissions.
- Credentials are kept out of public configuration and are never written by the application in this milestone.

## Delivery Plan

1. Core: configuration, credentials, NetBox discovery, and cache. Complete.
2. Launcher: built-in terminal selector and SSH command construction. Complete.
3. Reliability: TTL-aware cache refresh and more actionable error handling.
4. Integration: tmux/WezTerm and SSH configuration generation.
5. Service expansion: HTTPS and further connector types, jump-host resolution, and Ansible export.