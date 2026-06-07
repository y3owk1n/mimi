package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome expands a leading ~ to the current user's home directory.
// If path does not start with ~, it is returned unchanged.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()

		return filepath.Join(home, path[1:])
	}

	return path
}
