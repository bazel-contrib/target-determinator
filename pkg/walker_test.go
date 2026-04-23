package pkg

import (
	"testing"

	ss "github.com/bazel-contrib/target-determinator/common/sorted_set"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/analysis"
	gazelle_label "github.com/bazelbuild/bazel-gazelle/label"
)

func makeMatchingTargets(lbl gazelle_label.Label, config Configuration) *MatchingTargets {
	return &MatchingTargets{
		labels: ss.NewSortedSetFn([]gazelle_label.Label{lbl}, CompareLabels),
		labelsToConfigurations: map[gazelle_label.Label]*ss.SortedSet[Configuration]{
			lbl: ss.NewSortedSetFn([]Configuration{config}, ConfigurationLess),
		},
	}
}

// TestDiffSingleLabel_NoDifferenceWhenBothFromCache verifies that when both before and after
// metadata are loaded from cache (TransitiveConfiguredTargets == nil, pre-computed hashes
// present), DiffSingleLabel does not report any differences for an unchanged target.
func TestDiffSingleLabel_NoDifferenceWhenBothFromCache(t *testing.T) {
	const bazelRelease = "release 7.0.0"
	lbl := mustParseLabel("//foo:bar")
	config := NormalizeConfiguration("deadcafe")

	mt := makeMatchingTargets(lbl, config)
	fakeHash := []byte{0xde, 0xad, 0xca, 0xfe}
	hashKey := lbl.String() + "\x00" + config.String()

	makeFromCache := func() *QueryResults {
		thc := NewTargetHashCache(nil, &Normalizer{}, bazelRelease, false)
		if err := thc.RestoreHashes(map[string][]byte{hashKey: fakeHash}); err != nil {
			t.Fatalf("RestoreHashes: %v", err)
		}
		return &QueryResults{
			MatchingTargets:             mt,
			TransitiveConfiguredTargets: nil, // as set by LoadFromCache
			TargetHashCache:             thc,
			BazelRelease:                bazelRelease,
		}
	}

	beforeMetadata := makeFromCache()
	afterMetadata := makeFromCache()

	err := DiffSingleLabel(
		beforeMetadata, afterMetadata, false, lbl,
		func(_ gazelle_label.Label, diffs []Difference, _ *analysis.ConfiguredTarget) {
			t.Errorf("callback called for unchanged target (diffs when includeDifferences=false are always nil: %v)", diffs)
		},
	)
	if err != nil {
		t.Fatalf("DiffSingleLabel returned unexpected error: %v", err)
	}
}
