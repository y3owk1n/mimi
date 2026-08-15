package errors_test

// This file pins the one boundary the compiler cannot check: the "Available
// error codes" list in docs/CODING_STANDARDS.md and the exported Code*
// constants in errors.go are two independently hand-maintained lists of the
// same set. If a future edit adds, removes, or renames a Code* constant
// without updating the doc (or vice versa), this test fails instead of the
// drift sitting undetected until a contributor reaches for a code the docs
// promised and the compiler rejects it.
//
// It reads both files as plain text fixtures via regexp rather than using
// go/ast, following the same approach as
// internal/native/eventkinds_test.go's parsing of eventkinds.h.
//
// Unlike that precedent, this file lives in package errors_test (not
// errors) rather than carrying a //nolint:testpackage: it only needs the
// exported Code* names as plain text, never an unexported identifier, so
// there is nothing here that requires internal package access.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// codeConstRe matches an exported Code* constant declaration inside the Code
// enum block in errors.go, e.g.:
//
//	CodeAccessibilityDenied Code = "ACCESSIBILITY_DENIED"
var codeConstRe = regexp.MustCompile(`(?m)^\t(Code\w+)\s+Code\s*=\s*"[^"]*"\s*$`)

// docCodeRe matches a single backtick-quoted Code* identifier, e.g.
// "`CodeAccessibilityDenied`".
var docCodeRe = regexp.MustCompile("`(Code\\w+)`")

// parseErrorCodeConstants extracts every exported Code* constant name
// declared in errors.go.
func parseErrorCodeConstants(t *testing.T) map[string]struct{} {
	t.Helper()

	// go test runs with the package directory as the working directory, so
	// this relative path is stable regardless of the caller's cwd.
	data, err := os.ReadFile("errors.go")
	if err != nil {
		t.Fatalf("reading errors.go: %v", err)
	}

	matches := codeConstRe.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatal("errors.go: found no Code* constant declarations; parser regex may be stale")
	}

	names := make(map[string]struct{}, len(matches))

	for _, m := range matches {
		name := m[1]

		if _, dup := names[name]; dup {
			t.Fatalf("errors.go: Code%s is declared more than once", name)
		}

		names[name] = struct{}{}
	}

	return names
}

// parseDocErrorCodes extracts every backtick-quoted Code* identifier from
// the "Available error codes" line in docs/CODING_STANDARDS.md.
func parseDocErrorCodes(t *testing.T) map[string]struct{} {
	t.Helper()

	data, err := os.ReadFile("../../docs/CODING_STANDARDS.md")
	if err != nil {
		t.Fatalf("reading docs/CODING_STANDARDS.md: %v", err)
	}

	const marker = "Available error codes:"

	text := string(data)

	idx := strings.Index(text, marker)
	if idx == -1 {
		t.Fatal(
			"docs/CODING_STANDARDS.md: found no \"Available error codes:\" line; doc may have been reworded",
		)
	}

	// The list runs to the end of the line it starts on.
	line := text[idx:]
	if nl := strings.IndexByte(line, '\n'); nl != -1 {
		line = line[:nl]
	}

	matches := docCodeRe.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		t.Fatal(
			"docs/CODING_STANDARDS.md: \"Available error codes\" line has no backtick-quoted Code* identifiers",
		)
	}

	names := make(map[string]struct{}, len(matches))

	for _, m := range matches {
		name := m[1]

		if _, dup := names[name]; dup {
			t.Fatalf("docs/CODING_STANDARDS.md: %s is listed more than once", name)
		}

		names[name] = struct{}{}
	}

	return names
}

func TestCodingStandardsErrorCodeListMatchesErrorsGo(t *testing.T) {
	t.Parallel()

	codeConsts := parseErrorCodeConstants(t)
	docCodes := parseDocErrorCodes(t)

	for name := range codeConsts {
		if _, ok := docCodes[name]; !ok {
			t.Errorf("errors.go declares %s, which docs/CODING_STANDARDS.md's "+
				"\"Available error codes\" list does not mention", name)
		}
	}

	for name := range docCodes {
		if _, ok := codeConsts[name]; !ok {
			t.Errorf("docs/CODING_STANDARDS.md's \"Available error codes\" list mentions %s, "+
				"which errors.go does not declare", name)
		}
	}
}
