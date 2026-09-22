package pkg

import (
	"encoding/hex"
	"path/filepath"
	"sort"
	"strings"
)

// DirtySetResult contains the computed dirty set and metadata.
type DirtySetResult struct {
	// DirtyLabels is the set of target labels directly affected by changes.
	DirtyLabels map[string]bool
	// DirtyStarLabels is DirtyLabels plus all transitive reverse deps.
	DirtyStarLabels map[string]bool
	// DirtyPackages is the sorted list of packages containing changed files
	// (source or BUILD). Targets in these packages must be re-listed with a
	// package wildcard (e.g. //pkg:all) rather than by explicit label, so
	// that targets added to or removed from the package are handled.
	DirtyPackages []string
	// NeedsFallback is true when a change requires a full rehash.
	NeedsFallback bool
	// FallbackReason describes why a fallback was triggered.
	FallbackReason string
	// FallbackCode is a stable, bounded identifier suitable for reporting and
	// metrics. FallbackReason remains the human-readable detail.
	FallbackCode string
}

// ComputeDirtySet determines which targets need rehashing based on changed
// files and the persisted edge map.
//
// changedFiles maps file paths (relative to workspace root) to their git
// diff status code (M, A, D, R, etc.).
// edges maps target labels to their direct dependency labels (from the seed
// file's TargetEdges).
// allLabels is the complete set of labels known to the seed file (union of
// TargetEdges keys and values, plus TargetHashes keys). Used both to find
// labels in dirty packages and to derive the set of known packages.
// ruleClassFingerprintFiles is the set of workspace-relative paths used for
// rule-class fingerprints; a change to any of these triggers a fallback.
//
// A changed source file is attributed to its owning package by walking up
// the directory tree to the nearest package known to the seed. This mirrors
// how Bazel assigns files to packages: globs cannot cross package
// boundaries, and a label //pkg:path/file requires pkg to be a package, so
// the nearest enclosing package is the only one whose targets can reference
// the file. Files with no enclosing known package cannot be inputs to any
// seeded target and are ignored.
//
// Fallback (full rehash) is triggered by files that can change loading,
// repository resolution, or package boundaries without appearing in target
// edges: Starlark, module/workspace/repository metadata, Bazel configuration,
// rule-class fingerprint files, deleted or renamed BUILD files, and BUILD
// files for packages unknown to the seed.
func ComputeDirtySet(
	changedFiles map[string]string,
	edges map[string][]string,
	allLabels map[string]bool,
	ruleClassFingerprintFiles map[string]bool,
) *DirtySetResult {
	result := &DirtySetResult{
		DirtyLabels:     make(map[string]bool),
		DirtyStarLabels: make(map[string]bool),
	}

	knownPackages := make(map[string]bool)
	labelsByPackage := make(map[string][]string)
	for label := range allLabels {
		// Changed files are always in the main workspace. External labels can
		// share the same package path (including the root package) after
		// labelToPackage strips their repository prefix, but they must not be
		// selected directly by a workspace package change. Keep them in edges
		// so reverse-dependency propagation can still traverse them.
		if !strings.HasPrefix(label, "//") {
			continue
		}
		pkg := labelToPackage(label)
		knownPackages[pkg] = true
		labelsByPackage[pkg] = append(labelsByPackage[pkg], label)
	}

	// First pass: fallback triggers.
	for filePath, status := range changedFiles {
		base := filepath.Base(filePath)

		if ruleClassFingerprintFiles[filePath] {
			result.NeedsFallback = true
			result.FallbackCode = "rule_fingerprint_change"
			result.FallbackReason = "rule-class fingerprint file changed: " + filePath
			return result
		}

		if isFallbackTrigger(base) {
			result.NeedsFallback = true
			result.FallbackCode = "unsafe_file_change"
			result.FallbackReason = "fallback trigger file changed: " + filePath
			return result
		}

		if base == "BUILD" || base == "BUILD.bazel" {
			if status == "D" || strings.HasPrefix(status, "R") {
				result.NeedsFallback = true
				result.FallbackCode = "package_boundary_change"
				result.FallbackReason = "package deleted or renamed: " + filePath
				return result
			}
			if !knownPackages[fileToPackage(filePath)] {
				result.NeedsFallback = true
				result.FallbackCode = "package_boundary_change"
				result.FallbackReason = "BUILD file for package unknown to seed: " + filePath
				return result
			}
		}
	}

	// Second pass: map changed files to dirty packages and labels.
	dirtyPackages := make(map[string]bool)
	for filePath := range changedFiles {
		base := filepath.Base(filePath)

		var pkg string
		if base == "BUILD" || base == "BUILD.bazel" {
			// A BUILD file change dirties its own package (verified known above).
			pkg = fileToPackage(filePath)
		} else {
			// A source file belongs to the nearest enclosing known package.
			owner, ok := owningPackage(filePath, knownPackages)
			if !ok {
				// No enclosing package: the file cannot be an input to any
				// seeded target (see function comment).
				continue
			}
			pkg = owner
		}

		if !dirtyPackages[pkg] {
			dirtyPackages[pkg] = true
			for _, label := range labelsByPackage[pkg] {
				result.DirtyLabels[label] = true
			}
		}
	}

	for pkg := range dirtyPackages {
		result.DirtyPackages = append(result.DirtyPackages, pkg)
	}
	sort.Strings(result.DirtyPackages)

	// Propagate dirtiness through reverse deps.
	rdeps := BuildRdeps(edges)

	queue := make([]string, 0, len(result.DirtyLabels))
	for label := range result.DirtyLabels {
		result.DirtyStarLabels[label] = true
		queue = append(queue, label)
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, rdep := range rdeps[current] {
			if !result.DirtyStarLabels[rdep] {
				result.DirtyStarLabels[rdep] = true
				queue = append(queue, rdep)
			}
		}
	}

	return result
}

