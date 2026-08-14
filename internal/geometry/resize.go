package geometry

// percentageWhole is the denominator a percentage dimension is taken over.
const percentageWhole = 100.0

// bothSides splits a length between the two sides that share it: the leftover
// space either side of a centered window, and the margin between two tiled
// windows.
const bothSides = 2.0

// Screen is everything the geometry has to know about the display a window is
// being placed on — the set the action layer reads from macOS before it can
// compute anything.
type Screen struct {
	// Visible is the screen's visible frame — the full frame less the menu bar
	// and the Dock — in NSScreen coordinates: y-up, with the origin at the
	// primary display's bottom-left, exactly as macOS reports it.
	Visible Rect
	// PrimaryHeight is the primary display's height. It is the constant that
	// relates the y-up screen coordinates to the y-down window ones, and Resize
	// applies the conversion itself so that no caller can forget to.
	PrimaryHeight float64
	// MarginsEnabled is the system tiled-window-margins setting, used when the
	// request expresses no preference of its own.
	MarginsEnabled bool
	// MarginSize is the system tiled-window margin, in points.
	MarginSize float64
}

// Request is what the user asked for. Every field is optional; the zero
// Request keeps the window's current size and centers it.
type Request struct {
	// Preset is a named tiling shortcut, or "" for none. An unknown name is
	// ignored — IsPreset is what turns one into an error, at parse time.
	Preset string
	// Width and Height are the size asked for: a length in points, a share of
	// the visible frame, or the window's current size.
	Width  Dimension
	Height Dimension
	// X and Y place the window's origin explicitly, in window coordinates
	// (y-down). A nil coordinate is derived from the anchor instead.
	X *float64
	Y *float64
	// Anchor is where inside the visible frame the window is placed. A nil
	// anchor centers the window, unless a preset supplies one.
	Anchor *Anchor
	// UseMargins overrides the system tiled-window-margins setting. A nil
	// preference follows the system.
	UseMargins *bool
}

// dimensionKind is how a requested dimension is expressed.
type dimensionKind uint8

const (
	// keepKind holds the window's current size, and is the zero dimension.
	keepKind dimensionKind = iota
	// absoluteKind is a length in points.
	absoluteKind
	// percentKind is a share of the screen's visible frame.
	percentKind
)

// Dimension is one requested window dimension: a length in points, a share of
// the visible frame, or the current size kept as it is. The zero Dimension
// keeps the current size.
//
// The three are ordered rather than combined: a dimension is exactly one of
// them, so nothing has to fall through from one to the next at placement time.
type Dimension struct {
	kind  dimensionKind
	value float64
}

// Keep leaves the window's current size along this axis untouched.
//
// The CLI maps a zero --width or --width-percent onto it, which is how a zero
// stays inert rather than collapsing the window.
func Keep() Dimension {
	return Dimension{}
}

// Absolute is a size in points.
func Absolute(points float64) Dimension {
	return Dimension{kind: absoluteKind, value: points}
}

// Percent is a size given as a share of the screen's visible frame, from 0
// to 100.
func Percent(share float64) Dimension {
	return Dimension{kind: percentKind, value: share}
}

// resolve turns the requested dimension into a length, given the window's
// current length along that axis and the visible frame's.
func (d Dimension) resolve(current, available float64) float64 {
	switch d.kind {
	case absoluteKind:
		return d.value
	case percentKind:
		return available * d.value / percentageWhole
	case keepKind:
		return current
	default:
		return current
	}
}

// Resize returns the frame a window at cur should be moved to on scr to
// satisfy req.
//
// It is total: every input produces a frame. An unknown preset name is
// ignored, and nothing here reports an error.
//
// cur and the returned frame are in window coordinates — y-down, origin at the
// primary display's top-left — while scr.Visible is in screen coordinates.
// Resize converts between the two.
func Resize(cur Rect, scr Screen, req Request) Rect {
	req = expandPreset(req)

	width := req.Width.resolve(cur.W, scr.Visible.W)
	height := req.Height.resolve(cur.H, scr.Visible.H)

	// The visible frame in window coordinates. Its top edge is the one line
	// that converts between the two systems:
	//
	//	y-down top = primaryHeight - visibleFrameY - visibleFrameHeight
	//
	// Everything below this point is in window coordinates.
	bounds := Rect{
		X: scr.Visible.X,
		Y: scr.PrimaryHeight - scr.Visible.Y - scr.Visible.H,
		W: scr.Visible.W,
		H: scr.Visible.H,
	}

	anchor := Center
	if req.Anchor != nil {
		anchor = *req.Anchor
	}

	posX := anchor.originX(bounds, width)
	if req.X != nil {
		posX = *req.X
	}

	posY := anchor.originY(bounds, height)
	if req.Y != nil {
		posY = *req.Y
	}

	frame := Rect{X: posX, Y: posY, W: width, H: height}

	useMargins := scr.MarginsEnabled
	if req.UseMargins != nil {
		useMargins = *req.UseMargins
	}

	if useMargins {
		frame = inset(frame, bounds, scr.MarginSize)
	}

	return frame
}

// inset applies the tiled-window margin to a frame: a full margin on each edge
// that abuts the visible frame's boundary, half a margin on each internal one,
// so that the gap between two tiled windows is one margin rather than two.
//
// Two known defects live here, both preserved from the code this replaced. An
// edge abuts only on exact float equality, so a window a fraction of a point
// off the boundary is treated as internal (issue #66); and nothing stops the
// margins from driving a width or height to zero or below (issue #65).
func inset(frame, bounds Rect, size float64) Rect {
	left := marginOn(frame.X == bounds.X, size)
	right := marginOn(frame.Right() == bounds.Right(), size)
	top := marginOn(frame.Y == bounds.Y, size)
	bottom := marginOn(frame.Bottom() == bounds.Bottom(), size)

	frame.X += left
	frame.W -= left + right
	frame.Y += top
	frame.H -= top + bottom

	return frame
}

