//go:build !linux

package platform

import "context"

// NewBackgroundTray returns a no-op tray on platforms without the Linux
// StatusNotifier integration.
func NewBackgroundTray() BackgroundTray {
	return noopBackgroundTray{}
}

type noopBackgroundTray struct{}

func (noopBackgroundTray) Start(context.Context, func()) error {
	return nil
}

func (noopBackgroundTray) Stop() error {
	return nil
}
