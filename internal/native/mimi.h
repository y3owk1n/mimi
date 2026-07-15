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

#pragma mark - Screen Functions

bool MimiIsMissionControlActive(void);
double *MimiGetScreenFrameForPoint(double x, double y);
double *MimiGetScreenVisibleFrameForPoint(double x, double y);

#pragma mark - Window Frame Functions

double *MimiGetWindowFrame(void *window);
int MimiSetWindowFrame(void *window, double x, double y, double w, double h);

#pragma mark - Tiling Margins

bool MimiTiledWindowMarginsEnabled(void);
double MimiTiledWindowMarginSize(void);

#pragma mark - Space Functions

int MimiCountMissionControlSpaces(void);
uint64_t MimiMissionControlSpaceID(int index);
uint32_t MimiSpaceDisplayID(uint64_t sid);
uint64_t MimiActiveSpaceID(void);
int MimiFocusSpaceUsingGesture(uint32_t new_did, uint64_t new_sid);
int MimiMoveWindowToSpace(void *windowElement, uint64_t spaceID);
uint32_t MimiCursorDisplayID(void);
void MimiActivateDisplay(uint32_t did);

#endif  // ACCESSIBILITY_H
