package pkg

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	ss "github.com/bazel-contrib/target-determinator/common/sorted_set"
	"github.com/bazel-contrib/target-determinator/common/versions"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/analysis"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/build"
	gazelle_label "github.com/bazelbuild/bazel-gazelle/label"
	"github.com/hashicorp/go-version"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// NewTargetHashCache creates a TargetHashCache which uses context for metadata lookups.
func NewTargetHashCache(
	context map[gazelle_label.Label]map[Configuration]*analysis.ConfiguredTarget,
	normalizer *Normalizer,
	bazelRelease string,
) *TargetHashCache {
	bazelVersionSupportsConfiguredRuleInputs := isConfiguredRuleInputsSupported(bazelRelease)

	return &TargetHashCache{
		context: context,
		fileHashCache: &fileHashCache{
			cache: make(map[string]*cacheEntry),
		},
		normalizer:                               normalizer,
		bazelRelease:                             bazelRelease,
		bazelVersionSupportsConfiguredRuleInputs: bazelVersionSupportsConfiguredRuleInputs,
		cache:                                    make(map[gazelle_label.Label]map[Configuration]*cacheEntry),
		frozen:                                   false,
	}
}

func isConfiguredRuleInputsSupported(releaseString string) bool {
	isSupportedVersion, explanation := versions.ReleaseIsInRange(releaseString, version.Must(version.NewVersion("7.0.0-pre.20230628.2")), nil)
	if isSupportedVersion != nil {
		return *isSupportedVersion
	}
	log.Printf("%s - assuming cquery does not support configured rule inputs (which is supported from bazel 7), which may lead to over-estimates of affected targets", explanation)
	return false
}

// TargetHashCache caches hash computations for targets and files, so that transitive hashes can be
// cheaply computed via dynamic programming.
// Note that a TargetHashCache doesn't eagerly read files, it lazily reads them when they're needed
// for hash computation, so if you're going to mutate filesystem state after creating a
// TargetHashCache (e.g. because you're going to check out a different commit), you should
// pre-compute any hashes you're interested in before mutating the filesystem.
// In the future we may pre-cache file hashes to avoid this hazard (and to allow more efficient
// use of threadpools when hashing files).
type TargetHashCache struct {
	context                                  map[gazelle_label.Label]map[Configuration]*analysis.ConfiguredTarget
	fileHashCache                            *fileHashCache
	bazelRelease                             string
	bazelVersionSupportsConfiguredRuleInputs bool

	normalizer *Normalizer

	frozen bool

	cacheLock sync.Mutex
	cache     map[gazelle_label.Label]map[Configuration]*cacheEntry
}

var labelNotFound = fmt.Errorf("label not found in context")
var notComputedBeforeFrozen = fmt.Errorf("TargetHashCache has already been frozen")

// Hash hashes a given LabelAndConfiguration, returning a sha256 which will change if any of the
// following change:
//   - Values of attributes of the label (if it's a rule)
//   - Contents or mode of source files which are direct inputs to the rule (if it's a rule).
//   - The name of the rule class (e.g. `java_binary`) of the rule (if it's a rule).
//   - The rule definition, if it's a rule which was implemented in starlark.
//     Note that this is known to over-estimate - it currently factors in the whole contents of any
//     .bzl files loaded to define the rule, where some of this contents may not be relevant.
//   - The configuration the label is configured in.
//     Note that this is known to over-estimate - per-language fragments are not filtered from this
//     configuration, which means C++-affecting options are considered to affect Java.
//   - The above recursively for all rules and files which are depended on by the given
//     LabelAndConfiguration.
//     Note that this is known to over-estimate - the configuration of dependencies isn't easily
//     surfaced by Bazel, so if a dependency exists in multiple configurations, all of them will be
//     mixed into the hash, even if only one of the configurations is actually relevant.
//     See https://github.com/bazelbuild/bazel/issues/14610
func (thc *TargetHashCache) Hash(labelAndConfiguration LabelAndConfiguration) ([]byte, error) {
	thc.cacheLock.Lock()
	_, ok := thc.cache[labelAndConfiguration.Label]
	if !ok {
		if thc.frozen {
			thc.cacheLock.Unlock()
			return nil, fmt.Errorf("didn't have cache entry for label %s: %w", labelAndConfiguration.Label, notComputedBeforeFrozen)
		}
		thc.cache[labelAndConfiguration.Label] = make(map[Configuration]*cacheEntry)
	}
	entry, ok := thc.cache[labelAndConfiguration.Label][labelAndConfiguration.Configuration]
	if !ok {
		newEntry := &cacheEntry{}
		thc.cache[labelAndConfiguration.Label][labelAndConfiguration.Configuration] = newEntry
		entry = newEntry
	}
	thc.cacheLock.Unlock()
	entry.hashLock.Lock()
	defer entry.hashLock.Unlock()
	if entry.hash == nil {
		if thc.frozen {
			return nil, fmt.Errorf("didn't have cache value for label %s in configuration %s: %w", labelAndConfiguration.Label, labelAndConfiguration.Configuration, notComputedBeforeFrozen)
		}
		hash, err := hashTarget(thc, labelAndConfiguration)
		if err != nil {
			return nil, err
		}
		entry.hash = hash
	}
	return entry.hash, nil
}

