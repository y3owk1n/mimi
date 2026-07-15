//
//  space.m
//  Mimi
//
//  Copyright © 2025 Mimi. All rights reserved.
//

#import "constants.h"
#import "mimi.h"
#import "mimi_log.h"

#import <ApplicationServices/ApplicationServices.h>
#import <Cocoa/Cocoa.h>
#import <CoreFoundation/CoreFoundation.h>
#import <Foundation/Foundation.h>
#import <dispatch/dispatch.h>
#import <mach-o/dyld.h>
#import <mach-o/loader.h>
#import <mach-o/nlist.h>
#import <objc/message.h>
#import <objc/objc.h>
#import <objc/runtime.h>

#pragma mark - SkyLight External Declarations

// Private SkyLight / WindowServer symbols (not in the public SDK).
extern int SLSMainConnectionID(void);
extern CFArrayRef SLSCopyManagedDisplaySpaces(int cid);
extern CFStringRef SLSCopyManagedDisplayForSpace(int cid, uint64_t sid);
extern uint64_t SLSManagedDisplayGetCurrentSpace(int cid, CFStringRef uuid);
extern CGError SLSSetActiveMenuBarDisplayIdentifier(int cid, CFStringRef uuid, CFStringRef repeat_uuid);
extern CGError SLSGetCurrentCursorLocation(int cid, CGPoint *point);
extern AXError _AXUIElementGetWindow(AXUIElementRef element, CGWindowID *out);
extern CGError SLSMoveWindowsToManagedSpace(int cid, CFArrayRef window_list, uint64_t sid);

#pragma mark - Run Loop Helpers

static void mimiEnsureApplication(void) {
	static dispatch_once_t onceToken;
	dispatch_once(&onceToken, ^{
		@autoreleasepool {
			[NSApplication sharedApplication];
		}
	});
}

static void mimiPumpRunLoop(CFTimeInterval seconds) { CFRunLoopRunInMode(kCFRunLoopDefaultMode, seconds, false); }

#pragma mark - Display / Space Helpers

/// Translate a display ID to its UUID string.
/// @param did Display identifier
/// @return Retained CFStringRef (caller must CFRelease), or NULL on failure
static CFStringRef mimiDisplayUUID(uint32_t did) {
	CFUUIDRef uuidRef = CGDisplayCreateUUIDFromDisplayID(did);
	if (!uuidRef) {
		return NULL;
	}

	CFStringRef uuidStr = CFUUIDCreateString(NULL, uuidRef);
	CFRelease(uuidRef);

	return uuidStr;
}

/// Translate a display UUID string back to a display ID.
/// @param uuid UUID string
/// @return Display ID, or 0 on failure
static uint32_t mimiDisplayIDFromUUID(CFStringRef uuid) {
	if (!uuid) {
		return 0;
	}

	CFUUIDRef uuidRef = CFUUIDCreateFromString(NULL, uuid);
	if (!uuidRef) {
		return 0;
	}

	uint32_t did = CGDisplayGetDisplayIDFromUUID(uuidRef);
	CFRelease(uuidRef);

	return did;
}

/// Get the current Mission Control space for a display.
/// @param did Display identifier
/// @return Space ID, or 0 on failure
static uint64_t mimiDisplaySpaceID(uint32_t did) {
	CFStringRef uuid = mimiDisplayUUID(did);
	if (!uuid) {
		return 0;
	}

	uint64_t sid = SLSManagedDisplayGetCurrentSpace(SLSMainConnectionID(), uuid);
	CFRelease(uuid);

	return sid;
}

/// Return the center point of a display's bounds (in CG coordinates).
static CGPoint mimiDisplayCenter(uint32_t did) {
	CGRect bounds = CGDisplayBounds(did);

	return (CGPoint){bounds.origin.x + bounds.size.width / 2.0, bounds.origin.y + bounds.size.height / 2.0};
}

/// Return the display ID that currently contains the cursor.
static uint32_t mimiCursorDisplayID(void) {
	CGPoint cursor;
	SLSGetCurrentCursorLocation(SLSMainConnectionID(), &cursor);

	uint32_t matchingDisplays[16];
	uint32_t matchingCount = 0;

	CGError err = CGGetDisplaysWithPoint(cursor, 16, matchingDisplays, &matchingCount);
	if (err == kCGErrorSuccess && matchingCount > 0) {
		return matchingDisplays[0];
	}

	// Fall back to the main display if the cursor is somehow outside every display.
	return CGMainDisplayID();
}

