package tui

import (
	_ "embed"
	"strings"
)

//go:embed logo.txt
var embeddedLogo string

func logoText() string {
	return strings.TrimRight(embeddedLogo, "\n")
}
