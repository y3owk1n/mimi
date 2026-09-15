//nolint:testpackage // tests displayChangeEvent, an unexported function
package daemon

import (
	"errors"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/native"
)

var errNoDisplays = errors.New("no displays")

func TestDisplayChangeEvent_CarriesTheDisplayList(t *testing.T) {
	list := func() ([]action.DisplayEntry, error) {
		return []action.DisplayEntry{
			{Index: 1, ID: 7, Frame: action.Frame{Width: 1920, Height: 1080}},
			{Index: 2, ID: 9, Frame: action.Frame{Y: 1080, Width: 1440, Height: 900}},
		}, nil
	}

	evt := displayChangeEvent(list)(events.Event{Kind: events.DisplayChanged})

	if evt.Extra["displays_count"] != "2" {
		t.Fatalf("displays_count = %q", evt.Extra["displays_count"])
	}

	want := `[{"index":1,"id":7,"frame":{"x":0,"y":0,"width":1920,"height":1080},` +
		`"visible":{"x":0,"y":0,"width":0,"height":0}},` +
		`{"index":2,"id":9,"frame":{"x":0,"y":1080,"width":1440,"height":900},` +
		`"visible":{"x":0,"y":0,"width":0,"height":0}}]`
	if evt.Extra["displays"] != want {
		t.Fatalf("displays = %s", evt.Extra["displays"])
	}
}

func TestDisplayChangeEvent_SaysNothingWhenTheListFails(t *testing.T) {
	list := func() ([]action.DisplayEntry, error) { return nil, errNoDisplays }

	evt := displayChangeEvent(list)(events.Event{Kind: events.DisplayChanged})
	if evt.Extra != nil {
		t.Fatalf("got %+v", evt.Extra)
	}
}

func TestDisplayIndexEvent_NamesTheDisplayTheEventHappenedOn(t *testing.T) {
	list := func() ([]action.DisplayEntry, error) {
		return []action.DisplayEntry{
			{Index: 1, ID: 7, Frame: action.Frame{Width: 1920, Height: 1080}},
			{Index: 2, ID: 9, Frame: action.Frame{X: 1920, Width: 1440, Height: 900}},
		}, nil
	}
	enrich := displayIndexEvent(list)

	onSecond := enrich(
		events.Event{
			Kind:  events.WindowFocus,
			Extra: map[string]string{native.WindowCenterKey: "2500,300"},
		},
	)
	if onSecond.Extra[DisplayIndexKey] != "2" || onSecond.Extra[native.WindowCenterKey] != "" {
		t.Fatalf("a window centered on the second display gave %+v", onSecond.Extra)
	}

	// A center off every display, as a window parked off the right edge,
	// belongs to the nearest one.
	parked := enrich(
		events.Event{
			Kind:  events.WindowFocus,
			Extra: map[string]string{native.WindowCenterKey: "5000,300"},
		},
	)
	if parked.Extra[DisplayIndexKey] != "2" {
		t.Fatalf("a window parked off screen gave %+v", parked.Extra)
	}

	space := enrich(
		events.Event{
			Kind:  events.WorkspaceChanged,
			Extra: map[string]string{native.DisplayIDKey: "9"},
		},
	)
	if space.Extra[DisplayIndexKey] != "2" || space.Extra[native.DisplayIDKey] != "" {
		t.Fatalf("a space change on display 9 gave %+v", space.Extra)
	}

	unknown := enrich(
		events.Event{
			Kind:  events.WorkspaceChanged,
			Extra: map[string]string{native.DisplayIDKey: "42"},
		},
	)
	if _, ok := unknown.Extra[DisplayIndexKey]; ok || unknown.Extra[native.DisplayIDKey] != "" {
		t.Fatalf("an unknown display gave %+v", unknown.Extra)
	}

	if plain := enrich(events.Event{Kind: events.WindowFocus}); plain.Extra != nil {
		t.Fatalf("an event naming no display gained %+v", plain.Extra)
	}
}