// KnownConfigurations returns the configurations in which a Label is known to be configured.
func (thc *TargetHashCache) KnownConfigurations(label gazelle_label.Label) *ss.SortedSet[Configuration] {
	configurations := ss.NewSortedSetFn([]Configuration{}, ConfigurationLess)
	entry := thc.context[label]
	for c := range entry {
		configurations.Add(c)
	}
	return configurations
}

// Freeze should be called before the filesystem is mutated to signify to the TargetHashCache that
// any future Hash calls which need to read files should fail, because the files may no longer be
// accurate from when the TargetHashCache was created.
func (thc *TargetHashCache) Freeze() {
	thc.frozen = true
}

func (thc *TargetHashCache) ParseCanonicalLabel(label string) (gazelle_label.Label, error) {
	return thc.normalizer.ParseCanonicalLabel(label)
}

// Difference represents a difference of a target between two commits.
// All fields except Category are optional.
type Difference struct {
	// Category is the kind of change, e.g. that the target is new, that a file changed, etc.
	Category string
	// Key is the thing which changed, e.g. the name of an attribute, or the name of the input file.
	Key string
	// Before is the value of Key before the change.
	Before string
	// After is the value of Key after the change.
	After string
}

func (d Difference) String() string {
	s := d.Category
	if d.Key != "" {
		s += "[" + d.Key + "]"
	}
	if d.Before != "" {
		s += " Before: " + d.Before
	}
	if d.After != "" {
		s += " After: " + d.After
	}
	return s
}