// marginOn returns the margin one edge takes: the whole of it when the edge
// abuts the visible frame's boundary, half of it when the edge is internal and
// the gap is shared with whatever is tiled alongside.
func marginOn(abuts bool, size float64) float64 {
	if abuts {
		return size
	}

	return size / bothSides
}

// expandPreset fills in what the request's named preset asks for: its
// percentages wherever the request keeps the current size, and its anchor. An
// unknown name — including the empty one — leaves the request untouched.
//
// The preset's anchor is applied unconditionally, so a preset silently
// overrides an anchor the user gave explicitly. That is what the CLI does
// today; see issue #64.
func expandPreset(req Request) Request {
	named, ok := presets[req.Preset]
	if !ok {
		return req
	}

	anchor := named.anchor
	req.Anchor = &anchor

	if req.Width.kind == keepKind {
		req.Width = Percent(named.widthPercent)
	}

	if req.Height.kind == keepKind {
		req.Height = Percent(named.heightPercent)
	}

	return req
}

// Anchor is one of the nine positions a window can be placed at inside the
// screen's visible frame.
type Anchor uint8

// The nine anchors, named vertical-first to match the two-letter form the CLI
// takes ("tl", "cc", "br").
const (
	TopLeft Anchor = iota
	TopCenter
	TopRight
	CenterLeft
	Center
	CenterRight
	BottomLeft
	BottomCenter
	BottomRight
)

// anchorNames maps each anchor onto the two-letter name the CLI uses.
var anchorNames = map[Anchor]string{
	TopLeft:      "tl",
	TopCenter:    "tc",
	TopRight:     "tr",
	CenterLeft:   "cl",
	Center:       "cc",
	CenterRight:  "cr",
	BottomLeft:   "bl",
	BottomCenter: "bc",
	BottomRight:  "br",
}

// String returns the two-letter name of the anchor.
func (a Anchor) String() string {
	name, ok := anchorNames[a]
	if !ok {
		return "unknown"
	}

	return name
}

// ParseAnchor maps a two-letter anchor name onto its anchor, and reports
// whether the name is one of the nine.
//
// An unknown name reports false alongside the zero anchor, which is itself a
// valid one — following Nearest, callers have to read the boolean.
func ParseAnchor(name string) (Anchor, bool) {
	for anchor, known := range anchorNames {
		if known == name {
			return anchor, true
		}
	}

	return 0, false
}

// originX returns the x the anchor places a window of the given width at
// inside bounds, the visible frame in window coordinates.
func (a Anchor) originX(bounds Rect, width float64) float64 {
	switch a {
	case TopLeft, CenterLeft, BottomLeft:
		return bounds.X
	case TopCenter, Center, BottomCenter:
		return bounds.X + (bounds.W-width)/bothSides
	case TopRight, CenterRight, BottomRight:
		return bounds.X + bounds.W - width
	default:
		return bounds.X
	}
}

// originY returns the y the anchor places a window of the given height at
// inside bounds, the visible frame in window coordinates — so bounds.Y is its
// upper edge.
func (a Anchor) originY(bounds Rect, height float64) float64 {
	switch a {
	case TopLeft, TopCenter, TopRight:
		return bounds.Y
	case CenterLeft, Center, CenterRight:
		return bounds.Y + (bounds.H-height)/bothSides
	case BottomLeft, BottomCenter, BottomRight:
		return bounds.Y + bounds.H - height
	default:
		return bounds.Y
	}
}

// preset is the size and placement a named preset asks for.
type preset struct {
	widthPercent  float64
	heightPercent float64
	anchor        Anchor
}

// The shares of the visible frame the presets are defined in.
const (
	wholeScreen  = percentageWhole
	halfScreen   = 50.0
	centerWidth  = 60.0
	centerHeight = 80.0
)

// presets are the named tiling shortcuts resize_window accepts as its
// positional argument.
var presets = map[string]preset{
	"left-half":    {widthPercent: halfScreen, heightPercent: wholeScreen, anchor: TopLeft},
	"right-half":   {widthPercent: halfScreen, heightPercent: wholeScreen, anchor: TopRight},
	"top-half":     {widthPercent: wholeScreen, heightPercent: halfScreen, anchor: TopLeft},
	"bottom-half":  {widthPercent: wholeScreen, heightPercent: halfScreen, anchor: BottomLeft},
	"top-left":     {widthPercent: halfScreen, heightPercent: halfScreen, anchor: TopLeft},
	"top-right":    {widthPercent: halfScreen, heightPercent: halfScreen, anchor: TopRight},
	"bottom-left":  {widthPercent: halfScreen, heightPercent: halfScreen, anchor: BottomLeft},
	"bottom-right": {widthPercent: halfScreen, heightPercent: halfScreen, anchor: BottomRight},
	"center":       {widthPercent: centerWidth, heightPercent: centerHeight, anchor: Center},
	"fill":         {widthPercent: wholeScreen, heightPercent: wholeScreen, anchor: TopLeft},
}

// IsPreset reports whether name is one of the named resize presets.
//
// Resize ignores a name it does not know, so callers that want an unknown
// preset to be an error — the CLI does — check it with this first.
func IsPreset(name string) bool {
	_, ok := presets[name]

	return ok
}
