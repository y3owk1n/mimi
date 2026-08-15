package config

import "github.com/y3owk1n/mimi/internal/events"

// HookGroup classifies a hook kind by the macOS observer machinery it needs,
// mirroring the three sections users already see in docs/CONFIGURATION.md:
// "Application Lifecycle", "Window events (requires Accessibility)" and
// "Workspace events".
//
// The group says which kinds belong together, not which observers the daemon
// switches on for them — that mapping is a policy decision (window hooks also
// need the app-lifecycle observer, because AX observers attach per process and
// rely on launch/terminate notifications to attach and detach) and lives in
// internal/daemon, where it can be read alongside its reason.
type HookGroup int

// The hook groups. Every HookKind belongs to exactly one.
const (
	// GroupApp covers the app lifecycle kinds, which need no extra permission.
	GroupApp HookGroup = iota
	// GroupWindow covers the window kinds, which require Accessibility.
	GroupWindow
	// GroupWorkspace covers the Space/Desktop kinds.
	GroupWorkspace
)

// HookKind describes one hookable event kind as it appears in config: the
// events.EventKind it publishes as, the TOML key users write under [hooks],
// its group, and an accessor to its slice inside a HooksConfig.
type HookKind struct {
	// Kind is the event kind published on the bus and matched by the registry.
	Kind events.EventKind
	// TOMLKey is the key users write under [hooks] in their config file.
	TOMLKey string
	// Group classifies the kind; see HookGroup.
	Group HookGroup
	// Entries returns a pointer to this kind's entries inside a HooksConfig,
	// so both decoding (write) and every enumeration (read) can fold over the
	// same table rather than restating the twelve field names.
	Entries func(*HooksConfig) *[]HookEntry
}

// HookKinds is the single source of truth for the hookable event kinds on the
// config side. Decoding, validation, the hook registry's kind map and the
// daemon's observer wiring are all folds over this slice, so adding a kind is
// one row plus one HooksConfig field rather than an edit in six places.
//
// The order matches HooksConfig's field order, which is also the order used by
// configs/default-config.toml and docs/CONFIGURATION.md. It is user-visible:
// hook errors are reported in this order.
//
// The cross-language boundary is a separate concern. eventkinds.h, kindFromInt
// and internal/native/eventkinds_test.go own the C enum's agreement with
// events.EventKind, including the kinds deliberately not routed to Go.
var HookKinds = []HookKind{
	{
		Kind:    events.AppActivate,
		TOMLKey: "on_app_activate",
		Group:   GroupApp,
		Entries: func(h *HooksConfig) *[]HookEntry { return &h.AppActivate },
	},
	{
		Kind:    events.AppDeactivate,
		TOMLKey: "on_app_deactivate",
		Group:   GroupApp,
		Entries: func(h *HooksConfig) *[]HookEntry { return &h.AppDeactivate },
	},
	{
		Kind:    events.AppLaunch,
		TOMLKey: "on_app_launch",
		Group:   GroupApp,
		Entries: func(h *HooksConfig) *[]HookEntry { return &h.AppLaunch },
	},
	{
		Kind:    events.AppQuit,
		TOMLKey: "on_app_quit",
		Group:   GroupApp,
		Entries: func(h *HooksConfig) *[]HookEntry { return &h.AppQuit },
	},
	{
		Kind:    events.AppHide,
		TOMLKey: "on_app_hide",
		Group:   GroupApp,
		Entries: func(h *HooksConfig) *[]HookEntry { return &h.AppHide },
	},
	{
		Kind:    events.AppUnhide,
		TOMLKey: "on_app_unhide",
		Group:   GroupApp,
		Entries: func(h *HooksConfig) *[]HookEntry { return &h.AppUnhide },
	},
	{
		Kind:    events.WindowFocus,
		TOMLKey: "on_window_focus",
		Group:   GroupWindow,
		Entries: func(h *HooksConfig) *[]HookEntry { return &h.WindowFocus },
	},
	{
		Kind:    events.WindowTitleChange,
		TOMLKey: "on_window_title_change",
		Group:   GroupWindow,
		Entries: func(h *HooksConfig) *[]HookEntry { return &h.WindowTitleChange },
	},
	{
		Kind:    events.WindowCreated,
		TOMLKey: "on_window_created",
		Group:   GroupWindow,
		Entries: func(h *HooksConfig) *[]HookEntry { return &h.WindowCreated },
	},
	{
		Kind:    events.WindowClosed,
		TOMLKey: "on_window_closed",
		Group:   GroupWindow,
		Entries: func(h *HooksConfig) *[]HookEntry { return &h.WindowClosed },
	},
	{
		Kind:    events.WindowResize,
		TOMLKey: "on_window_resize",
		Group:   GroupWindow,
		Entries: func(h *HooksConfig) *[]HookEntry { return &h.WindowResize },
	},
	{
		Kind:    events.WorkspaceChanged,
		TOMLKey: "on_workspace_changed",
		Group:   GroupWorkspace,
		Entries: func(h *HooksConfig) *[]HookEntry { return &h.WorkspaceChanged },
	},
}

// HasGroup reports whether at least one hook is defined in group.
func (h *HooksConfig) HasGroup(group HookGroup) bool {
	for _, kind := range HookKinds {
		if kind.Group == group && len(*kind.Entries(h)) > 0 {
			return true
		}
	}

	return false
}
