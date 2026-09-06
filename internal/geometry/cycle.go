package geometry

// cycles are the presets that step through a sequence when asked to, keyed by
// the preset the sequence starts from, which is the name the user binds. The
// halves step to their two-thirds and then their third, the order Rectangle
// made familiar, and wrap back to the half.
var cycles = map[string][]string{
	leftHalfName:  {leftHalfName, leftTwoThirdsName, leftThirdName},
	rightHalfName: {rightHalfName, rightTwoThirdsName, rightThirdName},
}

// Cycles reports whether the preset has a cycle for Request.Cycle to step
// through.
func (p Preset) Cycles() bool {
	_, ok := cycles[p.name]

	return ok
}

// CyclingPresetNames returns the names of the presets that cycle, in the
// order PresetNames lists them. It is what a caller rejecting --cycle on any
// other preset tells the user about.
func CyclingPresetNames() []string {
	var names []string

	for _, named := range presets {
		if _, ok := cycles[named.name]; ok {
			names = append(names, named.name)
		}
	}

	return names
}

// nextInCycle picks the preset a cycling request applies. It places the
// window at each frame of the cycle in turn, on the same terms as the request
// (margins included), and steps to the one after the first frame the window
// is already at; a window at none of them starts the cycle over. A preset
// with no cycle comes back unchanged.
func nextInCycle(cur Rect, scr Screen, req Request) Preset {
	names, ok := cycles[req.Preset.name]
	if !ok {
		return req.Preset
	}

	step := req
	step.Cycle = false

	for index, name := range names {
		step.Preset = Preset{name: name}
		if SameFrame(cur, Resize(cur, scr, step)) {
			return Preset{name: names[(index+1)%len(names)]}
		}
	}

	return Preset{name: names[0]}
}
