//
//  dockswipe.m
//  Mimi
//
//  Copyright © 2025 Mimi. All rights reserved.
//

#import "dockswipe.h"

#import "mimi_log.h"

#import <CoreFoundation/CoreFoundation.h>
#import <mach/mach_time.h>
#import <stdint.h>
#import <stdlib.h>
#import <string.h>
#import <sys/sysctl.h>

#pragma mark - IOHID Queue Layout

// Layout of the IOHID event records the Dock expects to find serialized inside
// a synthetic dock-swipe CGEvent on macOS 27+. Reverse-engineered from
// IOHIDFamily; packed because the Dock parses the bytes positionally.
#pragma pack(push, 1)

typedef struct {
	uint32_t size;
	uint32_t type;
	uint32_t options;
	uint8_t depth;
	uint8_t reserved[3];
} MimiIOHIDEventBase;

typedef struct {
	MimiIOHIDEventBase base;
	int32_t positionX;
	int32_t positionY;
	int32_t positionZ;
	uint32_t swipeMask;
	uint16_t gestureMotion;
	uint16_t gestureFlavor;
	int32_t swipeProgress;
} MimiIOHIDFluidTouchGestureData;

typedef struct {
	MimiIOHIDEventBase base;
	int32_t velocityX;
	int32_t velocityY;
	int32_t velocityZ;
} MimiIOHIDVelocityEventData;

typedef struct {
	uint64_t timestamp;
	uint64_t senderID;
	uint32_t options;
	uint32_t attributeLength;
	uint32_t eventCount;
} MimiIOHIDSystemQueueElementHeader;

#pragma pack(pop)

// The Dock parses these records positionally, so a layout drift would be
// silently mis-read rather than rejected. Fail the build instead.
_Static_assert(sizeof(MimiIOHIDEventBase) == 16, "unexpected IOHID event base layout");
_Static_assert(sizeof(MimiIOHIDFluidTouchGestureData) == 40, "unexpected IOHID fluid gesture layout");
_Static_assert(sizeof(MimiIOHIDVelocityEventData) == 28, "unexpected IOHID velocity layout");
_Static_assert(sizeof(MimiIOHIDSystemQueueElementHeader) == 28, "unexpected IOHID queue header layout");

// See IOHIDEventType in IOHIDFamily.
static const uint32_t kMimiIOHIDEventTypeVelocity = 9;
static const uint32_t kMimiIOHIDEventTypeFluidTouchGesture = 23;
static const uint16_t kMimiIOHIDGestureFlavorDockPrimary = 3;

// Gesture fields read back off the CGEvent when building the payload. Kept
// local to this file so the payload builder stays independent of whichever
// caller populated the event.
static const int kMimiCGEventGestureSwipeMask = 115;
static const int kMimiCGEventGestureSwipeMotion = 123;
static const int kMimiCGEventGestureSwipeProgress = 124;
static const int kMimiCGEventGestureSwipePositionX = 125;
static const int kMimiCGEventGestureSwipePositionY = 126;
static const int kMimiCGEventGestureSwipeVelocityX = 129;
static const int kMimiCGEventGestureSwipeVelocityY = 130;
static const int kMimiCGEventGesturePhase = 132;

static const int kMimiCGSGesturePhaseEnded = 4;

// Field ID the payload is attached under in the serialized event, and the
// serialization format version this encoding is known to match.
static const uint16_t kMimiCGEventRawIOHIDPayloadField = 4205;
static const uint8_t kMimiCGEventDataFormatVersion = 2;

/// Convert a double to the 16.16 fixed-point encoding IOHID uses, preserving
/// the sign of values that would otherwise truncate to zero. Direction matters
/// more than magnitude here, and truncating a tiny progress value to zero reads
/// as "no movement".
static int32_t mimiDoubleToFixed1616(double value) {
	int32_t fixed = (int32_t)(value * 65536.0);
	if (fixed == 0 && value != 0.0) {
		return value > 0.0 ? 1 : -1;
	}

	return fixed;
}

