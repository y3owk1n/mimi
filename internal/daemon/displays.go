package daemon

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/native"
)

// DisplayIndexKey is the extra a window event and a space change carry
// naming the display they happened on, as move_window_to_display counts
// them, which is what a hook's display filter and mimi_DISPLAY_INDEX read.
const DisplayIndexKey = "display_index"

// displayIndexEvent replaces what the native layer put on an event to name
// its display, a window center or a display identifier, with the index a
// user counts. A window's display is the one holding its center, or the
// nearest when the center is off every display, as mimi query windows
// reports it. An event naming no display, or one the list does not hold,
// is left as it is with the native extra dropped, so a hook never sees a
// number that means nothing to it.
func displayIndexEvent(list func() ([]action.DisplayEntry, error)) func(events.Event) events.Event {
	return func(evt events.Event) events.Event {
		center, byCenter := evt.Extra[native.WindowCenterKey]
		wanted, byID := evt.Extra[native.DisplayIDKey]

		if !byCenter && !byID {
			return evt
		}

		delete(evt.Extra, native.WindowCenterKey)
		delete(evt.Extra, native.DisplayIDKey)

		displays, err := list()
		if err != nil {
			return evt
		}

		if byCenter {
			x, y, ok := parseCenter(center)
			if !ok {
				return evt
			}

			if display, found := action.DisplayOf(action.Frame{X: x, Y: y}, displays); found {
				evt.Extra[DisplayIndexKey] = strconv.Itoa(display.Index)
			}

			return evt
		}

		for _, display := range displays {
			if strconv.FormatUint(uint64(display.ID), 10) == wanted {
				evt.Extra[DisplayIndexKey] = strconv.Itoa(display.Index)

				break
			}
		}

		return evt
	}
}

// parseCenter reads the "x,y" a window event carries.
func parseCenter(center string) (float64, float64, bool) {
	xText, yText, found := strings.Cut(center, ",")
	if !found {
		return 0, 0, false
	}

	x, errX := strconv.ParseFloat(xText, 64)
	y, errY := strconv.ParseFloat(yText, 64)

	return x, y, errX == nil && errY == nil
}

// displayChangeEvent adds the connected displays to a display_changed
// event, as mimi_DISPLAYS_COUNT and mimi_DISPLAYS, the latter the JSON list
// mimi query displays prints. A hook then tells a dock from an undock
// without a query of its own. list is the read behind both, passed in so a
// test pins the shape without a desktop under it. When it fails the event
// says nothing about the displays rather than naming wrong ones.
func displayChangeEvent(
	list func() ([]action.DisplayEntry, error),
) func(events.Event) events.Event {
	return func(evt events.Event) events.Event {
		displays, err := list()
		if err != nil {
			return evt
		}

		data, err := json.Marshal(displays)
		if err != nil {
			return evt
		}

		if evt.Extra == nil {
			evt.Extra = map[string]string{}
		}

		evt.Extra["displays_count"] = strconv.Itoa(len(displays))
		evt.Extra["displays"] = string(data)

		return evt
	}
}