// WalkDiffs accumulates the differences of a LabelAndConfiguration before and after a change.
func WalkDiffs(before *TargetHashCache, after *TargetHashCache, labelAndConfiguration LabelAndConfiguration) ([]Difference, error) {
	beforeHash, err := before.Hash(labelAndConfiguration)
	if err != nil {
		return nil, err
	}
	afterHash, err := after.Hash(labelAndConfiguration)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(beforeHash, afterHash) {
		return nil, nil
	}
	var differences []Difference

	if before.bazelRelease != after.bazelRelease {
		differences = append(differences, Difference{
			Category: "BazelVersion",
			Before:   before.bazelRelease,
			After:    after.bazelRelease,
		})
	}

	cBefore, okBefore := before.context[labelAndConfiguration.Label]
	cAfter, okAfter := after.context[labelAndConfiguration.Label]

	if okBefore && !okAfter {
		differences = append(differences, Difference{
			Category: "DeletedTarget",
		})
		return differences, nil
	} else if !okBefore && okAfter {
		differences = append(differences, Difference{
			Category: "AddedTarget",
		})
		return differences, nil
	} else if !okBefore && !okAfter {
		return nil, fmt.Errorf("target %v didn't exist before or after", labelAndConfiguration.Label)
	}

	ctBefore, okBefore := cBefore[labelAndConfiguration.Configuration]
	ctAfter, okAfter := cAfter[labelAndConfiguration.Configuration]
	if !okBefore || !okAfter {
		differences = append(differences, Difference{
			Category: "ChangedConfiguration",
		})
		return differences, nil
	}

	targetBefore := ctBefore.GetTarget()
	targetAfter := ctAfter.GetTarget()

	// Did this target's type change?
	typeBefore := targetBefore.GetType()
	typeAfter := targetAfter.GetType()
	if typeBefore != typeAfter {
		differences = append(differences, Difference{
			Category: "TargetTypeChanged",
			Before:   typeBefore.String(),
			After:    typeAfter.String(),
		})
		return differences, nil
	}

	if typeBefore != build.Target_RULE {
		return differences, nil
	}

	ruleBefore := targetBefore.GetRule()
	ruleAfter := targetAfter.GetRule()
	if ruleBefore.GetRuleClass() != ruleAfter.GetRuleClass() {
		differences = append(differences, Difference{
			Category: "RuleKindChanged",
			Before:   ruleBefore.GetRuleClass(),
			After:    ruleAfter.GetRuleClass(),
		})
	}
	if ruleBefore.GetSkylarkEnvironmentHashCode() != ruleAfter.GetSkylarkEnvironmentHashCode() {
		differences = append(differences, Difference{
			Category: "RuleImplementationChanged",
			Before:   ruleBefore.GetSkylarkEnvironmentHashCode(),
			After:    ruleAfter.GetSkylarkEnvironmentHashCode(),
		})
	}

	attributesBefore := indexAttributes(ruleBefore.GetAttribute())
	attributesAfter := indexAttributes(ruleAfter.GetAttribute())
	sortedAttributeNamesBefore := sortKeys(attributesBefore)
	for _, attributeName := range sortedAttributeNamesBefore {
		attributeBefore := attributesBefore[attributeName]
		attributeAfter, ok := attributesAfter[attributeName]
		if !ok {
			attributeBeforeJson, _ := protojson.Marshal(attributeBefore)
			differences = append(differences, Difference{
				Category: "AttributeRemoved",
				Key:      attributeName,
				Before:   string(attributeBeforeJson),
			})
		} else {
			normalizedBeforeAttribute := before.AttributeForSerialization(attributeBefore)
			normalizedAfterAttribute := after.AttributeForSerialization(attributeAfter)
			if !equivalentAttributes(normalizedBeforeAttribute, normalizedAfterAttribute) {
				if attributeName == "$rule_implementation_hash" {
					differences = append(differences, Difference{
						Category: "RuleImplementedChanged",
					})
				} else {
					attributeBeforeJson, _ := protojson.Marshal(normalizedBeforeAttribute)
					attributeAfterJson, _ := protojson.Marshal(normalizedAfterAttribute)
					differences = append(differences, Difference{
						Category: "AttributeChanged",
						Key:      attributeName,
						Before:   string(attributeBeforeJson),
						After:    string(attributeAfterJson),
					})
				}
			}
		}
	}
	sortedAttributeNamesAfter := sortKeys(attributesAfter)
	for _, attributeName := range sortedAttributeNamesAfter {
		if _, ok := attributesBefore[attributeName]; !ok {
			attributeAfterJson, _ := protojson.Marshal(after.AttributeForSerialization(attributesAfter[attributeName]))
			differences = append(differences, Difference{
				Category: "AttributeAdded",
				Key:      attributeName,
				After:    string(attributeAfterJson),
			})
		}
	}

	ruleInputLabelsAndConfigurationsBefore, err := getConfiguredRuleInputs(before, ruleBefore, labelAndConfiguration.Configuration)
	if err != nil {
		return nil, err
	}
	ruleInputLabelsToConfigurationsBefore := indexByLabel(ruleInputLabelsAndConfigurationsBefore)

	ruleInputLabelsAndConfigurationsAfter, err := getConfiguredRuleInputs(after, ruleAfter, labelAndConfiguration.Configuration)
	if err != nil {
		return nil, err
	}
	ruleInputLabelsToConfigurationsAfter := indexByLabel(ruleInputLabelsAndConfigurationsAfter)

	for _, ruleInputLabelAndConfigurations := range ruleInputLabelsAndConfigurationsAfter {
		ruleInputLabel := ruleInputLabelAndConfigurations.Label
		knownConfigurationsBefore, ok := ruleInputLabelsToConfigurationsBefore[ruleInputLabel]
		if !ok {
			differences = append(differences, Difference{
				Category: "RuleInputAdded",
				Key:      ruleInputLabel.String(),
			})
		} else {
			// Ideally we would know the configuration of each of these ruleInputs from the
			// query information, so we could filter away e.g. host changes when we only have a target dep.
			// Unfortunately, Bazel doesn't currently expose this.
			// See https://github.com/bazelbuild/bazel/issues/14610#issuecomment-1024460141
			knownConfigurationsAfter := ruleInputLabelsToConfigurationsAfter[ruleInputLabel]

			for _, knownConfigurationAfter := range knownConfigurationsAfter.SortedSlice() {
				if knownConfigurationsBefore.Contains(knownConfigurationAfter) {
					hashBefore, err := before.Hash(LabelAndConfiguration{Label: ruleInputLabel, Configuration: knownConfigurationAfter})
					if err != nil {
						return nil, err
					}
					hashAfter, err := after.Hash(LabelAndConfiguration{Label: ruleInputLabel, Configuration: knownConfigurationAfter})
					if err != nil {
						return nil, err
					}
					if !bytes.Equal(hashBefore, hashAfter) {
						differences = append(differences, Difference{
							Category: "RuleInputChanged",
							Key:      formatLabelWithConfiguration(ruleInputLabel, knownConfigurationAfter),
						})
					}
				} else {
					differences = append(differences, Difference{
						Category: "RuleInputChanged",
						Key:      ruleInputLabel.String(),
						After:    fmt.Sprintf("Configuration: %v", knownConfigurationAfter),
					})
				}
			}
			for _, knownConfigurationBefore := range knownConfigurationsBefore.SortedSlice() {
				if !knownConfigurationsAfter.Contains(knownConfigurationBefore) {
					differences = append(differences, Difference{
						Category: "RuleInputChanged",
						Key:      ruleInputLabel.String(),
						Before:   fmt.Sprintf("Configuration: %v", knownConfigurationBefore),
					})
				}
			}
		}
	}
	for _, ruleInputLabelAndConfigurations := range ruleInputLabelsAndConfigurationsBefore {
		ruleInputLabel := ruleInputLabelAndConfigurations.Label
		if _, ok := ruleInputLabelsToConfigurationsAfter[ruleInputLabel]; !ok {
			differences = append(differences, Difference{
				Category: "RuleInputRemoved",
				Key:      ruleInputLabel.String(),
			})
		}
	}

	return differences, nil
}

