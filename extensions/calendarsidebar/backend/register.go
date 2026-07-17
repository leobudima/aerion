// Package backend implements the Calendar Sidebar extension.
//
// The extension is UI-only: it renders an upcoming-events agenda panel
// inside the mail view, backed entirely by the Calendar extension's data
// (via the Calendar_* Wails bridge from the frontend). It owns no stores,
// no sync, and no per-extension SQLite.
package backend

import (
	"github.com/hkdb/aerion/extensions/calendarsidebar"
	coreapi "github.com/hkdb/aerion/internal/core/api/v1"
)

// Extension is the Calendar Sidebar extension's lifecycle handle — just the
// manifest plus the Register handshake, per the lightweight-by-default rule.
type Extension struct {
	manifest coreapi.Manifest
}

// NewExtension constructs the Extension lifecycle handle.
func NewExtension() *Extension {
	return &Extension{manifest: calendarsidebar.Manifest()}
}

// Manifest returns the parsed manifest embedded at build time.
func (e *Extension) Manifest() coreapi.Manifest { return e.manifest }

// Register wires the extension's UI surfaces. The sidebar panel is hosted
// directly by the mail layout in App.svelte (a mail-view side panel is not
// yet a registerable surface — RegisterInboxView and friends are Phase 3+),
// so there is nothing to register today. The registration handshake still
// runs so the extension appears in Settings → Extensions and participates
// in the enable/disable flow like every other extension.
func (e *Extension) Register(core coreapi.Core) (coreapi.Unregister, error) {
	return func() {}, nil
}

// compile-time check: *Extension satisfies coreapi.Extension
var _ coreapi.Extension = (*Extension)(nil)
