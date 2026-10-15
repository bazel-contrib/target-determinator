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
		thc := NewTargetHashCache(nil, &Normalizer{}, bazelRelease)
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

func TestDiffSingleLabel_AddedConfiguration(t *testing.T) {
	lbl := mustParseLabel("//foo:bar")
	target, exec := NormalizeConfiguration("aaaa"), NormalizeConfiguration("bbbb")
	same, changed := []byte{1}, []byte{2}

	fromCache := func(hashes map[Configuration][]byte) *QueryResults {
		var configurations []Configuration
		restored := map[string][]byte{}
		for configuration, hash := range hashes {
			configurations = append(configurations, configuration)
			restored[lbl.String()+"\x00"+configuration.String()] = hash
		}
		thc := NewTargetHashCache(nil, &Normalizer{}, "release 7.0.0")
		if err := thc.RestoreHashes(restored); err != nil {
			t.Fatalf("RestoreHashes: %v", err)
		}
		return &QueryResults{
			MatchingTargets: &MatchingTargets{
				labels:                 ss.NewSortedSetFn([]gazelle_label.Label{lbl}, CompareLabels),
				labelsToConfigurations: map[gazelle_label.Label]*ss.SortedSet[Configuration]{lbl: ss.NewSortedSetFn(configurations, ConfigurationLess)},
			},
			TargetHashCache: thc,
			BazelRelease:    "release 7.0.0",
		}
	}

	for name, tc := range map[string]struct {
		before, after map[Configuration][]byte
		want          bool
	}{
		"added configuration of an unchanged target": {map[Configuration][]byte{target: same}, map[Configuration][]byte{target: same, exec: same}, false},
		"added configuration of a changed target":    {map[Configuration][]byte{target: same}, map[Configuration][]byte{target: changed, exec: same}, true},
		"replaced configuration":                     {map[Configuration][]byte{target: same}, map[Configuration][]byte{exec: same}, true},
	} {
		affected := false
		if err := DiffSingleLabel(fromCache(tc.before), fromCache(tc.after), false, lbl,
			func(gazelle_label.Label, []Difference, *analysis.ConfiguredTarget) { affected = true }); err != nil {
			t.Fatalf("%s: DiffSingleLabel: %v", name, err)
		}
		if affected != tc.want {
			t.Errorf("%s: affected = %v, want %v", name, affected, tc.want)
		}
	}
}
