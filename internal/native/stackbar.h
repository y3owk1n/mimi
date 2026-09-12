#ifndef MIMI_STACKBAR_H
#define MIMI_STACKBAR_H

#import "border.h"

#include <stdint.h>

/// How the windows behind the one in front are drawn: how much of each shows
/// above the one in front of it, how much narrower each is on either side,
/// the corner radius to follow when the window's own cannot be read, and the
/// colours of the nearest card and of the furthest. Sizes are in points.
typedef struct {
	double step;
	double taper;
	double radius;
	MimiColor color;
	MimiColor farColor;
} MimiStackbarStyle;

/// One stack to show: the whole frame the layout set aside for it, in window
/// coordinates (y down from the top of the main display), the window in
/// front, and how many windows are in that place altogether. The window in
/// front has already been given the lower part of the frame, and the cards
/// are drawn in what is left above it.
typedef struct {
	double x;
	double y;
	double width;
	double height;
	uint32_t front;
	int count;
	/// Where the window in front sits among them, counting from 0. The
	/// windows before it are drawn above and the ones after it below, so
	/// the deck shows which one of them is being looked at and which way
	/// the others lie.
	int active;
} MimiStackbar;

/// How many cards are drawn above and below the window in front, for a stack
/// of this many windows with that one active, in a frame of this size.
///
/// Fewer than the stack holds when the frame cannot spare the room. The deck
/// takes at most a quarter of the height in steps and a quarter of the width
/// in taper, and the two sides share what that allows.
void MimiStackbarCards(
    int windows, int active, double width, double height, double step, double taper, int *above, int *below);

/// Show exactly these stacks and no others, replacing whatever was shown
/// before. Passing none takes every one off screen.
void MimiStackbarsSync(const MimiStackbar *bars, int count, const MimiStackbarStyle *style);

/// Take every stack off screen and release the windows behind them.
void MimiStackbarsClear(void);

#endif  // MIMI_STACKBAR_H
