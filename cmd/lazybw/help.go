package main

import _ "embed"

// helpText is embedded so the installed binary always carries its editable CLI documentation.
//
//go:embed help.txt
var helpText string
