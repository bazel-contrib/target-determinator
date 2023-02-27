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
		if err := diffSingleLabel(context, beforeMetadata, afterMetadata, includeDifferences, l, callback); err != nil {
			return err
		}
	}

	return nil
}

func diffSingleLabel(context *Context, beforeMetadata, afterMetadata *QueryResults, includeDifferences bool, label label.Label, callback WalkCallback) error {
	for _, configuration := range afterMetadata.MatchingTargets.ConfigurationsFor(label) {
		if !hasConfiguredTarget(context, label, configuration) {
			continue
		}
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
			difference := Difference{
				Category: "NewConfiguration",
			}
			if includeDifferences {
				configurationsBefore := beforeMetadata.MatchingTargets.ConfigurationsFor(label)
				configurationsAfter := afterMetadata.MatchingTargets.ConfigurationsFor(label)
				if len(configurationsBefore) == 1 && len(configurationsAfter) == 1 {
					diff, _ := diffConfigurations(beforeMetadata.configurations[configurationsBefore[0]], afterMetadata.configurations[configurationsAfter[0]])
					difference = Difference{
						Category: "ChangedConfiguration",
						Before:   string(configurationsBefore[0]),
						After:    string(configurationsAfter[0]),
						Key:      diff,
					}
				}
			}
			collectDifference(difference)
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

func hasConfiguredTarget(context *Context, label label.Label, configuration Configuration) bool {
	// Calling "bazel cquery config(<label>, configuration)" attempts to find the
	// configured target for the label. If no results can be found, the query fails:
	// https://bazel.build/query/cquery#config
	_, err := context.BazelCmd.Execute(
		BazelCmdConfig{Dir: context.WorkspacePath},
		[]string{"--output_base", context.BazelOutputBase}, "cquery", fmt.Sprintf("config(%s,%s)", label, configuration))
	return err == nil
}
