# nb-connect

`nbcon` synchronizes NetBox application services into a local cache, enriches them with parent target metadata, and launches configured SSH and telnet services. It uses the cache by default, so connection selection works offline after a successful sync.

## Installation

### Manual Build

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

### NixOS Flake

Install `nbcon` system-wide on NixOS using the included flake:

**As a package:**
```sh
nix profile install github:STeffaine/nb-connect
```

**In a NixOS configuration (flake):**

Add to your system flake inputs:
```nix
inputs.nbcon.url = "github:STeffaine/nb-connect";
```

Enable the module in your host definition:
```nix
nixosConfigurations.my-host = nixpkgs.lib.nixosSystem {
	system = "x86_64-linux";
	modules = [
		inputs.nbcon.nixosModules.default
		({ ... }: {
			services.nbcon = {
				enable = true;
				settings = {
					netbox.servers = [
						{
							name = "production";
							url = "https://netbox.example.com";
						}
					];

					services.enabled = [ "sshd" "telnet" ];

					ssh = {
						default_user = "ops";
						keys.ops.identity_file = "/home/ops/.ssh/id_ops";
					};
				};

				# Recommended: keep tokens in a secret file managed by sops-nix/agenix.
				credentialsFile = "/run/secrets/nbcon-credentials.yaml";

				# Alternative (less secure): inline credentials become part of the Nix store.
				# credentials = {
				#   netbox.servers.production.token = "replace-with-real-token";
				# };

				# Optional: set a shared cache path instead of per-user XDG cache.
				# cachePath = "/var/cache/nbcon/services.json";
			};
		})
	];
};
```

When enabled, the module writes `/etc/nbcon/config.yaml` and exports `NBCON_CONFIG` and `NBCON_CREDENTIALS` so `nbcon` uses those files by default. If you use `credentialsFile`, that path is exported directly and no credentials file is written to `/etc`.

Then rebuild your system:
```sh
sudo nixos-rebuild switch --flake .#your-hostname
```

**For development:**
```sh
nix develop
go build -o nbcon ./cmd/nbcon
```


### Multiple NetBox servers

Configure each NetBox instance with a unique name and provide a matching token in the credentials file. `nbcon sync` validates and queries every configured server, then writes their combined services to the local cache. The `list` output includes the server name so duplicate targets are distinguishable. Run `nbcon sync <server>` (for example, `nbcon sync lab`) to synchronize only that server; cached services for other servers are left unchanged.

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

## Configuration Reference

This section documents every supported configuration option and how `nbcon` resolves file paths.

### Path resolution and precedence

`nbcon` resolves file paths in this order:

1. Explicit CLI flags: `--config`, `--credentials`, `--cache` (accepted by every command, including bare `nbcon`)
2. Environment variables: `NBCON_CONFIG`, `NBCON_CREDENTIALS`, `NBCON_CACHE` (blank values are ignored)
3. Built-in defaults:
	- Config: `${XDG_CONFIG_HOME:-~/.config}/nb-connect/config.yaml`
	- Credentials: `<directory of config path>/credentials.yaml`
	- Cache: `${XDG_CACHE_HOME:-~/.cache}/nb-connect/services.json`

### `config.yaml` options

