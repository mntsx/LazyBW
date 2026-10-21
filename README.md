# LazyBW — A TUI to manage the Bitwarden CLI

> [!NOTE]
> LazyBW project and its contributors are not affiliated with, endorsed by, or sponsored by Bitwarden, Inc.

LazyBW is a keyboard-driven terminal user interface (TUI) for **Linux**, built on top of the official [Bitwarden CLI (`bw`)](https://bitwarden.com/help/cli/). It lets you browse, search, create, edit, and organize Bitwarden vault items without working directly with CLI commands.

In addition to the capabilities exposed by `bw`, LazyBW can create encrypted, read-only local replicas of your **personal** vault for offline access. All remote vault operations continue to be performed through the official Bitwarden CLI.

## Requirements

| Requirement | Purpose |
| --- | --- |
| [`bw`](https://bitwarden.com/help/cli/) **(required)** | Bitwarden authentication and remote vault operations; it must be available on `PATH`. |
| UTF-8 ANSI terminal **(required)** | LazyBW runs in TTY/PTTY terminals, including SSH sessions. True color and mouse support are recommended. |
| Monospaced icon font **(recommended)** | A font with Material Design Icons or Nerd Font glyphs keeps icons aligned. |
| `wl-copy` (Wayland), `xclip` (X11), or `clip.exe` (WSL) **(optional)** | System clipboard integration. |
| `tput` **(optional)** | Better distinction between Backspace and Ctrl+Backspace in compatible terminals. |
| Git and Go 1.26.0+ **(build only)** | Required to build LazyBW from source. |

## Features

- API-key authentication for Bitwarden US and EU cloud regions.
- Vault browsing, filtering, search, item and folder management, field editing, clipboard support, and customizable keybindings.
- In-memory staging: review, push, or discard changes before they reach Bitwarden.
- Remote-change detection with soft and hard synchronization choices.
- Two Bitwarden CLI backends: traditional `bw` commands and the private-socket `bw serve` API.
- Encrypted, authenticated, read-only local replicas for offline access.
- Configurable theme, icons, connection checks, authentication cache, backend order, and replica encryption.
- Defense-in-depth measures for sensitive terminal input and in-memory application secrets.

## Screenshots

![LazyBW authentication screens](assets/screenshots/authentication.png)

![LazyBW vault interface and local mode](assets/screenshots/vault.png)

![LazyBW editing and synchronization](assets/screenshots/synchronization.png)

## Complete documentation

Setup, configuration, keybindings, staging and synchronization, replicas, security architecture, and CLI usage are documented in **[the full LazyBW documentation](assets/DOCUMENTATION.md)**.

## Development and maintenance

AI tools were used during the development of LazyBW. The project was created primarily for personal use, and **regular maintenance is not guaranteed**. It is published so others can use, study, fork, and modify it, subject to the repository's license.
