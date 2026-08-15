//
//  dockswipe.h
//  Mimi
//
//  Copyright © 2025 Mimi. All rights reserved.
//

#ifndef MIMI_DOCKSWIPE_H
#define MIMI_DOCKSWIPE_H

#import <CoreGraphics/CoreGraphics.h>
#import <stdbool.h>

// macOS 27 stopped reconstructing a dock swipe from the gesture fields set on
// a synthetic CGEvent. The Dock now reads a serialized IOHID queue payload
// carried in the event's data representation, so a synthetic swipe that only
// sets the gesture fields is silently ignored. These helpers gate and build
// that payload; everything here is undocumented and may break on any update.

/// Whether the running macOS requires synthetic dock swipes to carry a
/// serialized IOHID payload. True on macOS 27 and later.
///
/// The answer depends on the running OS, not the SDK the binary was built
/// against, and is cached after the first call. Setting
/// `MIMI_FORCE_DOCK_SWIPE_AUGMENTATION` to `1` or `0` overrides the version
/// check, which is the only way to exercise either path on a single machine.
bool MimiDockSwipeRequiresAugmentation(void);

/// Copy `event` with a serialized IOHID queue payload appended, built from the
/// gesture fields already set on it.
/// @param event Dock-control gesture event to augment
/// @return Retained CGEventRef (caller must CFRelease), or NULL on failure
CGEventRef MimiDockSwipeAugment(CGEventRef event);

#endif  // MIMI_DOCKSWIPE_H