| Key | Type | Required | Default | Details |
| --- | --- | --- | --- | --- |
| `netbox.servers` | list of objects | Yes | none | Must contain at least one NetBox server definition. |
| `netbox.servers[].name` | string | Yes | none | Server name used for filtering and credential lookup. Case-insensitive match at runtime. Leading/trailing whitespace is trimmed. Must be unique (case-insensitive). |
| `netbox.servers[].url` | string | Yes | none | NetBox base URL. Must start with `http://` or `https://`. Leading/trailing whitespace is trimmed and trailing `/` is removed. Do not include `/api/`. |
| `netbox.servers[].ssh` | object | No | unset | Per-server SSH configuration. When set, it fully replaces global `ssh` for that server (it does not merge with global `ssh`). |
| `services.enabled` | list of strings | No | empty list | NetBox service names to keep in cache and show in selector. Match is case-insensitive and trims surrounding whitespace. Empty list means no services are selected. |
| `ssh.default_user` | string | No | empty string | Default SSH username used for SSH-based services when no per-server `ssh` override applies. Used verbatim as the SSH login name. Practically required for SSH/SSHD connections: an empty user fails every SSH connection with a "no SSH user configured" error. |
| `ssh.keys` | map | No | empty map | Keyed by username. The entry whose key exactly equals the effective `default_user` (case-sensitive) supplies the identity file; other entries are ignored. |
| `ssh.keys.<user>.identity_file` | string | No | empty string | Path passed to `ssh -i` for that user. Only `~` and `~/...` are expanded to the home directory; `$HOME`, `~otheruser/`, and other forms are used as-is. |
| `cache.ttl` | duration string | No | `15m` | Parsed using Go duration syntax (for example `30s`, `15m`, `1h`). Currently parsed and validated, but not used to auto-expire cached data. |
| `ping.count` | integer | No | `4` | Number of ICMP echo requests for `ping -c` in the launcher network overlay. Unset or `0` selects `4`; negative values are rejected. |
| `launcher.info_panel_open_by_default` | boolean | No | `true` | Whether launcher info panel starts open when layout has room. |
| `filters` | list of objects | No | empty list | Saved filter presets for the selector. Created automatically when you save a preset with `s` in the filter menu. |

Per-server SSH block uses the same schema as global `ssh`. Because replacement is whole-block, a server with an `ssh` block that omits `default_user` has no SSH user even when the global block sets one.

| Key | Type | Required | Default | Details |
| --- | --- | --- | --- | --- |
| `netbox.servers[].ssh.default_user` | string | No | empty string | Default SSH user for that server only. Used verbatim as the SSH login name. |
| `netbox.servers[].ssh.keys` | map | No | empty map | Per-user SSH key map for that server only. Lookup is an exact, case-sensitive match on the effective user. |
| `netbox.servers[].ssh.keys.<user>.identity_file` | string | No | empty string | Identity file for that server/user pair. Only `~` and `~/...` are expanded; other forms are used as-is. |

Each `filters[]` entry is a named preset:

| Key | Type | Required | Default | Details |
| --- | --- | --- | --- | --- |
| `filters[].name` | string | Yes | none | Preset name. Trimmed, must be unique (case-insensitive). Saving a preset with an existing name replaces it. |
| `filters[].server` | string | No | empty | Single NetBox server for the preset. Shorthand for `servers` with one entry; merged with `servers` on load. |
| `filters[].servers` | string or list | No | empty | One or more NetBox server names. Accepts a single string or a list. |
| `filters[].match` | string | No | `all` | `all` requires every selected category to match; `any` requires at least one. Case-insensitive. |
| `filters[].tenants` | string or list | No | empty | Tenant values to select. Accepts a single string or a list. |
| `filters[].sites` | string or list | No | empty | Site values to select. |
| `filters[].roles` | string or list | No | empty | Role values to select. |

Filter values are trimmed and matched exactly (case-sensitive) in the selector, except server names which are matched case-insensitively against the cache when a preset is applied. Empty strings are dropped.

```yaml
filters:
  - name: prod vpn
    server: production
    match: all
    tenants: [team-a]
    sites: [site-a]
    roles: [vpn-server]
```

### `credentials.yaml` options

| Key | Type | Required | Default | Details |
| --- | --- | --- | --- | --- |
| `netbox.servers` | map | Yes | none | Must exist and include entries for each NetBox server you want to sync. |
| `netbox.servers.<name>.token` | string | Yes (per used server) | none | NetBox API token for that server. Server name matching is case-insensitive. Empty token is treated as missing. |

Notes:

1. For each configured NetBox server in `config.yaml`, `nbcon sync` requires a matching credentials entry.
2. Names are matched case-insensitively, so `Production` and `production` refer to the same server key.
3. Tokens are trimmed of surrounding whitespace; a token that is empty or only whitespace counts as missing.