// AttributeForSerialization redacts details about an attribute which don't affect the output of
// building them, and returns equivalent canonical attribute metadata.
// In particular it redacts:
//   - Whether an attribute was explicitly specified (because the effective value is all that
//     matters).
//   - Any attribute named `generator_location`, because these point to absolute paths for
//     built-in `cc_toolchain_suite` targets such as `@local_config_cc//:toolchain`.
func (thc *TargetHashCache) AttributeForSerialization(rawAttr *build.Attribute) *build.Attribute {
	normalized := *rawAttr
	normalized.ExplicitlySpecified = nil

	// Redact generator_location, which typically contains absolute paths but has no bearing on the
	// functioning of a rule.
	// This is also done in Bazel's internal target hash computation. See:
	// https://github.com/bazelbuild/bazel/blob/6971b016f1e258e3bb567a0f9fe7a88ad565d8f2/src/main/java/com/google/devtools/build/lib/query2/query/output/SyntheticAttributeHashCalculator.java#L78-L81
	if normalized.Name != nil {
		if *normalized.Name == "generator_location" {
			normalized.StringValue = nil
		}
	}

	return thc.normalizer.NormalizeAttribute(&normalized)
}

func equivalentAttributes(left, right *build.Attribute) bool {
	return proto.Equal(left, right)
}

func indexByLabel(labelsAndConfigurations []LabelAndConfigurations) map[gazelle_label.Label]*ss.SortedSet[Configuration] {
	m := make(map[gazelle_label.Label]*ss.SortedSet[Configuration], len(labelsAndConfigurations))
	for _, labelAndConfigurations := range labelsAndConfigurations {
		m[labelAndConfigurations.Label] = ss.NewSortedSetFn(labelAndConfigurations.Configurations, ConfigurationLess)
	}
	return m
}

func formatLabelWithConfiguration(label gazelle_label.Label, configuration Configuration) string {
	s := label.String()
	if configuration.String() != "" {
		s += "[" + configuration.String() + "]"
	}
	return s
}

func indexAttributes(attributes []*build.Attribute) map[string]*build.Attribute {
	m := make(map[string]*build.Attribute, len(attributes))
	for _, attribute := range attributes {
		m[attribute.GetName()] = attribute
	}
	return m
}

func sortKeys(attributes map[string]*build.Attribute) []string {
	keys := make([]string, 0, len(attributes))
	for attribute := range attributes {
		keys = append(keys, attribute)
	}
	sort.Strings(keys)
	return keys
}

