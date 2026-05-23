package platform

import "context"

// BackgroundTray exposes the small subset of tray behavior Aerion needs while
// the main window is hidden in background mode.
type BackgroundTray interface {
	Start(ctx context.Context, onActivate func()) error
	Stop() error
}
