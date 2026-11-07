package tray

import (
	"gemini-switcher/internal/icons"
	"strings"
)

var (
	IconGreen  = icons.Green
	IconYellow = icons.Yellow
	IconRed    = icons.Red
	IconGray   = icons.Gray
	IconBytes  = IconGreen
)

func GetIconBytes(color string) []byte {
	switch strings.ToLower(strings.TrimSpace(color)) {
	case "green":
		return IconGreen
	case "yellow":
		return IconYellow
	case "red":
		return IconRed
	case "gray", "grey":
		return IconGray
	default:
		return IconBytes
	}
}
