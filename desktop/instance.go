package main

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// singleInstanceID identifies this app to Wails' single-instance lock. It is
// the same for every copy of the executable, whatever its file name — a
// browser saves repeat downloads as "stash-desktop (2).exe", and those must
// still count as the same app.
const singleInstanceID = "com.plusclouds.stash-desktop"

// singleInstanceLock makes a second launch hand over to the copy that is
// already running instead of starting another one.
//
// Closing the window only hides it (HideWindowOnClose), so the app keeps
// running where it is easy to forget about. Launching it again used to start
// a second process, whose embedded agent then failed with "another
// backup-agent instance is already running" — an error about a lock file,
// shown to someone who only wanted their window back.
func singleInstanceLock(app *App) *options.SingleInstanceLock {
	return &options.SingleInstanceLock{
		UniqueId:               singleInstanceID,
		OnSecondInstanceLaunch: app.onSecondInstance,
	}
}

// onSecondInstance runs in the already-running copy when the app is launched
// again: bring the window back, which is what that launch was asking for.
func (a *App) onSecondInstance(_ options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	wailsruntime.WindowUnminimise(a.ctx)
	wailsruntime.WindowShow(a.ctx)
}
