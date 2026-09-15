package native

/*
#include "dockswipe.h"
*/
import "C"

// DockSwipeAugmented reports whether a synthetic dock swipe on this macOS
// must carry the serialized IOHID payload macOS 27 began reading, after the
// MIMI_FORCE_DOCK_SWIPE_AUGMENTATION override when it is set. It is what
// mimi doctor prints so a user near that boundary can see which encoding a
// space switch is sent with.
func DockSwipeAugmented() bool {
	return bool(C.MimiDockSwipeRequiresAugmentation())
}
