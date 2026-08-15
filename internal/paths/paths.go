package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome expands a leading ~ to the current user's home directory.
// If path does not start with ~, it is returned unchanged. If the home
// directory cannot be resolved, path is also returned unchanged (still
// containing the literal ~) rather than being joined onto an empty
// string, which would silently produce a path relative to the current
// working directory.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}

		return filepath.Join(home, path[1:])
	}

	return path
}
