// Package internal/space CGo bridge header.
//
// Cherry-picked from neru: only the symbols required by
// action.FocusWindow, action.Space, and action.MoveWindowToSpace
// are exposed here. Symbol names use the Mimi* prefix; they have
// been renamed from the upstream Neru* identifiers.
#pragma once
#include <ApplicationServices/ApplicationServices.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdbool.h>
#include <stdint.h>

#pragma mark - Cocoa Bootstrap

/// Bring up NSApplication on the calling thread. Must be
/// called once, from the main OS thread, before any other
/// call in this header. Required because AppKit helpers
/// (NSRunningApplication, NSWorkspace) need a live
/// NSApplication. The function is safe to call from any
/// thread; the actual setup only runs the first time.
void MimiInitCocoaApp(void);

#pragma mark - Element Reference Lifecycle

/// Release an AXUIElementRef obtained from one of the window
/// enumeration helpers below. Safe to call on NULL.
void MimiReleaseElement(void *element);

#pragma mark - Window Enumeration (focus_window)

/// Return all focusable windows on the active space across every
/// running application. Filters out non-focusable windows
/// (minimized, hidden, off-space, non-AXWindow roles).
///
/// Each returned reference is retained; the caller must release
/// them with MimiReleaseElement and free the returned array
/// with the C library free(3).
///
/// @param count Output parameter that receives the window count.
/// @return NULL-terminated array of AXUIElementRef pointers, or NULL
///         on failure or when no focusable windows exist.
void **MimiGetAllFocusableWindowsOnActiveSpace(int *count);

/// Return the frontmost window across the system. The returned
/// reference is retained; the caller must release it with
/// MimiReleaseElement.
void *MimiGetFrontmostWindow(void);

/// Bring the window to the foreground and give it keyboard focus.
int MimiActivateWindow(void *window);

/// Check whether two window references refer to the same
/// underlying AXUIElement. Returns 1 on equal, 0 otherwise.
int MimiAreElementsEqual(void *element1, void *element2);

#pragma mark - Mission Control Detection (space / move_window_to_space)

/// Return true when Mission Control is currently on screen.
bool MimiIsMissionControlActive(void);

#pragma mark - Space Switching (space)

/// Total number of Mission Control spaces across all displays
/// in their current ordering.
int MimiCountMissionControlSpaces(void);

/// Space ID for a 1-based Mission Control index. Returns 0
/// when the index is out of range or unavailable.
uint64_t MimiMissionControlSpaceID(int index);

/// Display ID that owns the given space ID. Returns 0 when the
/// space is invalid.
uint32_t MimiSpaceDisplayID(uint64_t sid);

/// Focus a space using a synthetic high-velocity horizontal
/// dock swipe gesture.
///
/// The caller must have already verified Mission Control is
/// not active and that the destination differs from the current
/// one. When focus crosses displays, the cursor is warped to
/// the destination display first so the gesture is attributed
/// to the correct screen.
int MimiFocusSpaceUsingGesture(uint32_t new_did, uint64_t new_sid);

#pragma mark - Window-to-Space (move_window_to_space)

/// Move a window to a specific space ID via the dynamic
/// SkyLight class. On the primary path the move is dispatched
/// asynchronously; 1 means the operation was queued, not that
/// the window has already moved. The synchronous
/// SLSMoveWindowsToManagedSpace fallback provides a stronger
/// completion guarantee but is only used when the primary
/// path is unavailable.
int MimiMoveWindowToSpace(void *windowElement, uint64_t spaceID);
