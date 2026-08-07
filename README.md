# nb-connect

`nbcon` discovers NetBox IPAM Services and stores them in a local cache for a terminal-friendly infrastructure connection workflow.

## Current scope

`nbcon` synchronizes NetBox application services into a local cache, enriches them with parent target metadata, and launches configured SSH and telnet services. It uses the cache by default, so connection selection works offline after a successful sync.

## Installation

Clone, build, and install `nbcon`, then copy the configuration templates:

```sh
git clone https://github.com/STeffaine/nb-connect.git
cd nb-connect
go build -o nbcon ./cmd/nbcon
sudo install -m 0755 nbcon /usr/local/bin/nbcon
mkdir -p ~/.config/nb-connect
install -m 0644 config.example.yaml ~/.config/nb-connect/config.yaml
install -m 0600 credentials.example.yaml ~/.config/nb-connect/credentials.yaml
```
To install in `/usr/bin` instead, replace the destination in the `install` command with `/usr/bin/nbcon`. On a system where you do not have administrator access, install it in your local bin directory:

Edit `~/.config/nb-connect/config.yaml` and `~/.config/nb-connect/credentials.yaml` before running `nbcon sync`.

```sh
mkdir -p ~/.local/bin
install -m 0755 nbcon ~/.local/bin/nbcon
export PATH="$HOME/.local/bin:$PATH"
```

Add the `PATH` export to your shell profile when `~/.local/bin` is not already on `PATH`.

### Multiple NetBox servers

Configure each NetBox instance with a unique name and provide a matching token in the credentials file. `nbcon sync` validates and queries every configured server, then writes their combined services to the local cache. The `list` output includes the server name so duplicate targets are distinguishable.

```yaml
# config.yaml
netbox:
	servers:
		- name: production
			url: https://netbox.example.com
		- name: lab
			url: https://netbox-lab.example.com
```

```yaml
# credentials.yaml
netbox:
	servers:
		production:
			token: production-token
		lab:
			token: lab-token
```

The `netbox.servers` mappings are required, including when connecting to only one NetBox instance.

## User Guide

1. Copy the example configuration files as shown above. Set each NetBox server URL, add its matching API token, and choose the service names that `nbcon` should expose in `services.enabled`.
2. Run `nbcon sync` to validate the configured servers and save their enabled services locally. Repeat this whenever NetBox service data changes.
3. Run `nbcon` to open the cached service selector. Search with `f`, move with arrow keys or `j`/`k`, select with Enter, and press `p` to ping the selected endpoint. Press Esc to exit without connecting.
4. For automation, use `nbcon connect --target <target> --service <service>`. Add `--server <name>` when targets are duplicated across NetBox instances, and use `--dry-run` to inspect the local connection command first.

The selector and `list` command work from the local cache, so they remain usable without NetBox access after a successful sync. Use `nbcon connect --refresh` when a connection should synchronize before selecting a service.

## Connecting

The plain `nbcon` command opens a built-in terminal selector over cached services. No additional host package is required. Press `1` through `9` to connect to a numbered result, `f` to search, `s` to synchronize from NetBox, use arrows or `j`/`k` to move, Enter to connect, and Esc to cancel.

Use explicit selectors for scripts:

```sh
nbcon connect --target router-01 --service sshd
nbcon connect --server production --target router-01 --service sshd
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

Use `--config`, `--credentials`, and `--cache` to override the default paths, which makes automation and testing straightforward.

### Ping checks

In the service selector, press `p` to ping the selected endpoint without leaving the TUI. Configure the number of probes with `ping.count`; it defaults to `4`:

```yaml
ping:
	count: 2
```

### API debugging

Use `sync --debug-api` to write each NetBox request URL, response status, and raw response body to standard error. Authorization headers and tokens are never printed.

```sh
nbcon sync --debug-api
```

Raw API responses may contain infrastructure names and addresses. Use this option only in a trusted terminal and do not paste its output into public channels.