func hashTarget(thc *TargetHashCache, labelAndConfiguration LabelAndConfiguration) ([]byte, error) {
	label := labelAndConfiguration.Label
	configurationMap, ok := thc.context[label]
	if !ok {
		return nil, fmt.Errorf("label %s not found in contxt: %w", label, labelNotFound)
	}
	configuration := labelAndConfiguration.Configuration
	configuredTarget, ok := configurationMap[configuration]
	if !ok {
		return nil, fmt.Errorf("label %s configuration %s not found in contxt: %w", label, configuration, labelNotFound)
	}
	target := configuredTarget.Target
	switch target.GetType() {
	case build.Target_SOURCE_FILE:
		absolutePath := AbsolutePath(target)
		hash, err := thc.fileHashCache.Hash(absolutePath)
		if err != nil {
			// Labels may be referred to without existing, and at loading time these are assumed
			// to be input files, even if no such file exists.
			// https://github.com/bazelbuild/bazel/issues/14611
			if os.IsNotExist(err) {
				return make([]byte, 0), nil
			}

			// Directories (spuriously) listed in srcs show up a SOURCE_FILEs.
			// We don't error on this, as Bazel doesn't, but we also don't manually walk the
			// directory (as globs should have been used in the BUILD file if this was the intent).
			// When this gets mixed into other hashes, that mixing in includes the target name, so
			// this sentinel "empty hash" vaguely indicates that a directory occurred.
			// We may want to do something more structured here at some point.
			// See https://github.com/bazelbuild/bazel/issues/14678
			if strings.Contains(err.Error(), "is a directory") {
				return make([]byte, 0), nil
			}
			return nil, fmt.Errorf("failed to hash file %v: %w", absolutePath, err)
		}
		return hash, nil
	case build.Target_RULE:
		return hashRule(thc, target.Rule, configuredTarget.Configuration)
	case build.Target_GENERATED_FILE:
		hasher := sha256.New()
		generatingLabel, err := thc.ParseCanonicalLabel(*target.GeneratedFile.GeneratingRule)
		if err != nil {
			return nil, fmt.Errorf("failed to parse generated file generating rule label %s: %w", *target.GeneratedFile.GeneratingRule, err)
		}
		writeLabel(hasher, generatingLabel)
		hash, err := thc.Hash(LabelAndConfiguration{Label: generatingLabel, Configuration: configuration})
		if err != nil {
			return nil, err
		}
		hasher.Write(hash)
		return hasher.Sum(nil), nil
	case build.Target_PACKAGE_GROUP:
		// Bits of the default local toolchain depend on package groups. We just ignore them.
		return make([]byte, 0), nil
	default:
		return nil, fmt.Errorf("didn't know how to hash target %v with unknown rule type: %v", label, target.GetType())
	}
}

// If this function changes, so should WalkDiffs.
func hashRule(thc *TargetHashCache, rule *build.Rule, configuration *analysis.Configuration) ([]byte, error) {
	hasher := sha256.New()
	// Mix in the Bazel version, because Bazel versions changes may cause differences to how rules
	// are evaluated even if the rules themselves haven't changed.
	hasher.Write([]byte(thc.bazelRelease))
	// Hash own attributes
	hasher.Write([]byte(rule.GetRuleClass()))
	hasher.Write([]byte(rule.GetSkylarkEnvironmentHashCode()))
	hasher.Write([]byte(configuration.GetChecksum()))

	// TODO: Consider using `$internal_attr_hash` from https://github.com/bazelbuild/bazel/blob/6971b016f1e258e3bb567a0f9fe7a88ad565d8f2/src/main/java/com/google/devtools/build/lib/query2/query/output/SyntheticAttributeHashCalculator.java
	// rather than hashing attributes ourselves.
	// On the plus side, this builds in some heuristics from Bazel (e.g. ignoring `generator_location`).
	// On the down side, it would even further decouple our "hashing" and "diffing" procedures.
	for _, attr := range rule.GetAttribute() {
		normalizedAttribute := thc.AttributeForSerialization(attr)

		protoBytes, err := proto.Marshal(normalizedAttribute)
		if err != nil {
			return nil, err
		}

		hasher.Write(protoBytes)
	}

	ownConfiguration := NormalizeConfiguration(configuration.GetChecksum())

	// Hash rule inputs
	labelsAndConfigurations, err := getConfiguredRuleInputs(thc, rule, ownConfiguration)
	if err != nil {
		return nil, err
	}
	for _, ruleInputLabelAndConfigurations := range labelsAndConfigurations {
		for _, ruleInputConfiguration := range ruleInputLabelAndConfigurations.Configurations {
			ruleInputLabel := ruleInputLabelAndConfigurations.Label
			ruleInputHash, err := thc.Hash(LabelAndConfiguration{Label: ruleInputLabel, Configuration: ruleInputConfiguration})
			if err != nil {
				return nil, fmt.Errorf("failed to hash configuredRuleInput %s %s which is a dependency of %s %s: %w", ruleInputLabel, ruleInputConfiguration, rule.GetName(), configuration.GetChecksum(), err)
			}

			writeLabel(hasher, ruleInputLabel)
			hasher.Write(ruleInputConfiguration.ForHashing())
			hasher.Write(ruleInputHash)
		}
	}

	return hasher.Sum(nil), nil
}