/// Build the IOHID queue payload described by an event's gesture fields.
/// @param event Dock-control gesture event to read fields from
/// @param outLength Receives the payload length in bytes
/// @return malloc'd buffer (caller must free), or NULL on allocation failure
static uint8_t *mimiBuildIOHIDPayload(CGEventRef event, size_t *outLength) {
	int64_t phase = CGEventGetIntegerValueField(event, kMimiCGEventGesturePhase);
	int64_t motion = CGEventGetIntegerValueField(event, kMimiCGEventGestureSwipeMotion);
	int64_t swipeMask = CGEventGetIntegerValueField(event, kMimiCGEventGestureSwipeMask);
	double progress = CGEventGetDoubleValueField(event, kMimiCGEventGestureSwipeProgress);
	double posX = CGEventGetDoubleValueField(event, kMimiCGEventGestureSwipePositionX);
	double posY = CGEventGetDoubleValueField(event, kMimiCGEventGestureSwipePositionY);
	double velX = CGEventGetDoubleValueField(event, kMimiCGEventGestureSwipeVelocityX);
	double velY = CGEventGetDoubleValueField(event, kMimiCGEventGestureSwipeVelocityY);

	// A real trackpad only reports velocity once the fingers lift, so the
	// velocity record is omitted unless there is a velocity to report.
	bool includeVelocity = (velX != 0.0 || velY != 0.0 || phase == kMimiCGSGesturePhaseEnded);

	size_t length = sizeof(MimiIOHIDSystemQueueElementHeader) + sizeof(MimiIOHIDFluidTouchGestureData);
	if (includeVelocity) {
		length += sizeof(MimiIOHIDVelocityEventData);
	}

	uint8_t *payload = (uint8_t *)calloc(1, length);
	if (!payload) {
		return NULL;
	}

	MimiIOHIDSystemQueueElementHeader *header = (MimiIOHIDSystemQueueElementHeader *)payload;
	uint64_t timestamp = CGEventGetTimestamp(event);
	header->timestamp = timestamp != 0 ? timestamp : mach_absolute_time();
	header->eventCount = includeVelocity ? 2 : 1;

	MimiIOHIDFluidTouchGestureData *fluid =
	    (MimiIOHIDFluidTouchGestureData *)(payload + sizeof(MimiIOHIDSystemQueueElementHeader));
	fluid->base.size = sizeof(MimiIOHIDFluidTouchGestureData);
	fluid->base.type = kMimiIOHIDEventTypeFluidTouchGesture;
	fluid->base.options = (uint32_t)((phase & 0xFF) << 24);
	fluid->positionX = mimiDoubleToFixed1616(posX);
	fluid->positionY = mimiDoubleToFixed1616(posY);
	fluid->swipeMask = (uint32_t)swipeMask;
	fluid->gestureMotion = (uint16_t)motion;
	fluid->gestureFlavor = kMimiIOHIDGestureFlavorDockPrimary;
	fluid->swipeProgress = mimiDoubleToFixed1616(progress);

	if (includeVelocity) {
		MimiIOHIDVelocityEventData *velocity =
		    (MimiIOHIDVelocityEventData *)(payload + sizeof(MimiIOHIDSystemQueueElementHeader) +
		                                   sizeof(MimiIOHIDFluidTouchGestureData));
		velocity->base.size = sizeof(MimiIOHIDVelocityEventData);
		velocity->base.type = kMimiIOHIDEventTypeVelocity;
		velocity->base.depth = 1;
		velocity->velocityX = mimiDoubleToFixed1616(velX);
		velocity->velocityY = mimiDoubleToFixed1616(velY);
	}

	*outLength = length;

	return payload;
}

#pragma mark - Public Dock Swipe API

CGEventRef MimiDockSwipeAugment(CGEventRef event) {
	if (!event) {
		return NULL;
	}

	CFDataRef data = CGEventCreateData(kCFAllocatorDefault, event);
	if (!data) {
		MIMI_LOG("dock swipe augmentation failed: could not serialize event");

		return NULL;
	}

	const uint8_t *bytes = CFDataGetBytePtr(data);
	CFIndex length = CFDataGetLength(data);

	// The trailing-field encoding below is only known to hold for format
	// version 2. Bail out rather than append bytes the Dock may misparse.
	if (length < 4 || bytes[0] != 0 || bytes[1] != 0 || bytes[2] != 0 || bytes[3] != kMimiCGEventDataFormatVersion) {
		MIMI_LOG("dock swipe augmentation failed: unexpected event data format (length=%ld)", (long)length);
		CFRelease(data);

		return NULL;
	}

	size_t payloadLength = 0;
	uint8_t *payload = mimiBuildIOHIDPayload(event, &payloadLength);
	if (!payload) {
		CFRelease(data);

		return NULL;
	}

	// Serialized event, then a 4-byte trailing field header (big-endian payload
	// length, then field ID), then the payload itself.
	size_t newLength = (size_t)length + 4 + payloadLength;
	uint8_t *newBytes = (uint8_t *)malloc(newLength);
	if (!newBytes) {
		free(payload);
		CFRelease(data);

		return NULL;
	}

	memcpy(newBytes, bytes, (size_t)length);
	newBytes[length] = (uint8_t)((payloadLength >> 8) & 0xFF);
	newBytes[length + 1] = (uint8_t)(payloadLength & 0xFF);
	newBytes[length + 2] = (uint8_t)((kMimiCGEventRawIOHIDPayloadField >> 8) & 0xFF);
	newBytes[length + 3] = (uint8_t)(kMimiCGEventRawIOHIDPayloadField & 0xFF);
	memcpy(newBytes + length + 4, payload, payloadLength);

	free(payload);
	CFRelease(data);

	CFDataRef newData = CFDataCreate(kCFAllocatorDefault, newBytes, (CFIndex)newLength);
	free(newBytes);
	if (!newData) {
		return NULL;
	}

	CGEventRef augmented = CGEventCreateFromData(kCFAllocatorDefault, newData);
	CFRelease(newData);

	if (!augmented) {
		MIMI_LOG("dock swipe augmentation failed: could not rebuild event from data");
	}

	return augmented;
}

bool MimiDockSwipeRequiresAugmentation(void) {
	static int cached = -1;
	if (cached != -1) {
		return cached == 1;
	}

	const char *override = getenv("MIMI_FORCE_DOCK_SWIPE_AUGMENTATION");
	if (override) {
		cached = (strcmp(override, "1") == 0) ? 1 : 0;
		MIMI_LOG("dock swipe augmentation forced by environment (enabled=%d)", cached);

		return cached == 1;
	}

	// Ask the kernel for the running product version rather than the SDK the
	// binary was built against — the Dock's behaviour follows the former.
	char version[32];
	size_t size = sizeof(version);
	if (sysctlbyname("kern.osproductversion", version, &size, NULL, 0) != 0) {
		MIMI_LOG("could not read kern.osproductversion; assuming legacy dock swipe encoding");
		cached = 0;

		return false;
	}

	int major = 0;
	if (sscanf(version, "%d", &major) != 1) {
		MIMI_LOG("could not parse kern.osproductversion; assuming legacy dock swipe encoding");
		cached = 0;

		return false;
	}

	cached = (major >= 27) ? 1 : 0;

	return cached == 1;
}
