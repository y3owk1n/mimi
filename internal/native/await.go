package native

import "time"

// spaceSettleTimeout is how long a space switch is given to land. A swipe
// the Dock takes changes the space in front within a few frames. One it
// drops never changes it, and a second is long past either.
const spaceSettleTimeout = time.Second

// spaceSettlePoll is how often the space in front is read while waiting.
const spaceSettlePoll = 20 * time.Millisecond

// awaitSpace polls inFront until it reports sid or spaceSettleTimeout
// passes, and reports whether it did.
func awaitSpace(inFront func() uint64, sid uint64) bool {
	deadline := time.Now().Add(spaceSettleTimeout)

	for {
		if inFront() == sid {
			return true
		}

		if time.Now().After(deadline) {
			return false
		}

		time.Sleep(spaceSettlePoll)
	}
}
