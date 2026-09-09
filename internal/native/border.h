#ifndef MIMI_BORDER_H
#define MIMI_BORDER_H

#import <CoreGraphics/CoreGraphics.h>
#import <Foundation/Foundation.h>

/// A colour, each channel 0 to 1.
typedef struct {
	double red;
	double green;
	double blue;
	double alpha;
} MimiColor;

/// How borders are drawn: how wide, the radius of the window's corner the
/// border follows on its inside, or -1 to follow each window's own corner as
/// the window server reports it, and the colour for the focused window and
/// for every other.
typedef struct {
	double width;
	double radius;
	MimiColor active;
	MimiColor inactive;
} MimiBorderStyle;

/// Draw borders with the given style from now on, restyling the borders
/// already on screen. Borders are drawn for every real window on the spaces
/// in front, under the window they belong to, so nothing draws over another
/// application's content.
void MimiBordersSetStyle(const MimiBorderStyle *style);

/// Bring the borders up to date with the windows: add one for a window
/// without, move one whose window moved, drop one whose window is gone.
/// With refocus set, ask Accessibility which window is focused first; a
/// drag leaves focus where it was, so a caller reporting one leaves it
/// unset. Calls made before the last one has run are folded into it.
void MimiBordersSync(int refocus);

/// Take every border off the screen and draw none until the next
/// MimiBordersSetStyle.
void MimiBordersClear(void);

/// The window number of the border drawn under window number, or 0 when it
/// has none. Main thread only.
uint32_t MimiBorderWindowNumber(uint32_t number);

/// Describe the border drawn under window number, for drawing a copy of it
/// elsewhere: how wide, the corner radius it follows, and its colour. Returns
/// 0 when the window has none. Main thread only.
int MimiBorderRing(uint32_t number, double *width, double *radius, MimiColor *color);

/// A ring path for a window of size, in a rect grown by width on every side
/// with its origin at zero: the outside rounded by radius plus width, the
/// window's own rect cut out rounded by radius. The caller releases it.
CGPathRef MimiBorderRingPath(CGSize size, double width, double radius);

#endif  // MIMI_BORDER_H
