//go:build linux

package platform

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
	"github.com/hkdb/aerion/internal/logging"
)

const (
	statusNotifierWatcherDest  = "org.kde.StatusNotifierWatcher"
	statusNotifierWatcherPath  = "/StatusNotifierWatcher"
	statusNotifierWatcherIface = "org.kde.StatusNotifierWatcher"
	statusNotifierItemIface    = "org.kde.StatusNotifierItem"
	statusNotifierItemPath     = "/StatusNotifierItem"
)

// NewBackgroundTray creates a desktop tray item for background mode.
func NewBackgroundTray() BackgroundTray {
	return &linuxBackgroundTray{}
}

type linuxBackgroundTray struct {
	mu         sync.Mutex
	conn       *dbus.Conn
	props      *prop.Properties
	service    string
	onActivate func()
	running    bool
}

func (t *linuxBackgroundTray) Start(ctx context.Context, onActivate func()) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		t.onActivate = onActivate
		return nil
	}

	log := logging.WithComponent("background-tray")
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to session D-Bus for tray icon")
		return err
	}

	service := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	reply, err := conn.RequestName(service, dbus.NameFlagDoNotQueue)
	if err != nil {
		conn.Close()
		log.Warn().Err(err).Msg("Failed to request StatusNotifierItem bus name")
		return err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		err := fmt.Errorf("status notifier bus name is already owned: reply=%d", reply)
		log.Warn().Err(err).Msg("Failed to own StatusNotifierItem bus name")
		return err
	}

	path := dbus.ObjectPath(statusNotifierItemPath)
	item := &statusNotifierItem{tray: t}
	if err := conn.Export(item, path, statusNotifierItemIface); err != nil {
		conn.ReleaseName(service)
		conn.Close()
		log.Warn().Err(err).Msg("Failed to export StatusNotifierItem")
		return err
	}

	props, err := prop.Export(conn, path, prop.Map{
		statusNotifierItemIface: {
			"Category":      {Value: "ApplicationStatus", Emit: prop.EmitConst},
			"Id":            {Value: "aerion", Emit: prop.EmitConst},
			"Title":         {Value: "Aerion", Emit: prop.EmitConst},
			"Status":        {Value: "Active", Emit: prop.EmitTrue},
			"WindowId":      {Value: uint32(0), Emit: prop.EmitConst},
			"IconName":      {Value: "io.github.hkdb.Aerion", Emit: prop.EmitConst},
			"IconThemePath": {Value: "", Emit: prop.EmitConst},
			"ItemIsMenu":    {Value: false, Emit: prop.EmitConst},
			"Menu":          {Value: dbus.ObjectPath("/"), Emit: prop.EmitConst},
		},
	})
	if err != nil {
		conn.ReleaseName(service)
		conn.Close()
		log.Warn().Err(err).Msg("Failed to export StatusNotifierItem properties")
		return err
	}

	conn.Export(introspect.NewIntrospectable(statusNotifierIntrospection()), path, "org.freedesktop.DBus.Introspectable")

	watcher := conn.Object(statusNotifierWatcherDest, dbus.ObjectPath(statusNotifierWatcherPath))
	call := watcher.Call(statusNotifierWatcherIface+".RegisterStatusNotifierItem", 0, service)
	if call.Err != nil {
		conn.ReleaseName(service)
		conn.Close()
		log.Warn().Err(call.Err).Msg("Failed to register StatusNotifierItem with watcher")
		return call.Err
	}

	t.conn = conn
	t.props = props
	t.service = service
	t.onActivate = onActivate
	t.running = true

	go func() {
		<-ctx.Done()
		_ = t.Stop()
	}()

	log.Info().Msg("Background tray icon registered")
	return nil
}

func (t *linuxBackgroundTray) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return nil
	}

	if t.conn != nil {
		if t.props != nil {
			t.props.SetMust(statusNotifierItemIface, "Status", "Passive")
		}
		if t.service != "" {
			_, _ = t.conn.ReleaseName(t.service)
		}
		t.conn.Close()
	}

	t.conn = nil
	t.props = nil
	t.service = ""
	t.onActivate = nil
	t.running = false

	log := logging.WithComponent("background-tray")
	log.Info().Msg("Background tray icon stopped")
	return nil
}

func (t *linuxBackgroundTray) activate() {
	t.mu.Lock()
	onActivate := t.onActivate
	t.mu.Unlock()

	if onActivate != nil {
		go onActivate()
	}
}

type statusNotifierItem struct {
	tray *linuxBackgroundTray
}

func (i *statusNotifierItem) Activate(_ int32, _ int32) *dbus.Error {
	i.tray.activate()
	return nil
}

func (i *statusNotifierItem) SecondaryActivate(_ int32, _ int32) *dbus.Error {
	i.tray.activate()
	return nil
}

func (i *statusNotifierItem) ContextMenu(_ int32, _ int32) *dbus.Error {
	i.tray.activate()
	return nil
}

func (i *statusNotifierItem) Scroll(_ int32, _ string) *dbus.Error {
	return nil
}

func statusNotifierIntrospection() *introspect.Node {
	return &introspect.Node{
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{
				Name: statusNotifierItemIface,
				Methods: []introspect.Method{
					{Name: "ContextMenu", Args: []introspect.Arg{
						{Name: "x", Type: "i", Direction: "in"},
						{Name: "y", Type: "i", Direction: "in"},
					}},
					{Name: "Activate", Args: []introspect.Arg{
						{Name: "x", Type: "i", Direction: "in"},
						{Name: "y", Type: "i", Direction: "in"},
					}},
					{Name: "SecondaryActivate", Args: []introspect.Arg{
						{Name: "x", Type: "i", Direction: "in"},
						{Name: "y", Type: "i", Direction: "in"},
					}},
					{Name: "Scroll", Args: []introspect.Arg{
						{Name: "delta", Type: "i", Direction: "in"},
						{Name: "orientation", Type: "s", Direction: "in"},
					}},
				},
				Properties: []introspect.Property{
					{Name: "Category", Type: "s", Access: "read"},
					{Name: "Id", Type: "s", Access: "read"},
					{Name: "Title", Type: "s", Access: "read"},
					{Name: "Status", Type: "s", Access: "read"},
					{Name: "WindowId", Type: "u", Access: "read"},
					{Name: "IconName", Type: "s", Access: "read"},
					{Name: "IconThemePath", Type: "s", Access: "read"},
					{Name: "ItemIsMenu", Type: "b", Access: "read"},
					{Name: "Menu", Type: "o", Access: "read"},
				},
				Signals: []introspect.Signal{
					{Name: "NewTitle"},
					{Name: "NewIcon"},
					{Name: "NewAttentionIcon"},
					{Name: "NewOverlayIcon"},
					{Name: "NewToolTip"},
					{Name: "NewStatus", Args: []introspect.Arg{{Name: "status", Type: "s"}}},
				},
			},
		},
	}
}