// PruneDirtySet narrows a DirtySetResult by re-propagating reverse
// dependencies only from targets whose hashes actually changed. The caller
// must supply probeHashes — a map from "label\x00configuration" to the hex
// hash computed at the destination revision — for every target in the
// directly dirty packages. Targets whose probe hash matches the seed are
// excluded from rdeps propagation, dramatically reducing the dirty set when
// a BUILD.bazel change doesn't affect most existing targets (e.g. adding a
// new target to a high-fanout package).
//
// seedHashes is the TargetHashes map from the seed file (label → config → hex hash).
// edges is the TargetEdges map from the seed file.
// original is the DirtySetResult from ComputeDirtySet.
// probeHashes maps "label\x00config" → hex hash for targets in dirty packages.
//
// PruneDirtySet preserves the original DirtyLabels and DirtyPackages
// (packages still need re-listing via wildcards) but recomputes
// DirtyStarLabels from scratch.
func PruneDirtySet(
	original *DirtySetResult,
	seedHashes map[string]map[string]string,
	edges map[string][]string,
	probeHashes map[string]string,
) *DirtySetResult {
	// Identify directly dirty targets whose hash actually changed.
	actuallyChanged := make(map[string]bool)
	for label := range original.DirtyLabels {
		seedConfigs, inSeed := seedHashes[label]
		if !inSeed {
			// New target (not in seed) — always counts as changed.
			actuallyChanged[label] = true
			continue
		}
		matched := true
		for config, seedHex := range seedConfigs {
			probeKey := label + "\x00" + config
			probeHex, ok := probeHashes[probeKey]
			if !ok {
				// Target was in seed but not in probe results — may have been
				// removed or filtered. Treat as changed to be safe.
				matched = false
				break
			}
			if probeHex != seedHex {
				matched = false
				break
			}
		}
		if !matched {
			actuallyChanged[label] = true
		}
	}

	// Re-propagate rdeps only from actually changed targets.
	rdeps := BuildRdeps(edges)
	newDirtyStar := make(map[string]bool, len(actuallyChanged))
	queue := make([]string, 0, len(actuallyChanged))

	// Seed the queue with all directly dirty labels (they still need
	// re-listing even if their hash didn't change), but only propagate
	// rdeps from labels that actually changed.
	for label := range original.DirtyLabels {
		newDirtyStar[label] = true
	}
	for label := range actuallyChanged {
		queue = append(queue, label)
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, rdep := range rdeps[current] {
			if !newDirtyStar[rdep] {
				newDirtyStar[rdep] = true
				queue = append(queue, rdep)
			}
		}
	}

	return &DirtySetResult{
		DirtyLabels:     original.DirtyLabels,
		DirtyStarLabels: newDirtyStar,
		DirtyPackages:   original.DirtyPackages,
	}
}