### NixOS module options (`services.nbcon`)

When using `inputs.nbcon.nixosModules.default`, these options are available:

| Option | Type | Required | Default | Details |
| --- | --- | --- | --- | --- |
| `services.nbcon.enable` | bool | No | `false` | Enables module behavior and installs package. |
| `services.nbcon.package` | package | No | flake package `nbcon` | Package added to `environment.systemPackages`. |
| `services.nbcon.settings` | attrset (YAML) | No | `{}` | Rendered to `/etc/nbcon/config.yaml`. Should contain the same schema as `config.yaml`. |
| `services.nbcon.credentials` | attrset or `null` | Conditionally | `null` | Inline credentials rendered to `/etc/nbcon/credentials.yaml`. Not recommended for secrets because it is stored in the Nix store. |
| `services.nbcon.credentialsFile` | string or `null` | Conditionally | `null` | Path to external credentials file (recommended for secrets managers like sops-nix/agenix). |
| `services.nbcon.cachePath` | string or `null` | No | `null` | When set, exported as `NBCON_CACHE` for default cache location. |

Module assertions:

1. Set exactly one of `services.nbcon.credentials` or `services.nbcon.credentialsFile`.
2. If both are unset, evaluation fails.

Module exports:

1. `NBCON_CONFIG=/etc/nbcon/config.yaml`
2. `NBCON_CREDENTIALS=<credentialsFile or /etc/nbcon/credentials.yaml>`
3. `NBCON_CACHE=<cachePath>` only when `cachePath` is set

## User Guide

1. Copy the example configuration files as shown above. Set each NetBox server URL, add its matching API token, and choose the service names that `nbcon` should expose in `services.enabled`.
2. Run `nbcon sync` to validate the configured servers and save their enabled services locally. Repeat this whenever NetBox service data changes.
3. Run `nbcon` to open the cached service selector, or `nbcon <server>` (e.g., `nbcon server-a`) to open the selector pre-filtered to entries from that NetBox instance. Press `/` to search services, or `f` to filter by NetBox server, tenant, site, and role. In the filter menu, use `j`/`k` or arrow keys to choose a filter category, press Tab to move to its values, then use `j`/`k` or arrow keys to navigate and Space or Enter to select values. Press Tab again to return to filter categories. Press `/` to search values; while searching, use arrow keys to navigate results, Space to select more values, and Enter to select and return to the options. Press `a` to switch between matching all selected filter categories and any selected category. Press `d` to remove all filters. Press `s` to save the current filters as a named preset in `config.yaml`, or `p` to browse saved presets in a picker (`1`–`9` or `Enter` applies the preset and returns to the list; `r` deletes the highlighted preset after a `y`/`n` confirmation). Press `q` to return to the list with your filter selections kept. Select with Enter (or `1`–`9`), press `m` to star a favorite, `p` to ping and traceroute the selected endpoint, `i` for details, and Esc to exit without connecting. `q` never quits; Esc quits from anywhere. See Selector keys below for the full list.
4. For automation, use `nbcon connect --target <target> --service <service>` (the `connect` word may be omitted). Add `--server <name>` or pass the server positional argument when targets are duplicated across NetBox instances, and use `--dry-run` to inspect the local connection command first.

The selector and `list` command work from the local cache, so they remain usable without NetBox access after a successful sync. Only synchronization reads the credentials file. Use `nbcon connect --refresh` when a connection should synchronize before selecting a service.

## Connecting

The plain `nbcon` command is shorthand for `nbcon connect`: it accepts every `connect` flag, so `nbcon --target router-01 --service sshd --dry-run` works without the `connect` word. With none of `--target`, `--service`, or `--endpoint` given, it opens a built-in terminal selector over cached services. Running `nbcon <server>` (such as `nbcon production`) opens the same selector with that NetBox server pre-selected in the filters. Use `--preset <name>` (for example, `nbcon --preset "prod vpn"`) to open the selector with that saved `filters` preset applied; `--server` (or the positional argument) overrides the preset's server selection. No additional host package is required.

