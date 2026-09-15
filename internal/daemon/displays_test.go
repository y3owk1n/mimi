//nolint:testpackage // tests displayChangeEvent, an unexported function
package daemon

import (
	"errors"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/events"
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
