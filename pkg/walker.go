package pkg

import (
	"bytes"
	"fmt"
	"log"

	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/analysis"
	"github.com/bazelbuild/bazel-gazelle/label"
)

type WalkCallback func(label.Label, []Difference, *analysis.ConfiguredTarget)

// WalkAffectedTargets computes which targets have changed between two commits, and calls
// callback once for each target which has changed.
// Explanation of the differences may be expensive in both time and memory to compute, so if
// includeDifferences is set to false, the []Difference parameter to the callback will always be nil.
func WalkAffectedTargets(context *Context, revBefore LabelledGitRev, targets TargetsList, includeDifferences bool, callback WalkCallback) error {
	// The revAfter revision represents the current state of the working directory, which may contain local changes.
	// It is distinct from context.OriginalRevision, which represents the original commit that we want to reset to before exiting.
	revAfter, err := NewLabelledGitRev(context.WorkspacePath, "", "after")
	if err != nil {
		return fmt.Errorf("could not create \"after\" revision: %w", err)
	}

	beforeMetadata, afterMetadata, err := FullyProcess(context, revBefore, revAfter, targets)
	if err != nil {
		return fmt.Errorf("failed to process change: %w", err)
	}

	if beforeMetadata.BazelRelease == afterMetadata.BazelRelease && beforeMetadata.BazelRelease == "development version" {
		log.Printf("WARN: Bazel was detected to be a development version - if you're using different development versions at the before and after commits, differences between those versions may not be reflected in this output")
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
			category := "NewLabel"
			if beforeMetadata.QueryError != nil {
				category = "ErrorInQueryBefore"
			}
			collectDifference(Difference{
				Category: category,
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
