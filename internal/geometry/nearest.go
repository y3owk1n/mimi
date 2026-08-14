package geometry

import "math"

// Direction is one of the four axes directional window focus navigates along.
type Direction uint8

// The directions directional focus navigates in. Up and Left move towards
// smaller coordinates, Down and Right towards larger ones.
const (
	Up Direction = iota //nolint:varnamelen // the directions read best as their plain names
	Down
	Left
	Right
)

// Nearest returns the index of the window nearest to frames[current] in the
// given direction, and whether one was found.
//
// A nil entry marks a frame that could not be read; it is never a candidate,
// and a nil at current reports not found. The returned index is an index into
// frames, so callers never have to map it back onto a filtered list.
func Nearest(frames []*Rect, current int, dir Direction) (int, bool) {
	if current < 0 || current >= len(frames) || frames[current] == nil {
		return 0, false
	}

	cur := *frames[current]
	best := candidate{index: -1}

	for index, frame := range frames {
		if index == current || frame == nil {
			continue
		}

		score, ok := scoreOf(cur, *frame, dir)
		if !ok {
			continue
		}

		next := candidate{index: index, rect: *frame, score: score}
		if best.index < 0 || next.beats(best) {
			best = next
		}
	}

	if best.index < 0 {
		return 0, false
	}

	return best.index, true
}

// candidate is one window scored against the one focus is navigating from.
type candidate struct {
	index int
	rect  Rect
	score float64
}

// beats reports whether c is a better target than other. Equal scores resolve
// to the topmost window, then the leftmost; a candidate that ties on all three
// loses, which leaves the lowest index holding the win.
func (c candidate) beats(other candidate) bool {
	if c.score != other.score {
		return c.score < other.score
	}

	if c.rect.Y != other.rect.Y {
		return c.rect.Y < other.rect.Y
	}

	return c.rect.X < other.rect.X
}

// scoreOf scores one candidate against the current window: the distance
// between their centers along the requested axis, plus the distance across it
// unless the two windows overlap on that perpendicular axis. Lower is nearer.
// It reports false when the candidate does not lie in the requested direction.
func scoreOf(cur, cand Rect, dir Direction) (float64, bool) {
	deltaX := cand.CenterX() - cur.CenterX()
	deltaY := cand.CenterY() - cur.CenterY()

	var (
		primary, secondary float64
		overlap            bool
	)

	switch dir {
	case Left:
		if deltaX >= 0 {
			return 0, false
		}

		primary, secondary = -deltaX, math.Abs(deltaY)
		overlap = cur.Y < cand.Bottom() && cur.Bottom() > cand.Y
	case Right:
		if deltaX <= 0 {
			return 0, false
		}

		primary, secondary = deltaX, math.Abs(deltaY)
		overlap = cur.Y < cand.Bottom() && cur.Bottom() > cand.Y
	case Up:
		if deltaY >= 0 {
			return 0, false
		}

		primary, secondary = -deltaY, math.Abs(deltaX)
		overlap = cur.X < cand.Right() && cur.Right() > cand.X
	case Down:
		if deltaY <= 0 {
			return 0, false
		}

		primary, secondary = deltaY, math.Abs(deltaX)
		overlap = cur.X < cand.Right() && cur.Right() > cand.X
	default:
		return 0, false
	}

	if overlap {
		return primary, true
	}

	return primary + secondary, true
}