### Selector keys

| Key | Action |
| --- | --- |
| `j`/`k` or arrow keys | Move the cursor |
| `Enter` | Connect to the highlighted row |
| `1`–`9` | Connect to visible rows 1–9 |
| `/` | Search; type to filter, `Enter` keeps the text and returns to the list |
| `f` | Filter menu over NetBox server, tenant, site, and role |
| `s` | Synchronize all servers from NetBox |
| `m` | Toggle the highlighted row as a favorite (`*`) |
| `l` | Connect to the most recently used entry still present in the cache |
| `p` | Ping and traceroute overlay for the highlighted row (`q` or `p` closes it) |
| `i` | Toggle the service information panel |
| `q` | Home: close menus and overlays, clear search text, reset cursor (keeps chosen filter values; never quits) |
| `Esc` or `Ctrl+C` | Quit without connecting (exit status 0), from anywhere including menus and overlays |

Search matches case-insensitively against server, target, service name, endpoint, role, tenant, status, and description. A row matches when the query is a substring or a fuzzy in-order abbreviation of that combined text, and the list filters live as you type. (`q` returns home instead of typing the letter, even while searching or naming a preset; edit `config.yaml` directly for preset names containing lowercase `q`.)

In the filter menu, categories are focused first: use `j`/`k` or arrow keys to move within the focused pane and `Tab` to switch between categories and values (`left`/`right` also switch categories). Press `Space` or `Enter` to toggle a value, `/` to search values (arrows move, `Space` toggles more, `Enter` toggles and returns to the values), and `a` to switch between `all conditions` and `any condition`, and `d` to remove all filters. Press `s` to save the current filters as a preset (uppercase `S` also works): type a name and press `Enter` to write it to the `filters` section of `config.yaml` (creating that section when missing; an existing name is replaced). Press `p` (`P` also works) to browse saved presets in a picker: use `j`/`k` or arrow keys to move, `1`–`9` or `Enter` to apply the highlighted preset and return to the list, `r` (`R` also works) to delete the highlighted preset from `config.yaml` (`y` confirms, `n` cancels), or `p` to go back to the filter menu without applying. Values in the same category are alternatives. In `all conditions` mode a row must match every selected category, so selecting a category hides rows with no value for it; in `any condition` mode a match in any selected category is enough. Value matching is exact and case-sensitive. Press `q` to return to the list with your selections kept; `Esc` quits `nbcon` instead of closing the menu.

`nbcon <server>` pre-selects that server's filter value. The pre-selection lookup is case-insensitive: it uses the casing stored in the cache when that server already has cached services, and your typed text verbatim otherwise — matching is exact, so a differently-cased guess matches nothing.

Filter presets live in `config.yaml` under `filters:` (see Configuration Reference). Each preset has a unique `name`, an optional `server` (or `servers` list), an optional `match` mode (`all` or `any`), and optional `tenants`, `sites`, and `roles` lists. Manually editing `config.yaml` works the same as saving from the selector. In explicit (`--target`/`--service`) mode, `--preset` only contributes its single server value for disambiguation when `--server` is omitted; tenant/site/role values are ignored there.

Favorites (`m`) pin rows to a `Favorites` section at the top of the list. Every connection choice (via `Enter`, `1`–`9`, or `l`) is recorded in a most-recent-first `Recents` section holding the last 8 entries; `l` immediately connects to the newest one still present, and does nothing when there is none. Both are persisted in `${XDG_CACHE_HOME:-~/.cache}/nb-connect/launcher-state.json` (mode `0600`), which is independent of `--cache`/`NBCON_CACHE`. An unreadable or corrupt state file silently starts empty.

The selector needs at least one cached endpoint to open; otherwise it exits immediately with an error telling you to run `nbcon sync`. The `SERVER` column is shown only when the cache spans more than one NetBox server.

### Scripted connections

Use explicit selectors for scripts:

```sh
nbcon connect --target router-01 --service sshd
nbcon connect --server production --target router-01 --service sshd
```

