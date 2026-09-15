package border_test

import (
	"context"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/border"
	"github.com/y3owk1n/mimi/internal/config"
	"github.com/y3owk1n/mimi/internal/events"
	"github.com/y3owk1n/mimi/internal/native"
)

// fakeDrawer records what the engine asked of the screen.
type fakeDrawer struct {
	styles []native.BorderStyle
	syncs  chan bool
	clears int
}

func newFakeDrawer() *fakeDrawer {
	return &fakeDrawer{syncs: make(chan bool, 16)}
}

func (f *fakeDrawer) drawer() border.Drawer {
	return border.Drawer{
		SetStyle: func(style native.BorderStyle) { f.styles = append(f.styles, style) },
		Sync:     func(refocus bool) { f.syncs <- refocus },
		Clear:    func() { f.clears++ },
	}
}

func enabledConfig() config.BorderConfig {
	return config.BorderConfig{
		Enabled:       true,
		Width:         4,
		ActiveColor:   "#ff0000",
		InactiveColor: "#80000000",
	}
}

func TestEngine_UpdateStylesEnablesAndClears(t *testing.T) {
	t.Parallel()

	fake := newFakeDrawer()
	engine := border.New(fake.drawer())

	if engine.KindFilter()(events.WindowFocus) {
		t.Fatal("a disabled engine admits window_focus, want nothing")
	}

	engine.Update(enabledConfig())

	if len(fake.styles) != 1 {
		t.Fatalf("SetStyle called %d times, want 1", len(fake.styles))
	}

	style := fake.styles[0]
	if style.Width != 4 || style.Active.Red != 1 || style.Inactive.Alpha != 128.0/255 {
		t.Errorf("style = %+v, want the config's width and colors", style)
	}

	if style.Radius != native.FollowWindowRadius {
		t.Errorf("style.Radius = %v, want FollowWindowRadius", style.Radius)
	}

	if style.Inside || style.HideWhenSingle {
		t.Errorf("style = %+v, want neither Inside nor HideWhenSingle", style)
	}

	if !engine.KindFilter()(events.WindowFocus) || engine.KindFilter()(events.WindowTitleChange) {
		t.Error("an enabled engine admits the waking kinds and nothing else")
	}

	// The same config again redraws nothing.
	engine.Update(enabledConfig())

	if len(fake.styles) != 1 {
		t.Errorf("SetStyle called %d times after an unchanged reload, want 1", len(fake.styles))
	}

	cfg := enabledConfig()
	cfg.Width = 8
	engine.Update(cfg)

	if len(fake.styles) != 2 {
		t.Errorf("SetStyle called %d times after a restyle, want 2", len(fake.styles))
	}

	cfg.Placement = config.BorderInside
	engine.Update(cfg)

	if len(fake.styles) != 3 || !fake.styles[2].Inside {
		t.Errorf(
			"styles after moving the border inside = %+v, want a third with Inside set",
			fake.styles,
		)
	}

	cfg.HideWhenSingle = true
	engine.Update(cfg)

	if len(fake.styles) != 4 || !fake.styles[3].HideWhenSingle {
		t.Errorf(
			"styles after hide_when_single = %+v, want a fourth with HideWhenSingle set",
			fake.styles,
		)
	}

	cfg.Enabled = false
	engine.Update(cfg)

	if fake.clears != 1 {
		t.Errorf("Clear called %d times after disabling, want 1", fake.clears)
	}
}

func TestEngine_RunSyncsOnEventsAndNudges(t *testing.T) {
	t.Parallel()

	fake := newFakeDrawer()
	engine := border.New(fake.drawer())
	engine.Update(enabledConfig())

	bus := events.NewBus()
	sub := bus.SubscribeWithFilter(4, engine.KindFilter())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	go func() {
		engine.Run(ctx, sub)
		close(done)
	}()

	bus.Publish(events.Event{Kind: events.WindowFocus})

	select {
	case refocus := <-fake.syncs:
		if !refocus {
			t.Error("a focus event synced without refocus, want refocus")
		}
	case <-time.After(time.Second):
		t.Fatal("no sync after a window_focus event")
	}

	engine.Nudge()

	select {
	case refocus := <-fake.syncs:
		if refocus {
			t.Error("a nudge synced with refocus, want the frames alone")
		}
	case <-time.After(time.Second):
		t.Fatal("no sync after a nudge")
	}

	cancel()
	<-done

	if fake.clears != 1 {
		t.Errorf("Clear called %d times after Run ended, want 1", fake.clears)
	}

	engine.Nudge()

	select {
	case <-fake.syncs:
		t.Error("a closed engine synced on a nudge")
	default:
	}
}

// TestEngine_RunSyncsAgainWhileANewWindowSettles pins that the engine syncs
// again after a window is created. The window server may not list the window
// yet, and a window no layout places has no later event to draw its border on.
func TestEngine_RunSyncsAgainWhileANewWindowSettles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind  events.EventKind
		after bool
	}{
		{kind: events.WindowCreated, after: true},
		{kind: events.AppActivate, after: true},
		{kind: events.WindowFocus, after: false},
	}

	for _, testCase := range tests {
		t.Run(string(testCase.kind), func(t *testing.T) {
			t.Parallel()

			fake := newFakeDrawer()
			engine := border.New(fake.drawer())
			engine.Update(enabledConfig())

			bus := events.NewBus()
			sub := bus.SubscribeWithFilter(4, engine.KindFilter())

			ctx := t.Context()

			go engine.Run(ctx, sub)

			bus.Publish(events.Event{Kind: testCase.kind})

			select {
			case <-fake.syncs:
			case <-time.After(time.Second):
				t.Fatalf("no sync after %s", testCase.kind)
			}

			select {
			case <-fake.syncs:
				if !testCase.after {
					t.Fatalf("synced again after %s, want the one sync", testCase.kind)
				}
			case <-time.After(2 * time.Second):
				if testCase.after {
					t.Fatalf("no second sync after %s", testCase.kind)
				}
			}
		})
	}
}