/// Set the active menu bar display, which updates which display is the
/// "focused" one for Space purposes.
static void mimiSetActiveMenuBarDisplay(uint32_t did) {
	CFStringRef uuid = mimiDisplayUUID(did);
	if (!uuid) {
		return;
	}

	SLSSetActiveMenuBarDisplayIdentifier(SLSMainConnectionID(), uuid, uuid);
	CFRelease(uuid);
}

#pragma mark - Public Space API

/// Get the total number of Mission Control spaces across all displays
/// in their current ordering.
int MimiCountMissionControlSpaces(void) {
	@autoreleasepool {
		CFArrayRef displaySpaces = SLSCopyManagedDisplaySpaces(SLSMainConnectionID());
		if (!displaySpaces) {
			return 0;
		}

		int total = 0;
		CFIndex displayCount = CFArrayGetCount(displaySpaces);
		for (CFIndex i = 0; i < displayCount; i++) {
			CFDictionaryRef displayRef = (CFDictionaryRef)CFArrayGetValueAtIndex(displaySpaces, i);
			CFArrayRef spacesRef = (CFArrayRef)CFDictionaryGetValue(displayRef, CFSTR("Spaces"));
			if (!spacesRef) {
				continue;
			}

			total += (int)CFArrayGetCount(spacesRef);
		}

		CFRelease(displaySpaces);

		return total;
	}
}

/// Get the space ID at the given 1-based Mission Control index.
uint64_t MimiMissionControlSpaceID(int index) {
	if (index < 1) {
		return 0;
	}

	@autoreleasepool {
		CFArrayRef displaySpaces = SLSCopyManagedDisplaySpaces(SLSMainConnectionID());
		if (!displaySpaces) {
			return 0;
		}

		uint64_t result = 0;
		int counter = 1;

		CFIndex displayCount = CFArrayGetCount(displaySpaces);
		for (CFIndex i = 0; i < displayCount; i++) {
			CFDictionaryRef displayRef = (CFDictionaryRef)CFArrayGetValueAtIndex(displaySpaces, i);
			CFArrayRef spacesRef = (CFArrayRef)CFDictionaryGetValue(displayRef, CFSTR("Spaces"));
			if (!spacesRef) {
				continue;
			}

			CFIndex spacesCount = CFArrayGetCount(spacesRef);
			for (CFIndex j = 0; j < spacesCount; j++) {
				if (counter == index) {
					CFDictionaryRef spaceRef = (CFDictionaryRef)CFArrayGetValueAtIndex(spacesRef, j);
					CFNumberRef sidRef = (CFNumberRef)CFDictionaryGetValue(spaceRef, CFSTR("id64"));
					if (sidRef) {
						CFNumberGetValue(sidRef, CFNumberGetType(sidRef), &result);
					}

					CFRelease(displaySpaces);

					return result;
				}

				counter++;
			}
		}

		CFRelease(displaySpaces);

		return 0;
	}
}

/// Get the display ID that owns a given space.
uint32_t MimiSpaceDisplayID(uint64_t sid) {
	CFStringRef uuid = SLSCopyManagedDisplayForSpace(SLSMainConnectionID(), sid);
	if (!uuid) {
		return 0;
	}

	uint32_t did = mimiDisplayIDFromUUID(uuid);
	CFRelease(uuid);

	return did;
}

/// Get the space ID currently active on the cursor's display.
uint64_t MimiActiveSpaceID(void) { return mimiDisplaySpaceID(mimiCursorDisplayID()); }

#pragma mark - Gesture-Based Space Focus

// Private Core Graphics event field IDs used to synthesize a high-velocity
// horizontal dock swipe that the Dock treats as a real multi-finger swipe
// gesture. These constants are not part of the public SDK and require
// suppressing -Wdeprecated-declarations around the implementation.
static const int kMimiCGSEventTypeField = 55;              // kCGSEventTypeField
static const int kMimiCGSEventDockControl = 30;            // kCGSEventDockControl
static const int kMimiCGEventGestureHIDType = 110;         // kCGEventGestureHIDType
static const int kMimiIOHIDEventTypeDockSwipe = 23;        // kIOHIDEventTypeDockSwipe
static const int kMimiCGEventGestureSwipeMotion = 123;     // kCGEventGestureSwipeMotion
static const int kMimiCGGestureMotionHorizontal = 1;       // kCGGestureMotionHorizontal
static const int kMimiCGEventGestureSwipeProgress = 124;   // kCGEventGestureSwipeProgress
static const int kMimiCGEventGestureSwipeVelocityX = 129;  // kCGEventGestureSwipeVelocityX
static const int kMimiCGEventGesturePhase = 132;           // kCGEventGesturePhase
static const int kMimiCGSGesturePhaseBegan = 1;            // kCGSGesturePhaseBegan
static const int kMimiCGSGesturePhaseEnded = 4;            // kCGSGesturePhaseEnded

