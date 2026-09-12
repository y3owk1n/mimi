#ifndef MIMI_DROPZONE_H
#define MIMI_DROPZONE_H

#import "border.h"

/// How the drop zone is drawn: the fill, the outline and its width, and the
/// corner radius, all in points.
typedef struct {
	MimiColor fill;
	MimiColor outline;
	double width;
	double radius;
} MimiDropzoneStyle;

/// Show the drop zone over the given frame, in window coordinates (y down
/// from the top of the main display), or move it there when it is shown.
void MimiDropzoneShow(const MimiDropzoneStyle *style, double x, double y, double w, double h);
/// Take the drop zone off screen.
void MimiDropzoneHide(void);
/// Whether the left mouse button is down right now.
int MimiLeftMouseButtonDown(void);

#endif  // MIMI_DROPZONE_H