Passing any of `--target`, `--service`, or `--endpoint` selects explicit mode, in which `--target` and `--service` must be used together. Both are matched case-insensitively against the cached entries: `--target` against the device name, or the virtual-machine name when the service has no device, and `--service` against the NetBox service name. `--server` takes precedence over the positional `[server]` argument; both filter case-insensitively before matching. When neither is given, `--preset` supplies its single server value for disambiguation when the named preset has exactly one server. When several cached entries match, the command fails and asks you to disambiguate with `--server` (or the positional argument).

`--endpoint` takes a `host:port` value that is used verbatim without checking it against the service's own endpoints. When it is omitted, the matched service must have exactly one endpoint; services with zero or with more than one endpoint fail with an error naming the count and asking for `--endpoint`.

### How connection commands are built

Only `ssh`, `sshd`, and `telnet` services can be connected (names match case-insensitively, ignoring surrounding whitespace), and the NetBox service protocol must be TCP (case-insensitive). Other service names may be listed in `services.enabled` and will appear in the cache and selector, but connecting to them fails with an "unsupported service" error.

Endpoints are derived from each NetBox service as every IP address combined with every port. A CIDR suffix is stripped first, so `192.0.2.10/24` becomes host `192.0.2.10`. In the selector, every endpoint becomes its own row, so one service can appear several times. In explicit mode, an omitted `--endpoint` selects the service's single endpoint automatically, as described above.

SSH connections run the local `ssh` client as:

```sh
ssh [-i <identity_file>] <user>@<host> -p <port>
```

`<user>` is the effective `ssh.default_user`: the per-server `ssh` block when the service's server defines one, otherwise the global `ssh` block. An empty user fails with a "no SSH user configured" error. `<identity_file>` comes from the `ssh.keys` entry whose key exactly equals that user (case-sensitive); it is omitted when empty, and `~`/`~/...` in it is expanded to the home directory at config load. No other SSH options are added: host aliases and extra options remain the job of the local SSH configuration. IPv6 hosts are passed without brackets, for example `ops@2001:db8::10`.

Telnet connections run the local `telnet` client as:

```sh
telnet <host> <port>
```

They ignore all SSH configuration.

Use `--dry-run` to print the connection invocation to standard output without opening a session. Each argument is double-quoted in the printed form.

```sh
nbcon connect --target router-01 --service sshd --dry-run
# ssh "-i" "/home/ops/.ssh/id_ops" "ops@192.0.2.10" "-p" "22"
nbcon connect --target router-01 --service sshd --endpoint 192.0.2.10:22
nbcon connect --target switch-01 --service telnet --dry-run
# telnet "192.0.2.11" "23"
```

Without `--dry-run`, `nbcon` executes the `ssh` or `telnet` binary found on `PATH` with the terminal attached as standard input, output, and error, so a missing binary fails at that point.

To assign a private key to a user, add it under `ssh.keys`:

```yaml
ssh:
	default_user: ansible
	keys:
		ansible:
			identity_file: ~/.ssh/id_ansible
```

`nbcon` expands `~/...` and passes the configured key to `ssh -i` for the matching user.

To use different SSH credentials for one NetBox server, add an `ssh` block to that server. A server with its own `ssh` block uses only that block; all other servers fall back to the global `ssh` settings.

```yaml
netbox:
	servers:
		- name: production
			url: https://netbox.example.com
			ssh:
				default_user: prod-admin
				keys:
					prod-admin:
						identity_file: ~/.ssh/id_prod_admin
		- name: lab
			url: https://netbox-lab.example.com
			# No ssh block: uses the global ssh settings below.

ssh:
	default_user: admin
	keys:
		admin:
			identity_file: ~/.ssh/id_admin
```

Use `--refresh` to synchronize with NetBox before reading the cache. `--refresh` always synchronizes every configured server, ignoring `--server` and the positional argument, and aborts the connection when the sync fails:

```sh
nbcon connect --refresh
```

