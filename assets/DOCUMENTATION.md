# LazyBW documentation

LazyBW is a terminal interface for the official Bitwarden CLI. This manual is the complete reference for installing, configuring, operating, and securing it.

## Table of contents

- [Introduction](#introduction)
  - [Screens and workflows](#screens-and-workflows)
- [Requirements and build](#requirements-and-build)
- [Configuration and files](#configuration-and-files)
- [Configuration: `settings.jsonc`](#configuration-settingsjsonc)
  - [Default file](#default-settingsjsonc-file)
  - [Operational settings](#operational-settings)
  - [Visual settings](#visual-settings)
- [Configuration: `keybinds.jsonc`](#configuration-keybindsjsonc)
  - [Default file](#default-keybindsjsonc-file)
  - [Keybinding conventions](#keybinding-conventions)
  - [Keybinding areas](#keybinding-areas)
- [Staging, push, and synchronization](#staging-push-and-synchronization)
- [`bw` and `bw serve` backends](#bw-and-bw-serve-backends)
- [Encrypted local replicas](#encrypted-local-replicas)
- [Encryption and security](#encryption-and-security)
- [CLI reference](#cli-reference)

## Introduction

LazyBW is a keyboard-driven Linux TUI that uses the official [Bitwarden CLI (`bw`)](https://bitwarden.com/help/cli/) for all Bitwarden authentication and remote-vault operations. It supports the US and EU Bitwarden cloud regions, API-key sign-in, and unlocking with the master password.

Its normal workspace provides an interface for browsing, filtering, searching, creating, editing, and organizing vault items and folders. Local changes are held in an in-memory staging area so they can be reviewed before being sent to Bitwarden. LazyBW also detects remote changes and offers reconciliation choices before a write occurs.

The only vault capability added beyond the upstream CLI is an optional encrypted, read-only local replica of the **personal vault**. It is intended for offline browsing; organization-owned items are excluded and local mode never writes to Bitwarden.

### Screens and workflows

1. **Initial check (`AuthChecking`)** — checks connectivity, the selected CLI backend, and the status of the isolated Bitwarden profile.
2. **API credentials (`AuthClient`)** — enter a Bitwarden API client ID and secret, choose the US or EU region, and choose a backend. Credentials can instead be loaded from an `.env` file.
3. **Master-password unlock (`AuthMaster`)** — unlock the authenticated Bitwarden profile with the master password. The same screen is used to unlock an available local replica when working offline.
4. **Offline state (`AuthOffline`)** — shown when Bitwarden cannot be reached and no usable local replica is available.
5. **Workspace (`Workspace`)** — the online interface has panels for vault filters, item types, folders, staged edits, items, and the selected item's content. You can open items, edit supported fields, create or delete items and folders, search, copy values, reveal hidden fields, and review staged changes.
6. **Local workspace** — after opening a replica, the sidebar is reduced to item types and folders. Navigation, search, copying, and revealing values remain available; editing, staging, push, and discard operations are disabled.
7. **Dialogs and editors** — LazyBW presents focused dialogs for server/backend selection, creating item and field types, choosing folders or linked fields, entering names and URLs, confirmations, master-password re-prompts, and remote-change review. The detail editor supports selection, clipboard operations, undo/redo, line navigation, wrapping, and hidden-value reveal.

![Authentication, API-key, and unlock screens](screenshots/authentication.png)

![Online vault workspace and read-only local workspace](screenshots/vault.png)

![Field editing, staged changes, and remote synchronization review](screenshots/synchronization.png)

## Requirements and build

### Runtime requirements

| Requirement | Required? | Integration and purpose |
| --- | --- | --- |
| Linux | Target platform | LazyBW is developed for Linux. Its terminal/process hardening uses Linux features when available. Non-Linux builds fall back for some in-memory protection, but are not the supported target. |
| [`bw`](https://bitwarden.com/help/cli/) on `PATH` | Yes for remote use and replication | LazyBW starts `bw` to log in, unlock, synchronize, list, create, edit, and delete Bitwarden data. The standalone `bw` backend works with the CLI; `bw serve` needs Bitwarden CLI 2025.9.0 or newer. |
| UTF-8 ANSI TTY/PTTY | Yes for the TUI | The interface works in local and SSH terminal sessions. A terminal at least 65×19 cells is needed for the workspace. True color and mouse support are recommended. |
| HTTPS access to Bitwarden | Yes for online use | LazyBW checks the configured US/EU reachability endpoints before remote operations. An already-created local replica can be opened offline. |
| Monospaced Material Design Icons or Nerd Font | Recommended | Renders configured icon glyphs at their intended width. Disable `visual.icons.enabled` when no compatible font is available. |
| Clipboard helper | Optional | `wl-copy`/`wl-paste` on Wayland, `xclip` on X11, or `clip.exe`/`cmd.exe`/PowerShell in WSL/Windows enable clipboard operations. Without one, the rest of the application works. |
| `tput` | Optional | `tput kbs` lets LazyBW distinguish Backspace and Ctrl+Backspace more reliably in compatible terminals. |

### Building from source

Git and Go **1.26.0 or newer** are required to build. Dependencies are Go modules and are also vendored in this repository.

```bash
git clone https://github.com/mntsx/LazyBW.git
cd LazyBW
go build -o lazybw ./cmd/lazybw
sudo install -m 755 lazybw /usr/local/bin/lazybw
```

Use `git@github.com:mntsx/LazyBW.git` instead when you prefer SSH and have a GitHub SSH key configured.

### Go modules

| Module | Version | Why LazyBW uses it |
| --- | ---: | --- |
| `github.com/awesome-gocui/gocui` | `v1.1.0` | TUI layout, views, and rendering. |
| `github.com/gdamore/tcell/v2` | `v2.4.0` | Terminal input, raw-mode handling, terminfo, and key events. |
| `github.com/ericlagergren/siv` | `v0.0.0-20220507050439-0b757b3aa5f1` | AES-256-GCM-SIV encryption for local replicas. |
| `golang.org/x/crypto` | `v0.58.0` | Argon2id and XChaCha20-Poly1305 for replicas. |

The implementation also uses Go's `crypto/aes` and `crypto/cipher` for AES-256-GCM, `golang.org/x/sys` for Linux/TTY/keyring integration, and `golang.org/x/term` for hidden password input. Indirect UI dependencies handle encoding, color, terminal character widths, and Unicode segmentation.

## Configuration and files

### Configuration directory and compatibility

On first use, LazyBW creates its configuration directory with mode `0700`:

- default: `~/.config/lazybw/`
- override: the value of `LAZYBW_CONFIG_DIR`

It creates `settings.jsonc`, `keybinds.jsonc`, and a local `.gitignore` containing `bitwarden/`. JSONC permits comments and trailing commas. Legacy `settings.json` and `keybinds.json` remain supported; when both forms exist, `.jsonc` takes precedence.

Existing configuration files are not overwritten during normal loading. Missing default fields are added while existing values and comments are preserved. Use `lazybw reconfig` to intentionally restore generated defaults.

### Files created on the user's system

| Location | Permissions / lifetime | Content and purpose |
| --- | --- | --- |
| `<config>/settings.jsonc` | User configuration | Authentication preferences, replica settings, reachability URLs, icons, and colors. |
| `<config>/keybinds.jsonc` | User configuration | Key mappings per screen, panel, editor, and dialog. |
| `<config>/.gitignore` | Created once | Ignores `bitwarden/`, preventing session/cache data from being committed if the config directory is versioned. |
| `<config>/bitwarden/sessions/<id>/<region>/` | Directory `0700`; temporary | Isolated Bitwarden CLI application profile for each LazyBW execution. It is protected with an advisory lock and removed when the session closes. Abandoned profiles older than 24 hours are cleaned up only after their lock can be acquired. |
| `<config>/bitwarden/cached-logins/<region>/` | Directories `0700`, files `0600`; persistent when enabled | Cached **locked** API-key login state by region. It avoids repeating API-key login but does not restore an unlocked LazyBW session. Metadata includes a SHA-256 consistency check. |
| `<config>/bitwarden/cached-logins/.disabled` | Persistent marker | Created when `cache_client_login` is disabled; the cached login directories are removed. |
| `$XDG_CACHE_HOME/lazybw/` or `~/.cache/lazybw/` | Directory `0700`, file `0600` | Cache of `bw` binary capabilities, such as version and `bw serve`/Unix-socket support. It contains no vault secrets. |
| `local_replication.vault_path` | Replica file `0600` | Encrypted local vault snapshot. Its parent directory must already exist. The default is `$HOME/.local/share/lazybw/bw-vault`. |
| `$XDG_RUNTIME_DIR/lazybw-*` or private `/tmp` directory | Directory `0700`; session lifetime | Private Unix socket and process state for a LazyBW-managed `bw serve` process. The socket is restricted to mode `0600`. |

`<config>` in the table means the configured LazyBW configuration directory. The credentials file referenced by `client_env_path` is **not** generated automatically: create and protect it yourself, for example with `chmod 600`.

```dotenv
BW_CLIENTID=user.xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
BW_CLIENTSECRET=your-client-secret
```

The file accepts ordinary `.env` assignments with or without `export`. It stores an API key, not the master password. Keep it outside Git and restrict its read permissions.

## Configuration: `settings.jsonc`

### Default `settings.jsonc` file

This is the complete file generated by `lazybw reconfig --settings` (and on first run in an empty configuration directory):

```jsonc
// Visual colors: transparent, ansi_<color>, ansi_bright_<color>, ansi256:0..255, #RRGGBB.
// Available colors: black, red, green, yellow, blue, magenta, cyan, white.
{
  "authentication": {
    "client_env_path": null,
    "cache_client_login": true,
    // Allowed values: us, eu
    "default_server": "us",
    // Allowed values: bw, bw serve
    "backend_mode": [
      "bw serve",
      "bw"
    ]
  },
  "local_replication": {
    "save_vault": false,
    // Allowed values: aes-256-gcm, aes-256-gcm-siv, xchacha20-poly1305
    "vault_encryption": "aes-256-gcm",
    "vault_path": "$HOME/.local/share/lazybw/bw-vault"
  },
  "network": {
    "eu_ping_url": "https://api.bitwarden.eu",
    "us_ping_url": "https://api.bitwarden.com"
  },
  "visual": {
    // Icons: set enabled to false to use text-only markers.
    "icons": {
      "enabled": true,
      // Authentication
      "authentication": {
        "backend": "\U0000F013",
        "encryption": "\U000F0780",
        "local_vault": "\U000F01DA",
        "server": "\U000F0379"
      },
      // Layout
      "layout": {
        "connection": "\U000F0780",
        "edits": "\U0000F440",
        "folders": "\U000F10E5",
        "items": "\U000F10E5",
        "vault": "\U000F10E5"
      },
      // Items
      "items": {
        "all": "\U0000EB94",
        "card": "\U0000F09D",
        "favourite": "\U0000F005",
        "folder": "\U000F0256",
        "identity": "\U0000F2BB",
        "login": "\U000F059F",
        "no_folder": "\U000F19F9",
        "secure_note": "\U0000F24A",
        "ssh_key": "\U000F0306",
        "user": "\U0000F007",
        "vault": "\U0000EDCA"
      },
      // Contents
      "contents": {
        "checkbox": "\U000F0135",
        "edit": "\U0000F044",
        "favourite": "\U0000F005",
        "hidden": "\U000F07F5",
        "last_modified": "\U0000F0CE",
        "linked": "\U000F0339",
        "text": "\U000F0284",
        "web": "\U000F059F"
      },
      // Dialogs
      "dialogs": {
        "cancel": "\U000F073A",
        "linked_field": "\U000F0339"
      }
    },
    // Colors
    "colors": {
      "accent": "ansi_blue",
      "auth_tip_background": "ansi_black",
      "auth_tip_text": "ansi_white",
      "background": "transparent",
      "black": "ansi_black",
      "blue": "ansi_blue",
      "border": "ansi_blue",
      "border_focus": "ansi_bright_blue",
      "bright_black": "ansi_bright_black",
      "bright_blue": "ansi_bright_blue",
      "bright_cyan": "ansi_bright_cyan",
      "bright_green": "ansi_bright_green",
      "bright_magenta": "ansi_bright_magenta",
      "bright_red": "ansi_bright_red",
      "bright_white": "ansi_bright_white",
      "bright_yellow": "ansi_bright_yellow",
      "button": "ansi_bright_black 8%",
      "button_hover": "ansi_bright_black 28%",
      "cyan": "ansi_cyan",
      "error": "ansi_bright_red",
      "find_background": "#222427",
      "foreground": "ansi_white",
      "green": "ansi_green",
      "hint_action": "ansi256:244",
      "hint_key": "ansi256:250",
      "hover": "ansi_bright_black 12%",
      "info": "ansi_bright_cyan",
      "magenta": "ansi_magenta",
      "muted": "ansi_bright_black",
      "panel": "transparent",
      "primary": "ansi_bright_blue",
      "primary_text": "ansi_bright_blue",
      "red": "ansi_red",
      "selection": "ansi_black",
      "selection_text": "ansi_white",
      "subtitle": "ansi256:249",
      "success": "ansi_bright_green",
      "surface": "ansi_black 35%",
      "title_background": "#0c1634",
      "warning": "ansi_bright_cyan",
      "white": "ansi_white",
      "yellow": "ansi_yellow"
    }
  }
}
```

### Operational settings

#### `authentication`

| Field | Accepted values | Meaning |
| --- | --- | --- |
| `client_env_path` | `null` or a path | Path to the `.env` file with `BW_CLIENTID` and `BW_CLIENTSECRET`. `null` requests credentials in the TUI. `$HOME` and `~/` are expanded. |
| `cache_client_login` | `true` or `false` | Enables the persistent locked-login cache by region. With `false`, LazyBW deletes that cache and writes its disabled marker. It does not cache an unlocked session or the master password. |
| `default_server` | `"us"` or `"eu"` | Initial Bitwarden cloud region. Missing, `null`, or unrecognized values fall back to `"us"`. |
| `backend_mode` | `"bw"`, `"bw serve"`, or a non-empty ordered list without duplicates | Backend or fallback order. The default tries `bw serve`, then classic `bw`. `null` and a missing field are interpreted as `["bw", "bw serve"]` for compatibility. |

#### `local_replication`

| Field | Accepted values | Meaning |
| --- | --- | --- |
| `save_vault` | `true` or `false` | Enables automatic creation/update of the encrypted local replica after confirmed remote loading and after a confirmed push. It defaults to `false`. Running `lazybw replicate` explicitly still creates a replica when this is `false`. |
| `vault_path` | Non-empty path whose parent exists | Destination file for the replica. `$HOME` and `~/` are expanded. The destination must be a regular file if it already exists; LazyBW does not create its parent directory. |
| `vault_encryption` | `"aes-256-gcm"`, `"aes-256-gcm-siv"`, or `"xchacha20-poly1305"` | Authenticated-encryption algorithm for new replicas. The configured algorithm must match an existing replica's header to open it. Restore the old setting or regenerate the file after changing this value. |

#### `network`

| Field | Accepted values | Meaning |
| --- | --- | --- |
| `us_ping_url` | HTTPS URL accepting `HEAD` | Reachability target checked for the US region. It does **not** change Bitwarden's actual authentication or vault endpoint. |
| `eu_ping_url` | HTTPS URL accepting `HEAD` | Equivalent reachability target for the EU region; it also does not change the actual Bitwarden server. |

### Visual settings

`visual.icons.enabled` is the only visual option with behavioral impact: set it to `false` to suppress configured glyphs and use text-only markers (favorite markers become `f`). The nested `authentication`, `layout`, `items`, `contents`, and `dialogs` maps independently select Unicode glyphs for their named interface elements.

`visual.colors` is a theme map. Every value accepts `transparent` or `default`, `ansi_<color>`, `ansi_bright_<color>`, `ansi256:0` through `ansi256:255`, or `#RRGGBB`. Some surfaces additionally use a blend suffix such as `ansi_black 35%`. The role names cover the base ANSI palette, structure (`background`, `surface`, `panel`, `foreground`, `muted`), emphasis (`primary`, `accent`, `info`, `success`, `warning`, `error`), interaction (`selection`, `hover`, `button`, `border`), and supporting text.

For compatibility, a root-level `colors` object is accepted. If present, it takes precedence over `visual.colors`; new defaults are written under `visual.colors`.

## Configuration: `keybinds.jsonc`

### Default `keybinds.jsonc` file

This is the complete file generated by `lazybw reconfig --keybinds` (and on first run in an empty configuration directory):

```jsonc
// Each action accepts a list of keys. The first is used in hints.
// Examples: ["s", "enter"], ["ctrl+p"]. Existing strings still work.
{
  "authentication": {
    "backend": ["ctrl+b"],
    "button_left": ["left"],
    "button_right": ["right"],
    "next": ["down"],
    "previous": ["up"],
    "quit": ["ctrl+q"],
    "reveal_secret": ["ctrl+r"],
    "server": ["ctrl+s"],
    "submit": ["enter"]
  },
  "authentication_offline": {
    "backend": ["ctrl+b"],
    "button_left": ["left"],
    "button_right": ["right"],
    "quit": ["ctrl+q"],
    "server": ["ctrl+s"],
    "submit": ["enter"]
  },
  "backend_picker": {
    "cancel": ["escape"],
    "confirm": ["right", "enter"],
    "next": ["down"],
    "previous": ["up"]
  },
  "card_brand_popup": {
    "cancel": ["escape"],
    "confirm": ["right", "enter"],
    "next": ["down"],
    "previous": ["up"]
  },
  "confirmation_popup": {
    "accept": ["y"],
    "button_left": ["left"],
    "button_right": ["right"],
    "cancel": ["escape", "c"],
    "confirm": ["enter"],
    "next_page": ["?"],
    "reject": ["n"]
  },
  "content": {
    "back": ["left"],
    "copy": ["c"],
    "delete_field": ["d"],
    "edit": ["e", "right", "enter"],
    "favorite": ["f"],
    "new_field": ["n"],
    "next": ["down"],
    "previous": ["up"],
    "reveal": ["r"],
    "switch": ["s", "right", "enter"]
  },
  "create_field_popup": {
    "cancel": ["escape"],
    "confirm": ["right", "enter"],
    "next": ["down"],
    "previous": ["up"]
  },
  "create_item_popup": {
    "cancel": ["escape"],
    "confirm": ["right", "enter"],
    "next": ["down"],
    "previous": ["up"]
  },
  "detail": {
    "back": ["escape"],
    "blank_line_next": ["ctrl+down"],
    "blank_line_previous": ["ctrl+up"],
    "copy": ["ctrl+c"],
    "cursor_down": ["down"],
    "cursor_end": ["end"],
    "cursor_left": ["left"],
    "cursor_right": ["right"],
    "cursor_start": ["home"],
    "cursor_up": ["up"],
    "cut": ["ctrl+x"],
    "delete_backward": ["backspace"],
    "delete_forward": ["delete"],
    "delete_word_next": ["ctrl+delete", "alt+delete", "alt+d"],
    "delete_word_previous": ["ctrl+backspace", "alt+backspace"],
    "maximize": ["ctrl+e"],
    "newline": ["ctrl+j"],
    "paste": ["ctrl+v"],
    "redo": ["ctrl+y"],
    "reveal": ["ctrl+r"],
    "save": ["enter"],
    "select_all": ["ctrl+a"],
    "select_blank_line_next": ["ctrl+shift+down"],
    "select_blank_line_previous": ["ctrl+shift+up"],
    "select_down": ["shift+down"],
    "select_left": ["shift+left"],
    "select_right": ["shift+right"],
    "select_up": ["shift+up"],
    "select_word_next": ["ctrl+shift+right"],
    "select_word_previous": ["ctrl+shift+left"],
    "undo": ["ctrl+z"],
    "word_next": ["ctrl+right", "alt+right", "alt+f"],
    "word_previous": ["ctrl+left", "alt+left", "alt+b"],
    "wrap": ["ctrl+w"]
  },
  "edits": {
    "discard": ["d"],
    "next": ["down"],
    "previous": ["up"],
    "save": ["s", "enter"]
  },
  "filters": {
    "next": ["down"],
    "previous": ["up"],
    "return_to_items": ["escape"],
    "select": ["right", "enter"]
  },
  "folder_popup": {
    "button_left": ["left"],
    "button_right": ["right"],
    "cancel": ["escape"],
    "confirm": ["enter"],
    "next": ["down"],
    "previous": ["up"]
  },
  "folder_selector_popup": {
    "cancel": ["escape"],
    "confirm": ["enter"],
    "next": ["down"],
    "previous": ["up"],
    "switch": ["right", "s"]
  },
  "folders": {
    "delete": ["d"],
    "new": ["n"],
    "next": ["down"],
    "previous": ["up"],
    "return_to_items": ["escape"],
    "select": ["right", "enter"]
  },
  "global": {
    "discard_all": ["ctrl+d"],
    "focus_changes": ["4"],
    "focus_content": ["6"],
    "focus_folders": ["3"],
    "focus_item_types": ["2"],
    "focus_items": ["5"],
    "focus_my_vault": ["1"],
    "hint_next_page": ["?", "ctrl+n"],
    "push_all": ["ctrl+p"],
    "quit": ["ctrl+q"],
    "sync": ["ctrl+s"]
  },
  "identity_title_popup": {
    "cancel": ["escape"],
    "confirm": ["right", "enter"],
    "next": ["down"],
    "previous": ["up"]
  },
  "items": {
    "delete": ["d"],
    "new": ["n"],
    "next": ["down"],
    "open": ["right", "enter"],
    "previous": ["up"],
    "search": ["f"]
  },
  "linked_field_popup": {
    "cancel": ["escape"],
    "confirm": ["right", "enter"],
    "next": ["down"],
    "previous": ["up"]
  },
  "master_password": {
    "backend": ["ctrl+b"],
    "button_left": ["left"],
    "button_right": ["right"],
    "next": ["down"],
    "previous": ["up"],
    "quit": ["ctrl+q"],
    "reveal_password": ["ctrl+r"],
    "server": ["ctrl+s"],
    "submit": ["enter"]
  },
  "name_popup": {
    "button_left": ["left"],
    "button_right": ["right"],
    "cancel": ["escape"],
    "confirm": ["enter"],
    "next": ["down"],
    "previous": ["up"]
  },
  "offline": {
    "quit": ["enter", "escape", "ctrl+q"]
  },
  "reprompt_popup": {
    "button_left": ["left"],
    "button_right": ["right"],
    "cancel": ["escape"],
    "confirm": ["enter"],
    "reveal_password": ["ctrl+r"]
  },
  "search": {
    "apply": ["enter"],
    "button_left": ["left"],
    "button_right": ["right"],
    "leave": ["escape"],
    "previous": ["up"]
  },
  "server_picker": {
    "cancel": ["escape"],
    "confirm": ["right", "enter"],
    "next": ["down"],
    "previous": ["up"]
  },
  "sync_popup": {
    "abort": ["a", "escape"],
    "button_left": ["left"],
    "button_right": ["right"],
    "confirm": ["enter"],
    "hard_sync": ["h"],
    "next_page": ["?"],
    "soft_sync": ["s"]
  },
  "text_input": {
    "cursor_end": ["end"],
    "cursor_start": ["home"],
    "delete_backward": ["backspace"],
    "delete_forward": ["delete"],
    "delete_word_next": ["ctrl+delete", "alt+delete", "alt+d"],
    "delete_word_previous": ["ctrl+w", "ctrl+backspace", "alt+backspace"],
    "word_next": ["ctrl+right", "alt+right", "alt+f"],
    "word_previous": ["ctrl+left", "alt+left", "alt+b"]
  },
  "website_popup": {
    "cancel": ["escape"],
    "confirm": ["right", "enter"],
    "next": ["down"],
    "previous": ["up"]
  }
}
```

### Keybinding conventions

Each object is an active UI context. Each action accepts either one legacy string or an ordered list of keys; the first key in the list is displayed in footer hints. An empty list disables that configured action. Key names are case-insensitive and normalized; `esc` is accepted as `escape`, and arrow glyphs are accepted as their named counterparts.

Configured keys are aliases. Built-in navigation and confirmation keys remain available when the terminal supports them, even if omitted from configuration, as a safety measure. Avoid collisions with reserved terminal or text-entry controls.

For compatibility, `global.save_all` maps to `global.push_all` and the legacy `changes` section maps to `edits`.

### Keybinding areas

| Section | Screen/panel | What its actions do |
| --- | --- | --- |
| `global` | Workspace-wide commands | Quit, page through footer hints, focus the six normal workspace panes, inspect remote changes with `sync`, push all staged changes with `push_all`, or discard them with `discard_all`. `sync` reads and compares remote state; it never writes local edits. `push_all` performs a guarded remote verification before saving. |
| `authentication` | Connected API-key form | Move between controls, show/hide the API secret, open the server/backend picker, submit login, or quit. |
| `master_password` | Online unlock form | Same navigation model as authentication; submit unlocks the Bitwarden profile and the reveal action shows/hides the entered master password. |
| `authentication_offline` and `offline` | Unreachable/offline states | Select server/backend or exit. `offline` is the local-vault offline state and only provides exit. |
| `server_picker`, `backend_picker` | Region/backend selectors | Navigate choices, apply a selection, or cancel. Switching backend locks the previous backend and requires another unlock. |
| `filters` | Vault/type filter panes | Move, apply a filter, and return focus to items. |
| `folders` | Folder pane | Browse/select folders; create or delete a folder online; return to items. |
| `items` | Item list | Browse, open, search, create, or delete items (where permissions and mode allow). |
| `search` | Search overlay | Enter a query, leave the overlay, move through results, and select its buttons. |
| `content` | Fields/content pane | Browse fields, return to items, create or delete custom fields, copy or reveal values, edit a field, switch a checkbox, and toggle favorites. |
| `edits` | Staged-edits pane | Browse staged changes. `save` pushes the selected change after remote verification; `discard` asks to remove it from the in-memory stage. |
| `detail` | Multiline detail editor | Save/cancel, reveal a secret, maximize or wrap, cut/copy/paste, undo/redo, select, move by character/word/line/blank line, and delete text. |
| `text_input` | Single-line text entry | Home/end, deletion, word movement, and word deletion for dialogs and forms. |
| `confirmation_popup` | General confirmation dialog | Accept/reject, choose buttons, cancel, and page long confirmation text. |
| `sync_popup` | Remote-change review | Choose `soft_sync`, `hard_sync`, or `abort`; move buttons and page the review. Soft sync keeps and reconciles staged work where possible. Hard sync discards conflicting staged work while retaining independent local changes. |
| `create_item_popup`, `create_field_popup` | Type creation selectors | Choose a new item/custom-field type or cancel. |
| `linked_field_popup`, `folder_selector_popup`, `card_brand_popup`, `identity_title_popup`, `website_popup` | Specialized selectors | Navigate, choose/toggle the relevant linked field, folder, card brand, identity title, or website, then confirm or cancel. |
| `name_popup`, `folder_popup` | Item/field/folder naming dialogs | Navigate controls, choose buttons, confirm, or cancel. |
| `reprompt_popup` | Master-password re-prompt | Reveal/hide the password, confirm/cancel, and choose dialog buttons when an item requires re-prompt verification. |

## Staging, push, and synchronization

### In-memory staging

Edits are **not** written immediately. LazyBW retains ordered `Before` and `After` snapshots for each changed item or folder in an in-memory stage. It preserves the original baseline through successive changes, and removes a staged entry if the current value returns to that baseline. Staged edits disappear when the application closes; LazyBW warns before quitting with pending work.

The staged-edits panel lets you inspect, save, or discard individual changes. Global push/discard performs the corresponding operation for the complete stage. In local-replica mode staging and all mutation actions are disabled.

### Guarded push

Before saving any staged change, LazyBW executes a remote `sync`, reads the remote items and folders, and compares them with the last confirmed in-memory baseline. If that verification fails or detects differences, it does not write and presents synchronization review instead.

A full push is ordered to preserve references: new folders first, normal item/folder changes next, item deletions next, and folder deletions last. It will not push an item pointing to a local folder that is absent from the selected push plan. Permission checks prevent editing items without `edit`, creating organization items, or changing protected secrets when `viewPassword` is absent.

The batch is not transactional: writes occur sequentially, so a later error can happen after earlier successful writes. Each successful write is marked unverified until LazyBW performs a final `sync` and canonical remote read. If that final confirmation fails, later pushes are blocked until a manual `SYNC` succeeds. A remote change that happens after the pre-write comparison but before/during a save also cannot be fully prevented.

### Remote synchronization choices

`SYNC` only retrieves and compares remote data; it never sends staged edits.

- **Soft sync** updates the remote baseline and preserves staged edits. LazyBW performs a field-level three-way merge where possible. Use it when you want to retain local work and reconcile independent remote updates.
- **Hard sync** updates the remote baseline and discards *conflicting* local staged edits. It does not blindly erase every local change: independent staged changes whose remote baseline did not change, and newly created local entities, can remain.
- **Abort** leaves the current in-memory state untouched.

Remote changes imported during review are shown as session information and are not restaged as local edits.

## `bw` and `bw serve` backends

`authentication.backend_mode` accepts a backend name or an ordered fallback list. LazyBW chooses the first available one during initial connection. A later explicit selection is retained; changing backend locks the old backend, clears the unlocked token and re-prompt verifier, and requires the vault to be unlocked again.

| Aspect | `bw` | `bw serve` |
| --- | --- | --- |
| Availability | Standard Bitwarden CLI mode. | Requires Bitwarden CLI **2025.9.0+** and support for `bw serve --hostname`. |
| Transport | Starts individual CLI commands. | Starts a LazyBW-owned `bw serve` process and sends HTTP requests over a private Unix socket. |
| Login | Performs API-key login directly through the CLI. | Login still uses a temporary classic CLI invocation because serve does not expose login routes. |
| Unlock | Runs `bw unlock --raw --passwordenv`. | Uses the serve `/unlock` route. |
| Status before login | CLI `bw status`. | Also relies on `bw status` before authentication because serve cannot provide usable status then. |
| Operations | Supports all operations LazyBW needs through CLI commands and templates. | Handles CRUD, `sync`, lock, list, and get through serve routes. |
| Isolation/security | Uses a per-session `BITWARDENCLI_APPDATA_DIR`; clears inherited `BW_SESSION` and injects only the current LazyBW token. | Same profile isolation plus a Unix-socket directory mode `0700` and socket mode `0600`. LazyBW never discovers or reuses external serve processes. |
| Trade-off | Broad CLI compatibility, but repeated process invocations. | Preferred by default when available; keeps a private local service but requires a sufficiently new CLI and Unix-socket support. |

If `bw serve` is unavailable, the default `["bw serve", "bw"]` configuration falls back to `bw`. A configuration containing only `"bw serve"` does not have that fallback.

## Encrypted local replicas

### Purpose and limits

A replica is an encrypted snapshot of confirmed **personal-vault** items and folders. Organization-owned items are filtered out. It is read-only: when open, LazyBW permits browsing, filtering, searching, copying, and revealing values, but never pushes, stages, creates, edits, or deletes remote data.

A replica is based only on confirmed remote data, never unsaved staged edits. It can be generated automatically after confirmed online loading or a confirmed push when `save_vault` is true, or manually with `lazybw replicate` regardless of that setting.

### Configure and create a replica

Create the parent directory first, then enable replication:

```bash
mkdir -p ~/.local/share/lazybw
```

```jsonc
"local_replication": {
  "save_vault": true,
  "vault_path": "$HOME/.local/share/lazybw/bw-vault",
  "vault_encryption": "aes-256-gcm"
}
```

To force a fresh replica without opening the TUI:

```bash
lazybw replicate
```

The command validates settings, chooses a backend, logs in by API key when needed, unlocks with the master password, synchronizes and downloads the vault, removes organization data, encrypts it, and atomically replaces the destination. It reports its progress. Override configured paths only when necessary:

```bash
lazybw replicate --path "$HOME/.local/share/lazybw/bw-vault" \
  --client "$HOME/.config/lazybw/credentials.env"
```

`--path` must name a destination with no extension and `--client` must name a `.env` file. Omitting `--password` uses a hidden controlling-terminal prompt; passing it as an argument is less safe because shells may retain it in history and local processes may briefly observe arguments.

### File handling and opening offline

The output is written to a temporary file with mode `0600`, `fsync`ed, renamed atomically, and followed by a directory sync. The previous replica is not replaced if the operation is cancelled before the rename.

Opening requires the exact password used to encrypt that replica and the currently configured `vault_encryption` algorithm. Changing `vault_encryption` can make an existing file unavailable until the previous setting is restored or the replica is regenerated online.

## Encryption and security

LazyBW applies defense in depth but cannot protect against every threat in a terminal environment. A privileged process, a compromised terminal emulator or SSH host, a multiplexer, or another process that already controls the PTY can observe input; no terminal application can fully prevent that.

### In-memory sensitive state

At session startup, LazyBW generates 64 random bytes: 32 bytes for AES-256-GCM encryption and 32 bytes for HMAC-SHA-256. Sensitive application values are held as authenticated sealed values with nonce, ciphertext, original length, and tag. Passwords, TOTP values, card and identity data, SSH private keys, hidden custom fields, and notes are sealed when received or changed.

On Linux, LazyBW prefers the Linux Kernel Keyring for the session key. If unavailable, including on unsupported platforms, it falls back to process memory. It clears/revokes session material where practical at shutdown. This reduces exposure in working application state but does not mean plaintext can never transiently exist while communicating with `bw`, serializing a push, or creating a replica.

The master-password re-prompt does not retain plaintext for comparison: it stores an HMAC-derived verifier and compares it in constant time. Payloads sent to classic `bw` writes are passed through standard input instead of command-line arguments; serve payloads travel through the private Unix socket.

### Replica encryption at rest

All replica algorithms are authenticated-encryption schemes with a 256-bit key:

| Setting | Implementation | Characteristics |
| --- | --- | --- |
| `aes-256-gcm` | Go standard library (`crypto/aes`, `crypto/cipher`) | AES-GCM AEAD. |
| `aes-256-gcm-siv` | `github.com/ericlagergren/siv` | AES-GCM-SIV AEAD. |
| `xchacha20-poly1305` | `golang.org/x/crypto/chacha20poly1305` | XChaCha20-Poly1305 AEAD. |

The master password is transformed with Argon2id using a fresh random 16-byte salt, 64 MiB memory, time cost 3, two threads, and a 32-byte output key. The public replica header records version, origin region, creation time, algorithm, KDF, salt, and KDF parameters. That header is authenticated as AEAD associated data, so modifying it causes authentication failure.

The serialized record additionally carries SHA-256 for early detection of accidental corruption before running the KDF. That checksum is not the cryptographic authenticity mechanism; AEAD authentication protects the header and encrypted snapshot. While reading, LazyBW accepts regular files up to 128 MiB only and validates version, KDF, algorithm, KDF parameter limits, salt, nonce, and ciphertext structure.

### Terminal and process protections

The TUI uses terminal raw mode, which disables echo, canonical input buffering, and signal-generating control characters during text entry. Text-entry keystrokes are not written to LazyBW's key-feedback state. Hidden CLI password prompts similarly read from the controlling terminal with echo disabled.

On Linux, LazyBW disables core dumps, marks the process non-dumpable, requests exclusive TTY access while secret input is active, and restores terminal state on termination signals where practical. These protections are additive safeguards, not substitutes for trusting the local OS, terminal, and remote host.

## CLI reference

```text
lazybw
lazybw replicate [--path FILE] [--client FILE.env] [--password VALUE]
lazybw reconfig [--settings] [--keybinds]
lazybw --help
```

| Command | Description |
| --- | --- |
| `lazybw` | Starts the interactive TUI. It loads/generates configuration, checks its isolated Bitwarden profile, then guides authentication, unlock, online workspace, or local mode as applicable. |
| `lazybw --help` / `lazybw -h` | Prints the embedded usage text. These are treated as global help only when used alone. |
| `lazybw replicate` | Creates or updates the encrypted local replica. It runs even when `local_replication.save_vault` is `false`, because invocation explicitly requests replication. It accepts no positional arguments. |
| `lazybw reconfig` | Restores generated configuration. Without flags, overwrites both `settings.jsonc` and `keybinds.jsonc`; it accepts no positional arguments. |

### `replicate` flags

| Flag | Meaning |
| --- | --- |
| `--path FILE` | Overrides `local_replication.vault_path`. Must not be empty or have a file extension. |
| `--client FILE.env` | Overrides `authentication.client_env_path`. Must not be empty and must end in `.env`. |
| `--password VALUE` | Supplies the Bitwarden master password. If omitted, LazyBW prompts through the controlling terminal with echo disabled. Prefer the prompt to avoid shell history and process-list exposure. An empty value is rejected. |

### `reconfig` flags

| Flag | Meaning |
| --- | --- |
| `--settings` | Restores only `settings.jsonc`. |
| `--keybinds` | Restores only `keybinds.jsonc`. |

`LAZYBW_CONFIG_DIR` changes the configuration directory for all commands. It is useful for isolated profiles, tests, and non-default locations.
