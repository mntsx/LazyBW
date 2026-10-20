# LazyBW — A TUI to manage the Bitwarden CLI

> \[!NOTE]
> LazyBW project and its contributors are not affiliated with, endorsed by, or sponsored by Bitwarden, Inc.

LazyBW is a terminal user interface (TUI) for **Linux**, built on top of the official [Bitwarden CLI (`bw`)](https://bitwarden.com/help/cli/). It runs in UTF-8, ANSI-compatible terminals (TTY/PTY, including SSH sessions), with true-color and mouse support recommended. LazyBW provides a convenient, keyboard-driven way to browse, search, create, edit, and organize vault items without dealing directly with CLI commands. It also introduces a feature not supported by `bw`: **the ability to create encrypted, read-only local replicas of your personal vault**, allowing offline access without connecting to Bitwarden's servers. All other functionality relies on the existing capabilities of the official CLI.

<!--
**Snap installation**:

```bash
sudo snap install lazybw
```
-->

<!--
**APT installation (Ubuntu 24.04, x86-64)**:

```bash
sudo add-apt-repository ppa:mntsx/lazybw
sudo apt update
sudo apt install lazybw
```
-->

**Installation**:

```bash
git clone git@github.com:mntsx/LazyBW.git
cd LazyBW
go build -o lazybw ./cmd/lazybw
sudo install -m 755 lazybw /usr/local/bin/lazybw
```

## Dependencies

Terminal dependencies:

| Requirement | Purpose |
| --- | --- |
| [`bw`](https://bitwarden.com/help/cli/) **(required)** | All Bitwarden authentication and remote vault operations; must be available on `PATH`. |
| A UTF-8 terminal **(required)** | A terminal with true-color and mouse support is recommended. |
| `wl-copy` (Wayland), `xclip` (X11), or `clip.exe` (WSL) **(optional)** | Copying values to the system clipboard. |
| `tput` **(optional)** | Improved Backspace / Ctrl+Backspace detection through terminfo. |
| Go **(build only)** | Compiling from source; not needed for a Snap installation. |

Go dependencies (automatically fetched through Go modules):

| Package                                                         | Purpose                                           |
| --------------------------------------------------------------- | ------------------------------------------------- |
| [`awesome-gocui/gocui`](https://github.com/awesome-gocui/gocui) | Terminal user interface framework                 |
| [`gdamore/tcell/v2`](https://github.com/gdamore/tcell)          | Terminal input and event handling                 |
| [`ericlagergren/siv`](https://github.com/ericlagergren/siv)     | AES-SIV authenticated encryption                  |
| [`golang.org/x/crypto`](https://pkg.go.dev/golang.org/x/crypto) | Additional cryptographic algorithms and utilities |

## Sign in to Bitwarden

LazyBW supports Bitwarden's **US and EU** cloud regions and API-key authentication, followed by unlocking your vault with your master password. You can enter credentials in the TUI or load your API key from a local file.

On first launch, LazyBW creates `settings.jsonc` and `keybinds.jsonc` in `~/.config/lazybw/` (or `LAZYBW_CONFIG_DIR`). Existing `.json` files are also supported. To prefill your API credentials and select a region, edit the `authentication` section of `settings.jsonc`:

```jsonc
"authentication": {
  "client_env_path": "$HOME/.config/lazybw/credentials.env",
  "default_server": "eu",
  "backend_mode": ["bw", "bw serve"]
}
```

- `client_env_path`: path to a credentials file, or `null` to enter credentials manually. `$HOME` and `~/` are supported.
- `default_server`: `"us"` (default) or `"eu"`.
- `backend_mode`: `"bw"`, `"bw serve"`, or an ordered list of both. The default tries `bw` before `bw serve`; `bw serve` requires Bitwarden CLI 2025.9.0 or later and uses a private Unix socket.

The credentials file accepts ordinary `.env` assignments, with or without `export`:

```dotenv
BW_CLIENTID=user.xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
BW_CLIENTSECRET=your-client-secret
```

Keep this file out of Git and restrict access to it (`chmod 600`). It contains your **API key**, not your master password.

![LazyBW authentication screens](assets/screenshots/authentication.png)

## Browse and manage your vault

Browse items and folders, inspect or edit fields, create and delete entries, search, and review **staged edits** before pushing them. LazyBW also supports synchronization and reviewing remote changes. These are interfaces to functionality already available through `bw`.

**The only additional vault capability** is an optional, encrypted, **read-only local replica of your personal vault** for offline access. Organization-owned items are not included.

## Keep an encrypted local replica

In `settings.jsonc`, configure `local_replication`:

```jsonc
"local_replication": {
  "save_vault": true,
  "vault_path": "$HOME/.local/share/lazybw/bw-vault",
  "vault_encryption": "aes-256-gcm"
}
```

- `save_vault`: `true` to enable replication; `false` (default) to disable it.
- `vault_path`: destination file; its parent directory must already exist (for the example, run `mkdir -p ~/.local/share/lazybw`). `$HOME` and `~/` are supported.
- `vault_encryption`: one of the algorithms below (default: `aes-256-gcm`).

| Encryption algorithm | Go implementation / source repository |
| --- | --- |
| `aes-256-gcm` | [Go standard library — `crypto/aes` and `crypto/cipher`](https://pkg.go.dev/crypto/cipher) |
| `aes-256-gcm-siv` | [`github.com/ericlagergren/siv`](https://github.com/ericlagergren/siv) |
| `xchacha20-poly1305` | [`golang.org/x/crypto/chacha20poly1305`](https://pkg.go.dev/golang.org/x/crypto/chacha20poly1305) |

All three use **Argon2id** ([`golang.org/x/crypto/argon2`](https://pkg.go.dev/golang.org/x/crypto/argon2)) to derive the encryption key from your master password. Replicas are updated from confirmed personal-vault data, exclude unsaved edits, and can be opened offline with the password used to encrypt them. **Local mode never writes to Bitwarden.**

You can update the replica without opening the TUI:

```bash
lazybw replicate
```

This uses `vault_path`, `client_env_path`, `default_server`, and `vault_encryption` from `settings.jsonc`. The command reports each authentication, synchronization, and encryption step, performs a fresh Bitwarden sync, and overwrites the replica atomically. It runs even when `save_vault` is `false`, because invoking the command explicitly requests one replication. Optional overrides are available:

```bash
lazybw replicate --path "$HOME/.local/share/lazybw/bw-vault" \
  --client "$HOME/.config/lazybw/credentials.env"
```

- `--path`: destination file. An explicitly supplied filename must not have an extension.
- `--client`: API credentials file. An explicitly supplied filename must have the `.env` extension.
- `--password`: master password. If omitted, LazyBW reads it from the controlling terminal with echo disabled, so it is not added to shell history. Supplying a password on the command line is less secure because the shell may save it in history and other local processes may briefly see the argument.

![LazyBW vault interface and local mode](assets/screenshots/vault.png)

## Synchronize and review changes

Local edits are staged for review before being pushed. When remote data has changed, **soft sync** keeps your staged edits while updating their remote baseline; **hard sync** discards the staged edits and adopts the remote values. You can also abort the operation.

![LazyBW editing and synchronization](assets/screenshots/synchronization.png)

## Customize connection checks

LazyBW checks the availability of the US and EU endpoints. To use different **HTTPS** URLs for those reachability checks, edit `network` in `settings.jsonc`:

```jsonc
"network": {
  "us_ping_url": "https://api.bitwarden.com",
  "eu_ping_url": "https://api.bitwarden.eu"
}
```

Both values must be HTTPS URLs that respond to a `HEAD` request. These settings change **connectivity checks only**, not the Bitwarden servers used for authentication or vault operations.

## In-memory secret protection

LazyBW encrypts sensitive values held in its working state using **AES-256-GCM** and a randomly generated, per-session key. It uses one of two key-storage backends:

- **Linux Kernel Keyring (preferred):** stores the session key in the kernel keyring when available.
- **Memory (fallback):** keeps the key in the process when the kernel keyring cannot be used, including on unsupported platforms.

The key is revoked or cleared when the session closes, where practical. This reduces the time secrets remain exposed in application state; it **does not guarantee** that plaintext never appears transiently in process memory or when communicating with `bw`.

For terminal input, `tcell` puts the TUI in raw mode, disabling terminal echo, canonical buffering, and signal-generating control characters. LazyBW does not log text-entry keystrokes in its key-feedback state. On Linux it also disables core dumps and ordinary same-user process inspection, and requests exclusive access to the TTY while the TUI or hidden password prompt is active. These are defense-in-depth measures: a privileged process, compromised terminal emulator, SSH host, multiplexer, or process that already owns the PTY can still observe input, which cannot be prevented from inside a terminal application.

## Development and maintenance

AI tools were used during the development of LazyBW. The project was created primarily for personal use, and **regular maintenance is not guaranteed**. It is published so others can use, study, fork, and modify it, subject to the repository's license.