// ProbeHashesFromQueryResults extracts hex-encoded hashes for all matching
// targets in a QueryResults, keyed as "label\x00configuration". This is
// used by the probe phase to compare against seed hashes.
func ProbeHashesFromQueryResults(queryResults *QueryResults) (map[string]string, error) {
	hashes := make(map[string]string)
	for _, label := range queryResults.MatchingTargets.Labels() {
		for _, cfg := range queryResults.MatchingTargets.ConfigurationsFor(label) {
			hash, err := queryResults.TargetHashCache.Hash(LabelAndConfiguration{
				Label: label, Configuration: cfg,
			})
			if err != nil {
				return nil, err
			}
			hashes[label.String()+"\x00"+cfg.String()] = hex.EncodeToString(hash)
		}
	}
	return hashes, nil
}

func isFallbackTrigger(basename string) bool {
	if strings.HasSuffix(basename, ".bzl") {
		return true
	}
	lowerBasename := strings.ToLower(basename)
	if strings.HasSuffix(lowerBasename, ".lock") ||
		strings.Contains(lowerBasename, "-lock.") ||
		strings.Contains(lowerBasename, "_lock.") {
		return true
	}
	if basename == ".bazelrc" || strings.HasPrefix(basename, ".bazelrc.") ||
		strings.HasSuffix(basename, ".bazelrc") || strings.HasSuffix(basename, ".rc") {
		return true
	}
	switch basename {
	case "MODULE.bazel", "MODULE.bazel.lock",
		"WORKSPACE", "WORKSPACE.bazel", "WORKSPACE.bzlmod",
		"REPO.bazel", "VENDOR.bazel",
		"maven_install.json",
		".bazelversion", ".bazelignore", ".gitmodules":
		return true
	}
	if strings.HasSuffix(basename, ".MODULE.bazel") {
		return true
	}
	return false
}

func fileToPackage(filePath string) string {
	dir := filepath.Dir(filePath)
	if dir == "." {
		return "//"
	}
	return "//" + filepath.ToSlash(dir)
}

// owningPackage walks up the directory tree from filePath and returns the
// nearest enclosing package present in knownPackages.
func owningPackage(filePath string, knownPackages map[string]bool) (string, bool) {
	dir := filepath.ToSlash(filepath.Dir(filePath))
	for {
		var pkg string
		if dir == "." || dir == "/" || dir == "" {
			pkg = "//"
		} else {
			pkg = "//" + dir
		}
		if knownPackages[pkg] {
			return pkg, true
		}
		if pkg == "//" {
			return "", false
		}
		parent := filepath.ToSlash(filepath.Dir(dir))
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// LabelPackage returns the package portion of a label, e.g. "//foo/bar" for
// "//foo/bar:baz". Repository prefixes are stripped.
func LabelPackage(label string) string {
	return labelToPackage(label)
}

func labelToPackage(label string) string {
	if idx := strings.Index(label, "//"); idx >= 0 {
		label = label[idx:]
	}
	if idx := strings.IndexByte(label, ':'); idx >= 0 {
		return label[:idx]
	}
	return label
}

// BuildRdeps constructs a reverse-dependency map from an edge map.
func BuildRdeps(edges map[string][]string) map[string][]string {
	rdeps := make(map[string][]string)
	for label, deps := range edges {
		for _, dep := range deps {
			rdeps[dep] = append(rdeps[dep], label)
		}
	}
	for dep := range rdeps {
		sort.Strings(rdeps[dep])
	}
	return rdeps
}

// CollectAllLabels builds the complete set of labels from the edge map
// (both keys and values) and the hash map keys.
func CollectAllLabels(edges map[string][]string, hashLabels map[string]map[string]string) map[string]bool {
	all := make(map[string]bool)
	for lbl := range edges {
		all[lbl] = true
		for _, dep := range edges[lbl] {
			all[dep] = true
		}
	}
	for lbl := range hashLabels {
		all[lbl] = true
	}
	return all
}
