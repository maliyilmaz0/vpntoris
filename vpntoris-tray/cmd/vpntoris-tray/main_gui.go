//go:build linux || windows

package main

import (
	"fyne.io/systray"
)

func main() {
	app := newTrayApp()
	systray.Run(app.onReady, app.onExit)
}
