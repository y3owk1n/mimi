package baseline

import (
	_ "embed"
	"encoding/json"
	"fmt"

	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// FileName is the name of the recording file inside this package's directory.
// The integration recorder writes it; Load reads the copy embedded at build time.
const FileName = "window_baseline.json"

//go:embed window_baseline.json
var recordingJSON []byte

// Rect is a window or screen rectangle in points. Resize inputs and outputs use
// the Accessibility coordinate system (y-down, origin at the primary display's
// top-left); Display.Visible uses the NSScreen one (y-up). The field set matches
// the geometry rectangle the action layer works in, so the two convert directly.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// String renders the rectangle as "x,y wxh" for assertion messages.
func (r Rect) String() string {
	return fmt.Sprintf("%g,%g %gx%g", r.X, r.Y, r.W, r.H)
}

// Geometry returns the rectangle as the geometry module's, which holds the
// same four fields.
//
// The recording keeps a type of its own so that the names the frames are
// stored under stay a property of this file format rather than of the pure
// module, and so that re-recording is never needed to change one. This is the
// single place the two are converted.
func (r Rect) Geometry() geometry.Rect {
	return geometry.Rect(r)
}

// Display is everything the action layer had to ask macOS about the screen when
// the recording was made. A recording is only replayable against a display that
// reports the same values, which is why the whole set is stored.
type Display struct {
	// Visible is the screen's visible frame in NSScreen y-up coordinates.
	Visible Rect `json:"visible"`
	// PrimaryHeight is the primary display's height, the constant relating the
	// y-up and y-down coordinate systems.
	PrimaryHeight float64 `json:"primaryHeight"`
	// MarginsEnabled is the system tiled-window-margins setting.
	MarginsEnabled bool `json:"marginsEnabled"`
	// MarginSize is the system tiled-window margin, in points.
	MarginSize float64 `json:"marginSize"`
}

// Screen returns the display as the geometry module's screen — the same set of
// values, which is what the recording stores it for.
func (d Display) Screen() geometry.Screen {
	return geometry.Screen{
		Visible:        d.Visible.Geometry(),
		PrimaryHeight:  d.PrimaryHeight,
		MarginsEnabled: d.MarginsEnabled,
		MarginSize:     d.MarginSize,
	}
}

// ResizeCase is one recorded resize_window invocation: the flags, the frame the
// window started from, and the frame macOS ended up with.
//
// Args holds the command line as a user would type it, which is what the
// recorder ran. A test of the geometry alone has to put those flags through the
// action layer's argument parsing first — the recording deliberately does not
// store a parsed form, because the shape of that form is not settled.
type ResizeCase struct {
	Name  string   `json:"name"`
	Args  []string `json:"args"`
	Start Rect     `json:"start"`
	Want  Rect     `json:"want"`
}

// FocusCase is one recorded focus_window invocation. Arrangement holds the
// frames of the recorder's own windows in the order the action layer enumerates
// them; Current and Want are indices into it.
//
// The live run scored these against every window on the space, not just these.
// The recorder only runs a case once it has established that no other window
// can score better than its own, so dropping them cannot change which window
// wins, and replaying Arrangement on its own reproduces Want.
type FocusCase struct {
	Direction   string `json:"direction"`
	Arrangement []Rect `json:"arrangement"`
	Current     int    `json:"current"`
	Want        int    `json:"want"`
}

// Recording is the whole baseline: the display it was captured on, plus every
// recorded case.
type Recording struct {
	Display Display      `json:"display"`
	Resize  []ResizeCase `json:"resize"`
	Focus   []FocusCase  `json:"focus"`
}

// ResizeCase returns the recorded resize case with the given name.
func (r Recording) ResizeCase(name string) (ResizeCase, bool) {
	for _, rec := range r.Resize {
		if rec.Name == name {
			return rec, true
		}
	}

	return ResizeCase{}, false
}

// FocusCase returns the recorded focus case for the given direction.
func (r Recording) FocusCase(direction string) (FocusCase, bool) {
	for _, rec := range r.Focus {
		if rec.Direction == direction {
			return rec, true
		}
	}

	return FocusCase{}, false
}

// Load returns the recording embedded at build time.
func Load() (Recording, error) {
	var rec Recording

	err := json.Unmarshal(recordingJSON, &rec)
	if err != nil {
		return Recording{}, derrors.Wrapf(
			err,
			derrors.CodeSerializationFailed,
			"failed to decode the embedded window baseline",
		)
	}

	return rec, nil
}

// Encode renders a recording as the indented JSON the recording file stores.
func Encode(rec Recording) ([]byte, error) {
	data, err := json.MarshalIndent(rec, "", "\t")
	if err != nil {
		return nil, derrors.Wrapf(
			err,
			derrors.CodeSerializationFailed,
			"failed to encode the window baseline",
		)
	}

	return append(data, '\n'), nil
}
