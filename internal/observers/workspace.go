package observers

import (
	"context"
	"log/slog"

	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/observers/cgo_bridge"
)

type WorkspaceObserver struct {
	bus    *events.Bus
	axMgr  *AccessibilityManager
	logger *slog.Logger
}

func NewWorkspaceObserver(
	bus *events.Bus,
	axMgr *AccessibilityManager,
	logger *slog.Logger,
) *WorkspaceObserver {
	return &WorkspaceObserver{bus: bus, axMgr: axMgr, logger: logger}
}

func (o *WorkspaceObserver) Run(ctx context.Context) {
	ch := cgo_bridge.EventCh()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}

			o.handle(e)
		}
	}
}

func (o *WorkspaceObserver) handle(e events.Event) {
	switch e.Kind {
	case events.AppActivate, events.AppLaunch:
		if e.PID > 0 {
			if ok := o.axMgr.Install(e.PID); !ok {
				o.logger.Debug("AX observer install failed",
					"pid", e.PID, "app", e.AppName)
			}
		}
	case events.AppQuit:
		if e.PID > 0 {
			o.axMgr.Remove(e.PID)
		}
	}

	o.logger.Info("event",
		"kind", e.Kind,
		"app", e.AppName,
		"bundle", e.BundleID,
		"pid", e.PID,
		"title", e.WindowTitle,
		"vol", e.VolumeName,
	)
	o.bus.Publish(e)
}
