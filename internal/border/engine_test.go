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
		InactiveColor: "#00000080",
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
