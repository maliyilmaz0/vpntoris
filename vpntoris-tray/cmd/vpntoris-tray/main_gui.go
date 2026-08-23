//go:build linux || windows

package main

import (
	"fmt"
	"os"

	"fyne.io/systray"
)

func main() {
	release, err := acquireSingleInstance()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(0)
	}
	defer release()

	ensureDaemonRunning()

	app := newTrayApp()
	systray.Run(app.onReady, app.onExit)
}
