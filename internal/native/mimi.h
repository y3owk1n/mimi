#ifndef ACCESSIBILITY_H
#define ACCESSIBILITY_H

#import <ApplicationServices/ApplicationServices.h>
#import <Foundation/Foundation.h>

#pragma mark - Element Functions

void *MimiGetFocusedApplication(void);
void MimiReleaseElement(void *element);
void MimiRetainElement(void *element);
int MimiAreElementsEqual(void *element1, void *element2);

#pragma mark - Window Functions

void **MimiGetAllFocusableWindowsOnActiveSpace(int *count);
void **MimiGetAllFocusableWindowsOnActiveSpaceWithFocused(int *count, int *focusedIndex);
void *MimiGetFrontmostWindow(void);
int MimiActivateWindow(void *window);
/// Return the process identifier of the application owning the window, or 0.
int MimiGetWindowPID(void *window);
/// Return the window server's number for the window, or 0 when it has none.
uint32_t MimiGetWindowNumber(void *window);
/// Copy the window's title as a UTF-8 string the caller frees, or NULL when
/// the window has none or it cannot be read.
char *MimiCopyWindowTitle(void *window);

#pragma mark - Application Functions

/// One window of an application as the window server lists it.
typedef struct {
	/// The window server's number for the window.
	uint32_t number;
	/// The space the window is on, or 0 when it is on every space or none.
	uint64_t space;
} MimiAppWindow;

/// Resolve a running application by bundle identifier or localized name
/// (case-insensitive). Returns its pid, or 0 when nothing running matches.
int MimiFindApplication(const char *query);
/// Copy the pids of every running application with the regular activation
/// policy that owns a window, in no particular order. Sets *count; the
/// caller frees the array. NULL when there are none.
int *MimiCopyRegularApplicationPIDs(int *count);
/// Copy a running application's localized name as a UTF-8 string the caller
/// frees, or NULL when no application has that pid.
char *MimiCopyApplicationName(int pid);
/// Copy a running application's bundle identifier as a UTF-8 string the
/// caller frees, or NULL when no application has that pid or it has none.
char *MimiCopyApplicationBundleID(int pid);
/// Copy the numbers of every real, unminimized window on the given spaces
/// (an array of space ids), whoever owns them, as an array of NSNumber in
/// the window server's order. With radii given, it is set to a parallel
/// array of each window's corner radius in points, or -1 where the window
/// server does not say (before macOS 26). The caller releases both.
CFArrayRef MimiCopyRealWindowsOnSpaces(CFArrayRef spaceIDs, CFArrayRef *radii);
/// Fill radii with the corner radius in points of each window in numbers,
/// or -1 where the window server does not say (before macOS 26) or the
/// window is gone.
void MimiWindowCornerRadii(const uint32_t *numbers, int count, double *radii);
/// Turn an application's AXEnhancedUserInterface on or off, under which some
/// applications animate every move they are given. Returns whether it was
/// on, or -1 when the application has no such setting.
int MimiSetEnhancedUserInterface(int pid, int enabled);
/// Copy an application's real, unminimized windows on every space, front to
/// back. Sets *count; the caller frees the array. Auxiliary windows (popovers,
/// sheets, tab previews) are left out.
MimiAppWindow *MimiCopyApplicationWindows(int pid, int *count);
/// Bring one of an application's windows to the front by number. The window
/// has to be on the active space, which is when Accessibility lists it.
int MimiRaiseWindowNumber(int pid, uint32_t number);
/// Reopen an application as a Dock click would. An application with no window
/// opens one and comes to the front.
int MimiReopenApplication(int pid);

#pragma mark - Screen Functions

/// Doubles per display in MimiCopyScreenFrames' result.
#define MIMI_SCREEN_DOUBLES 9

bool MimiIsMissionControlActive(void);
double *MimiGetScreenFrameForPoint(double x, double y);
double *MimiGetScreenVisibleFrameForPoint(double x, double y);
/// Copy every connected display as nine doubles each: the CGDirectDisplayID,
/// then the frame and the visible frame as x, y, w, h in NSScreen coordinates.
/// Sets *count; the caller frees the result. NULL when there are none.
double *MimiCopyScreenFrames(int *count);

#pragma mark - Window Frame Functions

double *MimiGetWindowFrame(void *window);
int MimiSetWindowFrame(void *window, double x, double y, double w, double h);
int MimiSetWindowPosition(void *window, double x, double y);
/// Log what a stepped animation cost: how many windows and frames, over how
/// long, and the slowest single write, all in milliseconds.
void MimiLogAnimationSteps(int windows, int frames, double elapsedMS, double slowestMS);

/// Doubles per window in MimiCopyWindowList's result: number, x, y, width,
/// height, whether the window server named it, owner pid, layer, and whether
/// the owner is a regular, visible application.
#define MIMI_WINDOW_DOUBLES 9

/// Every window as the window server lists it, front to back, on screen only
/// or on every space, with its name in *names when Screen Recording is
/// granted. The caller frees the rows and each name.
double *MimiCopyWindowList(int onScreenOnly, int *count, char ***names);

/// An application's windows as Accessibility lists them, each with its
/// window server number and whether its role is a window, for the ones that
/// have a number. The caller releases each element and frees the arrays.
void **MimiCopyApplicationWindowElements(int pid, int *count, unsigned int **numbers, int **windows);

#pragma mark - Tiling Margins

bool MimiTiledWindowMarginsEnabled(void);
double MimiTiledWindowMarginSize(void);

#pragma mark - Space Functions

int MimiCountMissionControlSpaces(void);
uint64_t MimiMissionControlSpaceID(int index);
uint32_t MimiSpaceDisplayID(uint64_t sid);
uint64_t MimiActiveSpaceID(void);
/// The space ID in front on the given display, or 0.
uint64_t MimiDisplayActiveSpaceID(uint32_t did);
int MimiDisplaySpaceIsFullScreen(uint32_t did);
int MimiFocusSpaceUsingGesture(uint32_t new_did, uint64_t new_sid);
int MimiMoveWindowToSpace(void *windowElement, uint64_t spaceID);
uint32_t MimiCursorDisplayID(void);
void MimiActivateDisplay(uint32_t did);

#endif  // ACCESSIBILITY_H
