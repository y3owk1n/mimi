#pragma once

/// Start reporting the pointer's position to Go through goMouseMoved, in
/// window coordinates, on every move. Returns 1 when the tap is in place, 0
/// when the window server refused it, which it does without Accessibility.
int MimiMouseMonitorStart(void);
/// Stop reporting.
void MimiMouseMonitorStop(void);
