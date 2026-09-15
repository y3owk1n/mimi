package native //nolint:testpackage // awaitSpace is unexported

import "testing"

func TestAwaitSpace_ReturnsOnceTheSpaceLands(t *testing.T) {
	t.Parallel()

	reads := 0
	inFront := func() uint64 {
		reads++
		if reads < 3 {
			return 1
		}

		return 2
	}

	if !awaitSpace(inFront, 2) {
		t.Fatal("the space landed and was not seen")
	}
}

func TestAwaitSpace_GivesUpAtTheDeadline(t *testing.T) {
	t.Parallel()

	if awaitSpace(func() uint64 { return 1 }, 2) {
		t.Fatal("reported a space that never came in front")
	}
}