func getConfiguredRuleInputs(thc *TargetHashCache, rule *build.Rule, ownConfiguration Configuration) ([]LabelAndConfigurations, error) {
	labelsAndConfigurations := make([]LabelAndConfigurations, 0)
	if thc.bazelVersionSupportsConfiguredRuleInputs {
		for _, configuredRuleInput := range rule.ConfiguredRuleInput {
			ruleInputLabel, err := thc.ParseCanonicalLabel(configuredRuleInput.GetLabel())
			if err != nil {
				return nil, fmt.Errorf("failed to parse configuredRuleInput label %s: %w", configuredRuleInput.GetLabel(), err)
			}
			ruleInputConfiguration := NormalizeConfiguration(configuredRuleInput.GetConfigurationChecksum())
			if ruleInputConfiguration.String() == "" {
				// Configured Rule Inputs which aren't transitioned end up with an empty string as their configuration.
				// This _either_ means there was no transition, _or_ means that the input was a source file (so didn't have a configuration at all).
				// Fortunately, these are mutually exclusive - a target either has a configuration or doesn't, so we look up which one exists.
				if _, ok := thc.context[ruleInputLabel][ownConfiguration]; ok {
					ruleInputConfiguration = ownConfiguration
				} else if _, ok := thc.context[ruleInputLabel][ruleInputConfiguration]; !ok {
					return nil, fmt.Errorf("configuredRuleInputs for %s included %s in configuration %s but it couldn't be found either unconfigured or in the depending target's configuration %s. This probably indicates a bug in Bazel - please report it with a git repo that reproduces at https://github.com/bazel-contrib/target-determinator/issues so we can investigate", rule.GetName(), ruleInputLabel, ruleInputConfiguration, ownConfiguration)
				}
			}
			labelsAndConfigurations = append(labelsAndConfigurations, LabelAndConfigurations{
				Label:          ruleInputLabel,
				Configurations: []Configuration{ruleInputConfiguration},
			})
		}
	} else {
		for _, ruleInputLabelString := range rule.RuleInput {
			ruleInputLabel, err := thc.ParseCanonicalLabel(ruleInputLabelString)
			if err != nil {
				return nil, fmt.Errorf("failed to parse ruleInput label %s: %w", ruleInputLabelString, err)
			}
			labelAndConfigurations := LabelAndConfigurations{
				Label: ruleInputLabel,
			}
			var depConfigurations []Configuration
			// Aliases don't transition, and we've seen aliases expanding across configurations cause dependency cycles for nogo targets.
			if rule.GetRuleClass() == "alias" {
				knownDepConfigurations := thc.context[ruleInputLabel]
				isSourceFile := true
				for _, ct := range knownDepConfigurations {
					if ct.GetTarget().GetType() != build.Target_SOURCE_FILE {
						isSourceFile = false
					}
				}

				if isSourceFile {
					// If it's a source file, it doesn't exist in the current configuration, but does exist in the empty configuration.
					// Accordingly, we need to explicitly transition to the empty configuration.
					depConfigurations = []Configuration{NormalizeConfiguration("")}
				} else {
					// If it's not a source file, narrow just to the current configuration - we know there was no transition, so we must be in the same configuration.
					depConfigurations = []Configuration{ownConfiguration}
				}
			} else {
				depConfigurations = thc.KnownConfigurations(ruleInputLabel).SortedSlice()
			}
			for _, configuration := range depConfigurations {
				if _, err := thc.Hash(LabelAndConfiguration{Label: ruleInputLabel, Configuration: configuration}); err != nil {
					if errors.Is(err, labelNotFound) {
						// Two issues (so far) have been found which lead to targets being listed in
						// ruleInputs but not in the output of a deps query:
						//
						// cquery doesn't filter ruleInputs according to used configurations, which means
						// targets may appear in a Target's ruleInputs even though they weren't returned by
						// a transitive `deps` cquery.
						// Assume that a missing target should have been pruned, and that we should ignore it.
						// See https://github.com/bazelbuild/bazel/issues/14610
						//
						// Some targets are also just sometimes missing for reasons we don't yet know.
						// See https://github.com/bazelbuild/bazel/issues/14617
						continue
					}
					return nil, err
				}
				labelAndConfigurations.Configurations = append(labelAndConfigurations.Configurations, configuration)
			}
			labelsAndConfigurations = append(labelsAndConfigurations, labelAndConfigurations)
		}
	}
	return labelsAndConfigurations, nil
}

