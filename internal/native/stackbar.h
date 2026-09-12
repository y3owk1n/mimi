#ifndef MIMI_STACKBAR_H
#define MIMI_STACKBAR_H

#import "border.h"

#include <stdint.h>

/// How a stack indicator is drawn: the color of a member that is not the
/// active one, the color of the active one, the height of the bar and the
/// corner radius of each segment, in points.
typedef struct {
	MimiColor color;
	MimiColor activeColor;
	double height;
	double radius;
} MimiStackbarStyle;

/// One stack to mark: the frame its windows share, in window coordinates (y
/// down from the top of the main display), how many windows are in it, and
/// which of them, counting from 0, the layout means to be seen.
typedef struct {
	double x;
	double y;
	double width;
	double height;
	int count;
	int active;
} MimiStackbar;

/// Draw exactly these stacks and no others, replacing whatever was drawn
/// before. Passing none takes every indicator off screen.
void MimiStackbarsSync(const MimiStackbar *bars, int count, const MimiStackbarStyle *style);

/// Take every indicator off screen and release the windows behind them.
void MimiStackbarsClear(void);

#endif  // MIMI_STACKBAR_H
