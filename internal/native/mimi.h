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
void *MimiGetFrontmostWindow(void);
int MimiActivateWindow(void *window);

#pragma mark - Screen Functions

bool MimiIsMissionControlActive(void);

#pragma mark - Space Functions

int MimiCountMissionControlSpaces(void);
uint64_t MimiMissionControlSpaceID(int index);
uint32_t MimiSpaceDisplayID(uint64_t sid);
int MimiFocusSpaceUsingGesture(uint32_t new_did, uint64_t new_sid);
int MimiMoveWindowToSpace(void *windowElement, uint64_t spaceID);

#endif  // ACCESSIBILITY_H