type fileHashCache struct {
	cacheLock sync.Mutex
	cache     map[string]*cacheEntry
}

type cacheEntry struct {
	hashLock sync.Mutex
	hash     []byte
}

// Hash computes the digest of the contents of a file at the given path, and caches the result.
func (hc *fileHashCache) Hash(path string) ([]byte, error) {
	hc.cacheLock.Lock()
	entry, ok := hc.cache[path]
	if !ok {
		newEntry := &cacheEntry{}
		hc.cache[path] = newEntry
		entry = newEntry
	}
	hc.cacheLock.Unlock()
	entry.hashLock.Lock()
	defer entry.hashLock.Unlock()
	if entry.hash == nil {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		hasher := sha256.New()

		// Hash the file mode.
		// This is used to detect change such as file exec bit changing.
		info, err := file.Stat()
		if err != nil {
			return nil, err
		}

		// Only record the user permissions, and only the execute bit:
		// - group and others permissions differences don't affect the build and are not tracked by git. This means that
		//   a file created as 0775 by a script and then added to git might show up as 0755 when performing a
		//  `git clone` or a `git checkout`. This can cause issues when TD uses a git worktree for the `before` case.
		// - bazel and git don't care if a file is writeable, and the hashing below will fail if the file isn't readable
		//   anyway.
		userExecPerm := getUserExecuteBit(info.Mode())
		if _, err := fmt.Fprintf(hasher, userExecPerm.String()); err != nil {
			return nil, err
		}

		// Hash the content of the file
		if _, err := io.Copy(hasher, file); err != nil {
			return nil, err
		}
		entry.hash = hasher.Sum(nil)
	}
	return entry.hash, nil
}

func getUserExecuteBit(info os.FileMode) os.FileMode {
	var userPermMask os.FileMode = 0100
	return info & userPermMask
}

// Swallows errors, because assumes you're writing to an infallible Writer like a hasher.
func writeLabel(w io.Writer, label gazelle_label.Label) {
	labelStr := label.String()
	binary.Write(w, binary.LittleEndian, len(labelStr))
	w.Write([]byte(labelStr))
}

// AbsolutePath returns the absolute path to the source file Target.
// It assumes the passed Target is of type Source File.
func AbsolutePath(target *build.Target) string {
	colonIndex := strings.IndexByte(target.GetSourceFile().GetLocation(), ':')
	location := target.GetSourceFile().GetLocation()
	// Before Bazel 5, BUILD.bazel files would not have line/column data in their location fields.
	if colonIndex >= 0 {
		location = location[:colonIndex]
	}
	locationBase := filepath.Base(location)

	// Bazel before 5.0.0 (or with incompatible_display_source_file_location disabled) reported
	// source files as having a location relative to their BUILD file.
	// After, location simply refers to the actual location of the file.
	// Sniff for the former case, and perform the processing required to handle it.
	if locationBase == "BUILD" || locationBase == "BUILD.bazel" {
		location = filepath.Dir(location)
		name := target.GetSourceFile().GetName()[strings.LastIndexByte(target.GetSourceFile().GetName(), ':')+1:]
		return filepath.Join(location, name)
	}
	return location
}
