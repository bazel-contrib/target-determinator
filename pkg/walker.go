package pkg

import (
	"bytes"
	"fmt"

	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/analysis"
	"github.com/bazelbuild/bazel-gazelle/label"
)

type WalkCallback func(label.Label, []Difference, *analysis.ConfiguredTarget)

// WalkAffectedTargets computes which targets have changed between two commits, and calls
// callback once for each target which has changed.
// Explanation of the differences may be expensive in both time and memory to compute, so if
// includeDifferences is set to false, the []Difference parameter to the callback will always be nil.
func WalkAffectedTargets(context *Context, revBefore LabelledGitRev, pattern label.Pattern, includeDifferences bool, callback WalkCallback) error {
	beforeMetadata, afterMetadata, err := FullyProcess(context, revBefore, pattern)
	if err != nil {
		return fmt.Errorf("failed to process change: %w", err)
	}

	for _, l := range afterMetadata.MatchingTargets.Labels() {
		if err := DiffSingleLabel(beforeMetadata, afterMetadata, includeDifferences, l, callback); err != nil {
			return err
		}
	}

	return nil
}

func DiffSingleLabel(beforeMetadata, afterMetadata *QueryResults, includeDifferences bool, label label.Label, callback WalkCallback) error {
	for _, configuration := range afterMetadata.MatchingTargets.ConfigurationsFor(label) {
		configuredTarget := afterMetadata.TransitiveConfiguredTargets[label][configuration]

		var differences []Difference

		collectDifference := func(d Difference) {
			if includeDifferences {
				differences = append(differences, d)
			}
		}

		if len(beforeMetadata.MatchingTargets.ConfigurationsFor(label)) == 0 {
			collectDifference(Difference{
				Category: "NewLabel",
			})
			callback(label, differences, configuredTarget)
			return nil
		} else if !beforeMetadata.MatchingTargets.ContainsLabelAndConfiguration(label, configuration) {
			collectDifference(Difference{
				Category: "NewConfiguration",
			})
			callback(label, differences, configuredTarget)
			return nil
		}
		_, ok := beforeMetadata.TransitiveConfiguredTargets[label][configuration]
		if !ok {
			collectDifference(Difference{Category: "NewTarget"})
			callback(label, differences, configuredTarget)
			return nil
		} else {
			labelAndConfiguration := LabelAndConfiguration{
				Label:         label,
				Configuration: configuration,
			}
			hashBefore, err := beforeMetadata.TargetHashCache.Hash(labelAndConfiguration)
			if err != nil {
				return err
			}
			hashAfter, err := afterMetadata.TargetHashCache.Hash(labelAndConfiguration)
			if err != nil {
				return err
			}
			if bytes.Equal(hashBefore, hashAfter) {
				continue
			}
			if includeDifferences {
				differences, err = WalkDiffs(beforeMetadata.TargetHashCache, afterMetadata.TargetHashCache, labelAndConfiguration)
				if err != nil {
					return err
				}
			}
			callback(label, differences, configuredTarget)
		}
	}
	return nil
}
