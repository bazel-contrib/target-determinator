package versions

import (
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/require"
)

func TestReleaseIsInRange(t *testing.T) {
	for name, tc := range map[string]struct {
		bazelReleaseString string
		min                string
		max                string
		wantResult         *bool
		wantExplanation    string
	}{
		"in_range": {
			bazelReleaseString: "release 7.0.0",
			min:                "6.4.0",
			max:                "8.0.0",
			wantResult:         ptr(true),
			wantExplanation:    "",
		},
		"at_max": {
			bazelReleaseString: "release 7.0.0",
			min:                "6.4.0",
			max:                "7.0.0",
			wantResult:         ptr(false),
			wantExplanation:    "Bazel version 7.0.0 was not less than maximum 7.0.0",
		},
		"at_min": {
			bazelReleaseString: "release 7.0.0",
			min:                "7.0.0",
			max:                "8.0.0",
			wantResult:         ptr(true),
			wantExplanation:    "",
		},
		"above_max": {
			bazelReleaseString: "release 7.0.0",
			min:                "6.4.0",
			max:                "6.5.0",
			wantResult:         ptr(false),
			wantExplanation:    "Bazel version 7.0.0 was not less than maximum 6.5.0",
		},
		"below_min": {
			bazelReleaseString: "release 6.4.0",
			min:                "7.0.0",
			max:                "7.1.0",
			wantResult:         ptr(false),
			wantExplanation:    "Bazel version 6.4.0 was less than minimum 7.0.0",
		},
		"no_release_prefix": {
			bazelReleaseString: "7.0.0",
			min:                "6.4.0",
			max:                "8.0.0",
			wantResult:         nil,
			wantExplanation:    "Bazel wasn't a released version",
		},
		"no_version": {
			bazelReleaseString: "release beep",
			min:                "6.4.0",
			max:                "8.0.0",
			wantResult:         nil,
			wantExplanation:    "Failed to parse Bazel version \"release beep\"",
		},
		"prerelease_in_range": {
			bazelReleaseString: "release 8.0.0-pre.20240101.1",
			min:                "7.0.0",
			max:                "8.0.0",
			wantResult:         ptr(true),
			wantExplanation:    "",
		},
		"prerelease_below_range": {
			bazelReleaseString: "release 8.0.0-pre.20240101.1",
			min:                "8.0.0",
			max:                "8.1.0",
			wantResult:         ptr(false),
			wantExplanation:    "Bazel version 8.0.0-pre.20240101.1 was less than minimum 8.0.0",
		},
		"prerelease_above_range": {
			bazelReleaseString: "release 8.0.0-pre.20240101.1",
			min:                "7.0.0",
			max:                "7.1.0",
			wantResult:         ptr(false),
			wantExplanation:    "Bazel version 8.0.0-pre.20240101.1 was not less than maximum 7.1.0",
		},
		"above_only_min": {
			bazelReleaseString: "release 7.0.0",
			min:                "6.4.0",
			max:                "",
			wantResult:         ptr(true),
			wantExplanation:    "",
		},
		"at_only_min": {
			bazelReleaseString: "release 6.4.0",
			min:                "6.4.0",
			max:                "",
			wantResult:         ptr(true),
			wantExplanation:    "",
		},
		"below_only_max": {
			bazelReleaseString: "release 6.4.0",
			min:                "",
			max:                "7.0.0",
			wantResult:         ptr(true),
			wantExplanation:    "",
		},
		"at_only_max": {
			bazelReleaseString: "release 7.0.0",
			min:                "",
			max:                "7.0.0",
			wantResult:         ptr(false),
			wantExplanation:    "Bazel version 7.0.0 was not less than maximum 7.0.0",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var min *version.Version
			if tc.min != "" {
				min = version.Must(version.NewVersion(tc.min))
			}
			var max *version.Version
			if tc.max != "" {
				max = version.Must(version.NewVersion(tc.max))
			}
			result, explanation := ReleaseIsInRange(tc.bazelReleaseString, min, max)
			if tc.wantResult == nil {
				require.Nil(t, result)
			} else {
				require.NotNil(t, result)
				require.Equal(t, *tc.wantResult, *result)
			}
			require.Equal(t, tc.wantExplanation, explanation)
		})
	}
}
