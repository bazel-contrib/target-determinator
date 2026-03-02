package pkg

import (
	"fmt"
	"reflect"
	"testing"

	ss "github.com/bazel-contrib/target-determinator/common/sorted_set"
	"github.com/bazelbuild/bazel-gazelle/label"
)

func TestSerializeDeserializeMatchingTargets(t *testing.T) {
	labels := []label.Label{mustParseLabel("//foo:bar"), mustParseLabel("//baz:qux")}
	config := NormalizeConfiguration("abc123")
	mt := &MatchingTargets{
		labels: ss.NewSortedSetFn(labels, CompareLabels),
		labelsToConfigurations: map[label.Label]*ss.SortedSet[Configuration]{
			labels[0]: ss.NewSortedSetFn([]Configuration{config}, ConfigurationLess),
			labels[1]: ss.NewSortedSetFn([]Configuration{}, ConfigurationLess),
		},
	}

	data, err := serializeMatchingTargets(mt)
	if err != nil {
		t.Fatalf("serializeMatchingTargets failed: %v", err)
	}

	got, err := deserializeMatchingTargets(data)
	if err != nil {
		t.Fatalf("deserializeMatchingTargets failed: %v", err)
	}

	if !reflect.DeepEqual(mt.Labels(), got.Labels()) {
		t.Errorf("labels mismatch: want %v, got %v", mt.Labels(), got.Labels())
	}

	if !got.ContainsLabelAndConfiguration(labels[0], config) {
		t.Errorf("expected %v to contain configuration %v", labels[0], config)
	}

	if got.ContainsLabelAndConfiguration(labels[1], config) {
		t.Errorf("expected %v to not contain any configuration", labels[1])
	}

	if len(got.ConfigurationsFor(labels[1])) != 0 {
		t.Errorf("expected empty configurations for %v, got %v", labels[1], got.ConfigurationsFor(labels[1]))
	}
}

// fakeBazelCmd implements BazelCmd and returns a fixed release string for "bazel info release".
type fakeBazelCmd struct {
	release string
	hashKey string
}

func (f fakeBazelCmd) Execute(config BazelCmdConfig, startupArgs []string, command string, args ...string) (int, error) {
	if command == "info" && len(args) > 0 && args[0] == "release" {
		if config.Stdout != nil {
			fmt.Fprintf(config.Stdout, "%s\n", f.release)
		}
		return 0, nil
	}
	return 1, fmt.Errorf("unexpected bazel command: %s %v", command, args)
}

func (f fakeBazelCmd) Cquery(_ string, _ BazelCmdConfig, _ []string, _ ...string) (int, error) {
	return 1, fmt.Errorf("cquery not expected in this test")
}

func (f fakeBazelCmd) HashKey() string { return f.hashKey }

func TestSaveToCacheLoadFromCache(t *testing.T) {
	const bazelRelease = "release 7.0.0"

	ctx := &Context{
		CacheDirectory:            t.TempDir(),
		WorkspacePath:             t.TempDir(),
		BazelCmd:                  fakeBazelCmd{release: bazelRelease},
		FilterIncompatibleTargets: true,
	}

	lbl := mustParseLabel("//foo:bar")
	config := NormalizeConfiguration("deadcafe")

	mt := &MatchingTargets{
		labels: ss.NewSortedSetFn([]label.Label{lbl}, CompareLabels),
		labelsToConfigurations: map[label.Label]*ss.SortedSet[Configuration]{
			lbl: ss.NewSortedSetFn([]Configuration{config}, ConfigurationLess),
		},
	}

	qr := &QueryResults{
		MatchingTargets: mt,
		BazelRelease:    bazelRelease,
		TargetHashCache: NewTargetHashCache(nil, &Normalizer{}, bazelRelease),
	}

	if err := SaveToCache(ctx, "deadcafe", "//...", qr); err != nil {
		t.Fatalf("SaveToCache failed: %v", err)
	}

	loaded, err := LoadFromCache(ctx, "deadcafe", "//...")
	if err != nil {
		t.Fatalf("LoadFromCache failed: %v", err)
	}

	if !loaded.MatchingTargets.ContainsLabelAndConfiguration(lbl, config) {
		t.Errorf("loaded MatchingTargets does not contain expected label+config (%v, %v)", lbl, config)
	}

	if loaded.BazelRelease != qr.BazelRelease {
		t.Errorf("BazelRelease mismatch: want %q, got %q", qr.BazelRelease, loaded.BazelRelease)
	}

	// Changing FilterIncompatibleTargets must produce a different cache key (cache miss).
	ctxChanged := *ctx
	ctxChanged.FilterIncompatibleTargets = false
	if _, err := LoadFromCache(&ctxChanged, "deadcafe", "//..."); err == nil {
		t.Error("expected cache miss when FilterIncompatibleTargets changes, but got a hit")
	}

	// Changing BazelCmd.HashKey() must produce a different cache key (cache miss).
	ctxDiffCmd := *ctx
	ctxDiffCmd.BazelCmd = fakeBazelCmd{release: bazelRelease, hashKey: "different-hash"}
	if _, err := LoadFromCache(&ctxDiffCmd, "deadcafe", "//..."); err == nil {
		t.Error("expected cache miss when BazelCmd hash changes, but got a hit")
	}
}

func TestCollectAffectsCacheFields(t *testing.T) {
	t.Run("Context collects FilterIncompatibleTargets", func(t *testing.T) {
		ctx := &Context{FilterIncompatibleTargets: true}
		got := collectAffectsCacheFields(ctx)
		if got["FilterIncompatibleTargets"] != true {
			t.Errorf("FilterIncompatibleTargets = %v, want true", got["FilterIncompatibleTargets"])
		}
	})
}

func TestDefaultBazelCmdHashKey(t *testing.T) {
	baseline := DefaultBazelCmd{}.HashKey()

	t.Run("differs when BazelStartupOpts changes", func(t *testing.T) {
		h := DefaultBazelCmd{BazelStartupOpts: []string{"--opt"}}.HashKey()
		if h == baseline {
			t.Error("expected different hash when BazelStartupOpts changes")
		}
	})

	t.Run("differs when BazelOpts changes", func(t *testing.T) {
		h := DefaultBazelCmd{BazelOpts: []string{"--opt"}}.HashKey()
		if h == baseline {
			t.Error("expected different hash when BazelOpts changes")
		}
	})

	t.Run("BazelOpts order is significant", func(t *testing.T) {
		h1 := DefaultBazelCmd{BazelOpts: []string{"--z_first", "--a_second"}}.HashKey()
		h2 := DefaultBazelCmd{BazelOpts: []string{"--a_second", "--z_first"}}.HashKey()
		if h1 == h2 {
			t.Error("expected different hashes for different BazelOpts orderings")
		}
	})

	t.Run("BazelPath is not included", func(t *testing.T) {
		h := DefaultBazelCmd{BazelPath: "/custom/bazel"}.HashKey()
		if h != baseline {
			t.Error("BazelPath should not affect the hash key")
		}
	})
}
