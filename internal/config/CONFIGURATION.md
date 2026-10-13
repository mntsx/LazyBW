# LazyBW configuration

`settings.jsonc` and `keybinds.jsonc` are created the first time the program
loads an empty configuration directory. Existing files are **not overwritten**.

## Colors

`settings.jsonc` contains a `colors` object. Colors accept:

- ANSI palette names such as `ansi_red` and `ansi_bright_green`.
- True color strings like `#88AACC`.
- Palette indices `ansi256:0` through `ansi256:255`.
- `transparent` or `default` for the terminal default.

The 16 ANSI color roles (`red`, `bright_red`, etc.) permit changing the
existing UI palette. The additional `foreground`, `primary`, `muted`,
`hint_key`, `hint_action`, `subtitle`, `title_background`, and
`find_background` roles are also supported. Some other colors are still
inherited from the palette rather than having separate UI element-specific
controls.

## Keybindings

Each section represents a panel, state, or popup. An action accepts a list of
keys, with the **first** used in the footer. For example:

```jsonc
{
  "content": {
    "switch": ["s", "right", "enter", "v"]
  }
}
```

A single string is still accepted for backward compatibility. Legacy
`global.save_all` and `changes` keys are mapped to `global.push_all` and
`edits`, respectively.

Known terminal keys are supported as built-ins. Configured additional keys
are registered as aliases when they do not collide with reserved keys or
text-entry controls. Built-in navigation and confirmation keys remain
available for safety even when not listed in the user's configuration.

Generated JSONC comments list the finite valid values for fixed-choice
settings. Do not edit the `schema_version` property of the Bitwarden schema
unless its data format is changed together with the loader.