Use `--config`, `--credentials`, and `--cache` to override the default paths. You can also set `NBCON_CONFIG`, `NBCON_CREDENTIALS`, and `NBCON_CACHE` to provide defaults from your shell or system profile. Explicit flags always take precedence over environment variables.

### Synchronization and the cache

`nbcon sync` validates every configured server with `GET /api/status/`, then downloads all application services from `GET /api/ipam/services/?limit=100`, following `next` pages until exhausted. Each unique parent device or virtual machine is fetched once and reused for its sibling services, contributing the cached `site`, `role`, `tenant`, `platform`, `status`, and `description` fields. Tokens starting with `nbt_` are sent as `Authorization: Bearer <token>`; all other tokens use `Authorization: Token <token>`. Downloaded services are filtered by `services.enabled` (case-insensitive, surrounding whitespace ignored); an empty list caches nothing. A successful sync prints `Connected to NetBox`, `Found N services`, and `Cache updated`.

When some servers fail — including a missing credentials entry — `sync` still writes the services fetched from the servers that succeeded, prints `Connected to NetBox (partial)` with the same `Found N services` / `Cache updated` lines, warns on standard error, and exits non-zero. When every server fails, the cache is left untouched and the combined error is returned.

`nbcon sync <server>` (matched case-insensitively) synchronizes only that server and fails if the name matches no configured server. Any failure aborts without touching the cache; on success, the fresh services for that server are merged with the preserved cached services of the other servers. The `Found N services` count then covers only the synchronized server. If the cache cannot be read, the merged result contains only the synchronized server.

The cache is JSON (`synced_at` plus `services`) written atomically through a temporary file and rename; its directory is created with mode `0700` and the file with mode `0600`. `cache.ttl` is parsed and validated but never expires entries: the cache changes only through `sync`, `connect --refresh`, or the selector's `s` key, all of which synchronize every configured server. The selector's `s` key also clears the search text, resets the cursor, and reports `Synced N services` or the failure inline without leaving the selector.

The selector, `list`, and explicit `connect` work from the local cache, so they remain usable without NetBox access after a successful sync. Only synchronization reads the credentials file: `sync`, `connect --refresh`, and the selector's `s` key. `connect` and the selector additionally require a valid config file, while `list` needs only a readable cache and prints the `SERVER TARGET SERVICE ENDPOINTS ROLE TENANT STATUS` table (endpoints comma-separated, always with its header row) without contacting NetBox.

### Ping checks

In the service selector, press `p` to open a network overlay that runs `ping -c <ping.count> <host>` and `traceroute <host>` concurrently against the highlighted endpoint's host (the port is dropped) and shows both outputs side by side. Close the overlay with `q` or `p`, which stops both commands; `Esc` quits `nbcon` instead. Both commands come from your `PATH`, and failures are shown inline in the overlay. Configure the number of probes with `ping.count`:

```yaml
ping:
	count: 2
```

Unset or `0` selects the default of `4`; negative values are rejected.

### Info panel

Control whether the service information panel opens automatically when there is enough screen space with `launcher.info_panel_open_by_default`. Set it to `false` if you want the selector to start closed and only open the panel when you press `i`:

```yaml
launcher:
	info_panel_open_by_default: false
```

In the selector, `i` toggles the panel: a docked side panel when the terminal fits the full list plus a panel at least 28 columns wide, otherwise a fullscreen overlay where `Enter` still connects and `q` or `i` closes it. The panel lists server, target, device, VM, service, protocol, ports, IPs, site, role, tenant, platform, tags, status, description, the selected endpoint, and favorite state. Your first manual toggle wins over the configured default for later resizes.

### API debugging

Use `sync --debug-api` to write each NetBox request URL, response status, and raw response body to standard error. Authorization headers and tokens are never printed.

```sh
nbcon sync --debug-api
nbcon connect --refresh --debug-api
```

The `connect` variant traces only the pre-connection sync; without `--refresh`, `connect --debug-api` has nothing to trace.

Raw API responses may contain infrastructure names and addresses. Use this option only in a trusted terminal and do not paste its output into public channels.
