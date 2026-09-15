package native //nolint:testpackage // appearanceEvent is unexported

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/events"
)

func TestAppearanceEvent_CarriesTheModeNowInEffect(t *testing.T) {
	t.Parallel()

	dark := appearanceEvent(true)
	if dark.Kind != events.AppearanceChanged || dark.Extra["appearance"] != "dark" {
		t.Fatalf("got %+v", dark)
	}

	if light := appearanceEvent(false); light.Extra["appearance"] != "light" {
		t.Fatalf("got %+v", light)
	}
}
