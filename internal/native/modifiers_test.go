package native //nolint:testpackage // modifierNames is unexported

import (
	"slices"
	"testing"
)

func TestModifierNames_NamesTheHeldKeysInOrder(t *testing.T) {
	t.Parallel()

	got := modifierNames(flagCommand | flagShift)
	if !slices.Equal(got, []string{"shift", "command"}) {
		t.Fatalf("got %v", got)
	}

	if none := modifierNames(0); none != nil {
		t.Fatalf("no keys gave %v", none)
	}
}
