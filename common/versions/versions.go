package versions

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-version"
)

func ReleaseIsInRange(releaseString string, min *version.Version, max *version.Version) (*bool, string) {
	releasePrefix := "release "
	if !strings.HasPrefix(releaseString, releasePrefix) {
		return nil, "Bazel wasn't a released version"
	}

	bazelVersion, err := version.NewVersion(releaseString[len(releasePrefix):])
	if err != nil {
		return nil, fmt.Sprintf("Failed to parse Bazel version %q", releaseString)
	}
	if min != nil && !bazelVersion.GreaterThanOrEqual(min) {
		return ptr(false), fmt.Sprintf("Bazel version %s was less than minimum %s", bazelVersion, min.String())
	}
	if max != nil && !max.GreaterThan(bazelVersion) {
		return ptr(false), fmt.Sprintf("Bazel version %s was not less than maximum %s", bazelVersion, max.String())
	}
	return ptr(true), ""
}

func ptr[V any](v V) *V {
	return &v
}
