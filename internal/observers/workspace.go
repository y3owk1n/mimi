package observers

import (
	"context"

	"go.uber.org/zap"

	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/observers/cgo_bridge"
)

// WorkspaceObserver receives CGO bridge events and routes them to the bus.
type WorkspaceObserver struct {
	bus    *events.Bus
	axMgr  *AccessibilityManager
	logger *zap.SugaredLogger
}

// NewWorkspaceObserver creates a new workspace observer.
func NewWorkspaceObserver(
	bus *events.Bus,
	axMgr *AccessibilityManager,
	logger *zap.SugaredLogger,
) *WorkspaceObserver {
	return &WorkspaceObserver{bus: bus, axMgr: axMgr, logger: logger}
}

// Run starts the observer loop, consuming events and routing them.
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

func (o *WorkspaceObserver) handle(evt events.Event) {
	switch evt.Kind { //nolint:exhaustive
	case events.AppActivate, events.AppLaunch:
		if evt.PID > 0 {
			if ok := o.axMgr.Install(evt.PID); !ok {
				o.logger.Debugw("AX observer install failed",
					"pid", evt.PID, "app", evt.AppName)
			}
		}
	case events.AppQuit:
		if evt.PID > 0 {
			o.axMgr.Remove(evt.PID)
		}
	default:
	}

	o.logger.Debugw("event",
		"kind", evt.Kind,
		"app", evt.AppName,
		"bundle", evt.BundleID,
		"pid", evt.PID,
		"title", evt.WindowTitle,
		"vol", evt.VolumeName,
	)
	o.bus.Publish(evt)
}
