# Bitwarden item dictionary

`bitwarden_schema.json` declares the built-in metadata for:

- Item types, IDs, labels, and icons
- Default fields with JSON object paths and hidden status
- Selectable `Brand` and `Title` values
- Custom field types and linked field targets
- The item creation order

The JSON is embedded into the Go binary at build time. To update these
definitions, edit the dictionary and rebuild LazyBW; the file is not fetched
or modified at runtime. Dynamic fields are rendered from this dictionary.

The schema is maintained locally, **not** downloaded from the Bitwarden CLI.
Adding a genuinely new Bitwarden item or field format may still require
changes in parsing, editing, synchronization and permissions, even if its
metadata is represented here. Keep any new IDs aligned with Bitwarden's
actual API format.
