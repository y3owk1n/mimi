package daemon

import (
	"encoding/json"
	"strconv"

	"github.com/y3owk1n/mimi/internal/action"
	"github.com/y3owk1n/mimi/internal/events"
)

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