/// Return the 1-based local index of a space within its display's space
/// ordering. Returns 0 if the space is not found on the given display.
static int mimiLocalSpaceIndex(uint64_t sid, uint32_t did) {
	CFStringRef uuid = mimiDisplayUUID(did);
	if (!uuid) {
		return 0;
	}

	CFArrayRef displaySpaces = SLSCopyManagedDisplaySpaces(SLSMainConnectionID());
	if (!displaySpaces) {
		CFRelease(uuid);
		return 0;
	}

	int localIndex = 0;
	CFIndex displayCount = CFArrayGetCount(displaySpaces);
	for (CFIndex i = 0; i < displayCount; i++) {
		CFDictionaryRef displayRef = (CFDictionaryRef)CFArrayGetValueAtIndex(displaySpaces, i);
		CFStringRef displayUUID = (CFStringRef)CFDictionaryGetValue(displayRef, CFSTR("Display Identifier"));
		if (!displayUUID || CFStringCompare(displayUUID, uuid, 0) != kCFCompareEqualTo) {
			continue;
		}

		CFArrayRef spacesRef = (CFArrayRef)CFDictionaryGetValue(displayRef, CFSTR("Spaces"));
		if (!spacesRef) {
			break;
		}

		CFIndex spacesCount = CFArrayGetCount(spacesRef);
		for (CFIndex j = 0; j < spacesCount; j++) {
			localIndex++;
			CFDictionaryRef spaceRef = (CFDictionaryRef)CFArrayGetValueAtIndex(spacesRef, j);
			CFNumberRef sidRef = (CFNumberRef)CFDictionaryGetValue(spaceRef, CFSTR("id64"));
			if (sidRef) {
				uint64_t curSid = 0;
				CFNumberGetValue(sidRef, CFNumberGetType(sidRef), &curSid);
				if (curSid == sid) {
					CFRelease(displaySpaces);
					CFRelease(uuid);
					return localIndex;
				}
			}
		}
	}

	CFRelease(displaySpaces);
	CFRelease(uuid);
	return 0;
}

