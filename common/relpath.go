package common

import (
	"strings"

	"github.com/aristanetworks/goarista/key"
	"github.com/aristanetworks/goarista/path"
)

// RelPath represents a relative path.
// It interprets the inner path as if it did not have a trailing slash.
type RelPath struct {
	path key.Path
}

// NewRelPath creates a new RelPath from a string.
// Leading slashes on the string will simply be ignored.
func NewRelPath(str string) RelPath {
	return RelPath{path.FromString(str)}
}

// String returns the string representation of the RelPath.
// An empty path returns an empty string.
func (r RelPath) String() string {
	return strings.TrimLeft(r.path.String(), "/")
}

func (r RelPath) Path() key.Path {
	return r.path
}
