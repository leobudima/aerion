package app

import (
	"fmt"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// RefreshWindowConstraints removes window max size constraints. This works
// around a Wails v2 Linux limitation where GTK geometry hints are set once at
// startup using the initial monitor's dimensions, causing the window to be
// stuck at that size when moving to a larger monitor.
// We use a large value instead of 0,0 because GTK may interpret zero as
// "use current hints" rather than "remove constraints".
func (a *App) RefreshWindowConstraints() {
	wailsRuntime.WindowSetMaxSize(a.ctx, 100000, 100000)
}

// RestoreMainWindowState applies the last saved main window bounds before the
// frontend shows the window.
func (a *App) RestoreMainWindowState() error {
	if a.appStateStore == nil {
		return nil
	}

	state, err := a.appStateStore.GetUIState()
	if err != nil {
		return err
	}

	if state.WindowWidth >= 360 && state.WindowHeight >= 400 {
		wailsRuntime.WindowSetSize(a.ctx, state.WindowWidth, state.WindowHeight)
	}
	if state.WindowWidth > 0 && state.WindowHeight > 0 {
		wailsRuntime.WindowSetPosition(a.ctx, state.WindowX, state.WindowY)
	}
	if state.WindowMaximized {
		wailsRuntime.WindowMaximise(a.ctx)
	}

	return nil
}

// SaveMainWindowState stores the current native window bounds in the shared UI
// state blob so it survives app restarts.
func (a *App) SaveMainWindowState() error {
	if a.appStateStore == nil {
		return nil
	}

	state, err := a.appStateStore.GetUIState()
	if err != nil {
		return err
	}

	width, height := wailsRuntime.WindowGetSize(a.ctx)
	x, y := wailsRuntime.WindowGetPosition(a.ctx)
	if width < 360 || height < 400 {
		return fmt.Errorf("refusing to save invalid window size %dx%d", width, height)
	}

	state.WindowX = x
	state.WindowY = y
	state.WindowWidth = width
	state.WindowHeight = height
	state.WindowMaximized = wailsRuntime.WindowIsMaximised(a.ctx)

	return a.appStateStore.SaveUIState(state)
}

// RefreshWindowConstraints removes window max size constraints for the
// composer window. See App.RefreshWindowConstraints for details.
func (c *ComposerApp) RefreshWindowConstraints() {
	wailsRuntime.WindowSetMaxSize(c.ctx, 100000, 100000)
}