/// Focus a space using a synthetic high-velocity horizontal dock swipe
/// gesture to skip the standard Mission Control swipe animation — macOS
/// exposes no public API to activate a space directly.
///
/// Technique attribution: reverse-engineered from BetterTouchTool. Prior
/// art: https://github.com/jurplel/InstantSpaceSwitcher and the wacom-driver-fix
/// project by thenickdude.
int MimiFocusSpaceUsingGesture(uint32_t new_did, uint64_t new_sid) {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

	mimiEnsureApplication();

	uint32_t curDid = mimiCursorDisplayID();
	CGPoint point = mimiDisplayCenter(new_did);
	bool focusDisplay = curDid != new_did;

	if (focusDisplay) {
		CGWarpMouseCursorPosition(point);
		// Give the system a moment to process the warp before querying the
		// current space on the target display.
		mimiPumpRunLoop(kMimiSpaceGestureProcessingDelay);
	}

	// After any warp, resolve the swipe count using per-display local space
	// indices. Swipe gestures navigate spaces on the active display only, so
	// the global Mission Control index distance is wrong when crossing displays.
	uint64_t fromSid = mimiDisplaySpaceID(new_did);
	int fromIdx = mimiLocalSpaceIndex(fromSid, new_did);
	int toIdx = mimiLocalSpaceIndex(new_sid, new_did);

	if (fromIdx == 0 || toIdx == 0) {
		// Could not resolve local indices (e.g. transient state).
		// Best-effort fallback: ensure the right display is active so the OS
		// picks the closest matching space on that display.
		mimiSetActiveMenuBarDisplay(new_did);
		mimiPumpRunLoop(kMimiSpaceGestureProcessingDelay);

		return 1;
	}

	int count = abs(toIdx - fromIdx);
	if (count == 0) {
		// Already on the correct local space on the target display. Make sure
		// the menu bar is on the right display when crossing displays.
		if (focusDisplay) {
			mimiSetActiveMenuBarDisplay(new_did);
			if (mimiDisplaySpaceID(new_did) != new_sid) {
				CGPostMouseEvent(point, false, 1, true);
				CGPostMouseEvent(point, false, 1, false);
			}
		}

		mimiPumpRunLoop(kMimiSpaceGestureProcessingDelay);

		return 1;
	}

	CGEventRef event = CGEventCreate(NULL);
	if (!event) {
		return 0;
	}

	double sign = (toIdx - fromIdx) > 0 ? 1.0 : -1.0;

	CGEventSetIntegerValueField(event, kMimiCGSEventTypeField, kMimiCGSEventDockControl);
	CGEventSetIntegerValueField(event, kMimiCGEventGestureHIDType, kMimiIOHIDEventTypeDockSwipe);
	CGEventSetIntegerValueField(event, kMimiCGEventGestureSwipeMotion, kMimiCGGestureMotionHorizontal);
	CGEventSetDoubleValueField(event, kMimiCGEventGestureSwipeProgress, sign);
	CGEventSetDoubleValueField(event, kMimiCGEventGestureSwipeVelocityX, sign * 9999.0);

	for (int i = 0; i < count; i++) {
		CGEventSetIntegerValueField(event, kMimiCGEventGesturePhase, kMimiCGSGesturePhaseBegan);
		CGEventPost(kCGSessionEventTap, event);
		CGEventSetIntegerValueField(event, kMimiCGEventGesturePhase, kMimiCGSGesturePhaseEnded);
		CGEventPost(kCGSessionEventTap, event);
	}

	CFRelease(event);

	if (focusDisplay) {
		mimiSetActiveMenuBarDisplay(new_did);
		if (mimiDisplaySpaceID(new_did) != new_sid) {
			CGPostMouseEvent(point, false, 1, true);
			CGPostMouseEvent(point, false, 1, false);
		}
	}

	mimiPumpRunLoop(kMimiSpaceGestureProcessingDelay * (CFTimeInterval)count + kMimiSpaceGestureProcessingDelay);

	return 1;

#pragma clang diagnostic pop
}

#pragma mark - Mach-O / Symbol Resolution Helpers

static struct mach_header_64 *mimi_macho_find_image_header(const char *target_name, uint64_t *slide) {
	uint32_t image_count = _dyld_image_count();
	for (uint32_t i = 0; i < image_count; ++i) {
		const char *image_name = _dyld_get_image_name(i);
		if (!image_name)
			continue;
		if (strcmp(image_name, target_name) == 0) {
			*slide = _dyld_get_image_vmaddr_slide(i);
			return (struct mach_header_64 *)_dyld_get_image_header(i);
		}
	}
	return NULL;
}

static struct segment_command_64 *mimi_macho_find_linkedit_segment(struct mach_header_64 *header) {
	uint64_t offset = sizeof(struct mach_header_64);
	for (uint32_t i = 0; i < header->ncmds; ++i) {
		struct load_command *cmd = (struct load_command *)(((uint8_t *)header) + offset);
		if (cmd->cmd == LC_SEGMENT_64) {
			struct segment_command_64 *segment = (struct segment_command_64 *)cmd;
			if (strcmp(segment->segname, SEG_LINKEDIT) == 0) {
				return segment;
			}
		}
		offset += cmd->cmdsize;
	}
	return NULL;
}

static struct symtab_command *mimi_macho_find_symtab_command(struct mach_header_64 *header) {
	uint64_t offset = sizeof(struct mach_header_64);
	for (uint32_t i = 0; i < header->ncmds; ++i) {
		struct load_command *cmd = (struct load_command *)(((uint8_t *)header) + offset);
		if (cmd->cmd == LC_SYMTAB) {
			return (struct symtab_command *)cmd;
		}
		offset += cmd->cmdsize;
	}
	return NULL;
}

