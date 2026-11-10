package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	colorText      = -1
	colorDim       = 243
	colorFaint     = 240
	colorBorder    = 238
	colorAccent    = 208
	colorGreen     = 71
	colorRed       = 160
	colorYellow    = 178
	colorSelection = 237
)

const ansiReset = "\x1b[0m"

type theme struct {
	enabled bool
}

func newTheme() theme {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return theme{}
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return theme{}
	}
	return theme{enabled: true}
}

func (t theme) fg(color int, s string) string {
	if !t.enabled || color < 0 {
		return s
	}
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[39m", color, s)
}

func (t theme) bg(color int, s string) string {
	if !t.enabled {
		return s
	}
	return fmt.Sprintf("\x1b[48;5;%dm%s\x1b[49m", color, s)
}

func (t theme) bold(s string) string {
	if !t.enabled {
		return s
	}
	return "\x1b[1m" + s + "\x1b[22m"
}

func (theme) reverse(s string) string {
	return "\x1b[7m" + s + "\x1b[27m"
}
