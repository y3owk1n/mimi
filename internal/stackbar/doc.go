// Package stackbar draws the windows a layout stacked as a deck of cards, so
// that a place holding several of them does not look like one holding one.
//
// A stack is the layout's idea, not mimi's, and mimi changes no z-order to
// arrange it. macOS offers no way to raise one application's window above
// another's without also focusing it, so the window in front is whichever has
// focus. What is left invisible is that the others are there at all.
//
// The cards are drawn inside the frame the layout set aside, never around it.
// Reserve says how much of that frame they need, the engine takes it out of
// the window in front, and Show draws them in what is left, above the window
// for the members before it in the stack and below for the ones after. Where
// the window in front sits between them is where it sits in the stack.
package stackbar