static void *mimi_macho_find_symbol(const char *target_image, const char *target_symbol) {
	uint64_t slide = 0;
	struct mach_header_64 *header = mimi_macho_find_image_header(target_image, &slide);
	if (!header)
		return NULL;
	struct segment_command_64 *linkedit_segment = mimi_macho_find_linkedit_segment(header);
	if (!linkedit_segment)
		return NULL;
	struct symtab_command *symtab_command = mimi_macho_find_symtab_command(header);
	if (!symtab_command)
		return NULL;
	uint32_t symbol_count = symtab_command->nsyms;
	void *symbol_str = (void *)(linkedit_segment->vmaddr - linkedit_segment->fileoff) + symtab_command->stroff + slide;
	void *symbol_sym = (void *)(linkedit_segment->vmaddr - linkedit_segment->fileoff) + symtab_command->symoff + slide;
	for (uint32_t i = 0; i < symbol_count; ++i) {
		struct nlist_64 *list = (void *)symbol_sym + (i * sizeof(struct nlist_64));
		char *symbol_name = (char *)symbol_str + list->n_un.n_strx;
		if (strcmp(symbol_name, target_symbol) == 0) {
			return (void *)(list->n_value + slide);
		}
	}
	return NULL;
}

#pragma mark - Public Display Helpers

/// Return the display ID that currently contains the cursor.
uint32_t MimiCursorDisplayID(void) { return mimiCursorDisplayID(); }

/// Activate a display by setting it as the active menu bar display.
/// This keeps WindowServer's event routing state coherent when windows
/// are moved across displays.
void MimiActivateDisplay(uint32_t did) { mimiSetActiveMenuBarDisplay(did); }

#pragma mark - Window-to-Space Movement

// Private SkyLight API — undocumented, unsupported, may break on any
// macOS update. Dynamically resolved so the tool degrades gracefully, but
// no guarantees are made about future compatibility.
@protocol SLSBridgedMoveWindowsToManagedSpaceOperationProtocol <NSObject>
- (instancetype)initWithWindows:(id)windows spaceID:(uint64_t)spaceID;
@end

int MimiMoveWindowToSpace(void *windowElement, uint64_t spaceID) {
	if (!windowElement) {
		return 0;
	}

	mimiEnsureApplication();

	CGWindowID windowId = 0;
	AXError err = _AXUIElementGetWindow((AXUIElementRef)windowElement, &windowId);
	if (err != kAXErrorSuccess || windowId == 0) {
		MIMI_LOG("_AXUIElementGetWindow failed with error %d (windowId=%u)", (int)err, (unsigned)windowId);
		return 0;
	}

	// Create CFArray of window ID
	CFNumberRef windowNumber = CFNumberCreate(NULL, kCFNumberSInt32Type, &windowId);
	if (!windowNumber) {
		return 0;
	}
	CFArrayRef windowList = CFArrayCreate(NULL, (const void **)&windowNumber, 1, &kCFTypeArrayCallBacks);
	CFRelease(windowNumber);
	if (!windowList) {
		return 0;
	}

	int success = 0;

	// Resolve SLSPerformAsynchronousBridgedWindowManagementOperation dynamically
	static int64_t (*SLSPerformAsynchronousBridgedWindowManagementOperation)(void *) = NULL;
	static dispatch_once_t onceToken;
	dispatch_once(&onceToken, ^{
		SLSPerformAsynchronousBridgedWindowManagementOperation = (int64_t (*)(void *))mimi_macho_find_symbol(
		    "/System/Library/PrivateFrameworks/SkyLight.framework/Versions/A/SkyLight",
		    "__"
		    "ZL54SLSPerformAsynchronousBridgedWindowManagementOperationP47SLSAsynchronousBridgedWindowManagementOperati"
		    "on");
	});

	if (SLSPerformAsynchronousBridgedWindowManagementOperation) {
		Class cls = objc_getClass("SLSBridgedMoveWindowsToManagedSpaceOperation");
		if (cls) {
			id operation = [(id<SLSBridgedMoveWindowsToManagedSpaceOperationProtocol>)[cls alloc]
			    initWithWindows:(__bridge id)windowList
			            spaceID:spaceID];
			if (operation) {
				SLSPerformAsynchronousBridgedWindowManagementOperation((__bridge void *)operation);
				success = 1;
			}
		}
	}

	// Fallback to SLSMoveWindowsToManagedSpace
	if (!success) {
		CGError cgErr = SLSMoveWindowsToManagedSpace(SLSMainConnectionID(), windowList, spaceID);
		if (cgErr == kCGErrorSuccess) {
			success = 1;
		} else {
			MIMI_LOG(
			    "SLSMoveWindowsToManagedSpace failed with error %d (windowId=%u, spaceID=%llu)", (int)cgErr,
			    (unsigned)windowId, (unsigned long long)spaceID);
		}
	}

	CFRelease(windowList);

	if (success) {
		mimiPumpRunLoop(kMimiMoveWindowProcessingDelay);
	}

	return success;
}
