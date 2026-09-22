// hash-persister is a binary to compute and persist target hashes for a given git commit SHA.
// This allows for later comparison between commits without recomputing hashes.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/bazel-contrib/target-determinator/cli"
	"github.com/bazel-contrib/target-determinator/pkg"
)

type hashPersisterFlags struct {
	commonFlags     *cli.CommonFlags
	commitSha       string
	outputFile      string
	seedableOutput  bool
	seedFile        string
	seedSha         string
	reportFile      string
	fallbackPercent int
}

type config struct {
	Context         *pkg.Context
	CommitSha       string
	Targets         pkg.TargetsList
	OutputFile      string
	SeedableOutput  bool
	SeedFile        string
	SeedSha         string
	ReportFile      string
	FallbackPercent int
}

type seededOutcome struct {
	UsedSeed              bool
	FallbackCode          string
	FallbackDetail        string
	ChangedFileCount      int
	DirtyPackageCount     int
	DirtyTargetCount      int
	RecomputedTargetCount int
	ReusedTargetCount     int
	TotalTargetCount      int
}

type fallbackReason struct {
	Code   string
	Detail string
}

const (
	// Incremental hashing still pays the fixed costs of loading the seed,
	// propagating the dirty set, and merging a complete output. By default,
	// require it to reuse at least 30% of seeded targets before paying those costs.
	defaultRecomputationFallbackPercent = 70
	highRecomputationFallbackCode       = "high_recomputation_ratio"
)

type executionReport struct {
	SchemaVersion         int    `json:"schema_version"`
	RequestedMode         string `json:"requested_mode"`
	EffectiveMode         string `json:"effective_mode"`
	Status                string `json:"status"`
	FallbackCode          string `json:"fallback_code,omitempty"`
	FallbackDetail        string `json:"fallback_detail,omitempty"`
	ErrorDetail           string `json:"error_detail,omitempty"`
	ChangedFileCount      int    `json:"changed_file_count"`
	DirtyPackageCount     int    `json:"dirty_package_count"`
	DirtyTargetCount      int    `json:"dirty_target_count"`
	RecomputedTargetCount int    `json:"recomputed_target_count"`
	ReusedTargetCount     int    `json:"reused_target_count"`
	TotalTargetCount      int    `json:"total_target_count"`
}

func main() {
	start := time.Now()
	err := execute()
	log.Printf("Finished after %v", time.Since(start))
	if err != nil {
		log.Printf("Hash persister failed: %v", err)
		os.Exit(1)
	}
}

func execute() (returnErr error) {
	flags, err := parseFlags()
	if err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Failed to parse flags: %v\n", err)
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s <git-commit-sha>\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(flag.CommandLine.Output(), "Where <git-commit-sha> is the commit SHA to compute and persist hashes for.\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Optional flags:\n")
		flag.PrintDefaults()
		return err
	}

	cfg, err := resolveConfig(*flags)
	if err != nil {
		return fmt.Errorf("error during preprocessing: %w", err)
	}

	report := &executionReport{
		SchemaVersion: 1,
		RequestedMode: "full",
		EffectiveMode: "full",
	}
	if cfg.SeedFile != "" {
		report.RequestedMode = "incremental"
		report.EffectiveMode = "incremental"
	}
	defer func() {
		if returnErr != nil {
			report.Status = "failed"
			report.ErrorDetail = returnErr.Error()
		} else {
			report.Status = "success"
		}
		if cfg.ReportFile != "" {
			if err := writeExecutionReport(cfg.ReportFile, report); err != nil && returnErr == nil {
				returnErr = err
			}
		}
	}()

	defer func() {
		innerErr := gitCheckout(cfg.Context.WorkspacePath, cfg.Context.OriginalRevision)
		if innerErr != nil && returnErr == nil {
			returnErr = fmt.Errorf("failed to check out original commit during cleanup: %w", innerErr)
		}
	}()

	if cfg.SeedFile != "" {
		outcome, err := runSeeded(cfg)
		applySeededOutcome(report, outcome)
		return err
	}

	recomputedTargets, err := runFull(cfg)
	report.RecomputedTargetCount = recomputedTargets
	report.TotalTargetCount = recomputedTargets
	return err
}

func runFull(cfg *config) (int, error) {
	commitRev, err := pkg.NewLabelledGitRev(cfg.Context.WorkspacePath, cfg.CommitSha, "commit")
	if err != nil {
		return 0, fmt.Errorf("failed to resolve commit %s: %w", cfg.CommitSha, err)
	}

	log.Printf("Computing hashes for commit %s (full mode)", cfg.CommitSha)

	phaseStart := time.Now()
	queryResults, cleanup, err := pkg.LoadIncompleteMetadata(cfg.Context, commitRev, cfg.Targets)
	if err != nil {
		cleanup()
		return 0, fmt.Errorf("failed to load metadata for commit %s: %w", cfg.CommitSha, err)
	}
	defer cleanup()
	log.Printf("Phase query+parse completed in %v", time.Since(phaseStart))

	phaseStart = time.Now()
	queryResults.TargetHashCache.HashDebug = cfg.Context.HashDebug
	log.Println("Computing target hashes")
	if err := queryResults.PrefillCache(); err != nil {
		return 0, fmt.Errorf("failed to compute hashes for commit %s: %w", cfg.CommitSha, err)
	}
	targetCount := len(queryResults.MatchingTargets.Labels())
	log.Printf("Phase hash completed in %v (%d targets)",
		time.Since(phaseStart), targetCount)

	phaseStart = time.Now()
	log.Printf("Persisting hashes to %s", cfg.OutputFile)
	persist := pkg.PersistHashes
	if cfg.SeedableOutput {
		persist = pkg.PersistSeedableHashes
	}
	if err := persist(cfg.OutputFile, cfg.CommitSha, queryResults, cfg.Context, cfg.Targets.String()); err != nil {
		return 0, fmt.Errorf("failed to persist hashes: %w", err)
	}
	log.Printf("Phase persist completed in %v", time.Since(phaseStart))

	log.Printf("Successfully persisted hashes for %d targets to %s",
		targetCount, cfg.OutputFile)
	return targetCount, nil
}

func runSeeded(cfg *config) (seededOutcome, error) {
	var outcome seededOutcome
	fallback := func(code, detail string) (seededOutcome, error) {
		log.Printf("Falling back to full computation [%s]: %s", code, detail)
		recomputedTargets, err := runFull(cfg)
		outcome.FallbackCode = code
		outcome.FallbackDetail = detail
		outcome.RecomputedTargetCount = recomputedTargets
		outcome.TotalTargetCount = recomputedTargets
		return outcome, err
	}
	log.Printf("Computing hashes for commit %s (seeded from %s)", cfg.CommitSha, cfg.SeedSha)

	phaseStart := time.Now()
	seedData, err := pkg.LoadPersistedHashes(cfg.SeedFile)
	if err != nil {
		return fallback("seed_read_error", fmt.Sprintf("cannot load seed file %s: %v", cfg.SeedFile, err))
	}
	log.Printf("Loaded seed file: %d target hashes, %d edges (format v%d) in %v",
		len(seedData.TargetHashes), len(seedData.TargetEdges), seedData.FormatVersion,
		time.Since(phaseStart))

	currentBazelRelease, err := pkg.BazelRelease(cfg.Context.WorkspacePath, cfg.Context.BazelCmd)
	if err != nil {
		return seededOutcome{}, fmt.Errorf("failed to resolve current Bazel release: %w", err)
	}
	compatibilityFingerprint, err := pkg.ComputeSeedCompatibilityFingerprint(cfg.Context, cfg.Targets.String(), currentBazelRelease)
	if err != nil {
		return seededOutcome{}, err
	}
	if reason := validateSeed(seedData, cfg.SeedSha, compatibilityFingerprint); reason != nil {
		return fallback(reason.Code, reason.Detail)
	}

	phaseStart = time.Now()
	changedFiles, err := gitDiffNameStatus(cfg.Context.WorkspacePath, cfg.SeedSha, cfg.CommitSha)
	if err != nil {
		return fallback("git_diff_error", fmt.Sprintf("cannot compute git diff: %v", err))
	}
	outcome.ChangedFileCount = len(changedFiles)
	log.Printf("Git diff: %d changed files in %v", len(changedFiles), time.Since(phaseStart))

	fingerprintFiles := make(map[string]bool)
	for _, fp := range cfg.Context.RuleClassFingerprints {
		for _, f := range fp.Files {
			fingerprintFiles[f] = true
		}
	}

	phaseStart = time.Now()
	allLabels := pkg.CollectAllLabels(seedData.TargetEdges, seedData.TargetHashes)
	dirtyResult := pkg.ComputeDirtySet(changedFiles, seedData.TargetEdges, allLabels, fingerprintFiles)
	outcome.DirtyPackageCount = len(dirtyResult.DirtyPackages)
	outcome.DirtyTargetCount = len(dirtyResult.DirtyStarLabels)
	log.Printf("Dirty set computed in %v: %d dirty, %d dirty*, %d dirty packages (fallback=%v)",
		time.Since(phaseStart), len(dirtyResult.DirtyLabels),
		len(dirtyResult.DirtyStarLabels), len(dirtyResult.DirtyPackages),
		dirtyResult.NeedsFallback)

	if dirtyResult.NeedsFallback {
		code := dirtyResult.FallbackCode
		if code == "" {
			code = "unsafe_file_change"
		}
		return fallback(code, dirtyResult.FallbackReason)
	}

	if len(dirtyResult.DirtyPackages) == 0 && len(dirtyResult.DirtyStarLabels) == 0 {
		log.Printf("No targets affected; persisting seed hashes under commit %s", cfg.CommitSha)
		persistedData := newPersistedOutput(
			cfg,
			seedData.BazelRelease,
			seedData.TargetHashes,
			seedData.TargetEdges,
			seedData.SeedCompatibilityFingerprint,
		)
		if err := pkg.WritePersistedData(cfg.OutputFile, persistedData); err != nil {
			return seededOutcome{}, fmt.Errorf("failed to persist hashes: %w", err)
		}
		outcome.UsedSeed = true
		outcome.ReusedTargetCount = len(seedData.TargetHashes)
		outcome.TotalTargetCount = len(seedData.TargetHashes)
		return outcome, nil
	}

	commitRev, err := pkg.NewLabelledGitRev(cfg.Context.WorkspacePath, cfg.CommitSha, "commit")
	if err != nil {
		return seededOutcome{}, fmt.Errorf("failed to resolve commit %s: %w", cfg.CommitSha, err)
	}

	// Phase: probe dirty packages to prune false-positive rdeps.
	//
	// When a BUILD.bazel changes, all targets in the package are marked
	// dirty and their rdeps cascade through the graph. But most existing
	// targets are often unchanged (e.g. a new sibling was added). A small
	// probe query on just the dirty packages lets us compare hashes against
	// the seed and only propagate rdeps from targets that actually changed.
	unprunedDirtyStarCount := len(dirtyResult.DirtyStarLabels)
	if len(dirtyResult.DirtyStarLabels) > len(dirtyResult.DirtyLabels) {
		dirtyResult, err = probePruneDirtySet(cfg, commitRev, dirtyResult, seedData)
		if err != nil {
			log.Printf("Probe pruning failed, continuing with unpruned dirty set: %v", err)
		} else {
			log.Printf("Probe pruning: %d dirty* -> %d dirty* (%d eliminated)",
				unprunedDirtyStarCount, len(dirtyResult.DirtyStarLabels),
				unprunedDirtyStarCount-len(dirtyResult.DirtyStarLabels))
		}
	}

	estimatedRecomputedTargets := countDirtySeedTargets(seedData.TargetHashes, dirtyResult.DirtyStarLabels)
	seedTargetCount := len(seedData.TargetHashes)
	if shouldFallbackForRecomputation(
		estimatedRecomputedTargets,
		seedTargetCount,
		cfg.FallbackPercent,
	) {
		return fallback(highRecomputationFallbackCode, fmt.Sprintf(
			"dirty set includes %d of %d seeded targets (at least %d%%)",
			estimatedRecomputedTargets, seedTargetCount, cfg.FallbackPercent))
	}

	// The scoped universe re-lists every dirty package with a wildcard (to
	// pick up added and deleted targets) and names the rdeps-propagated
	// targets in unchanged packages explicitly.
	carriedLabels := make([]string, 0, len(dirtyResult.DirtyStarLabels))
	dirtyPackageSet := make(map[string]bool, len(dirtyResult.DirtyPackages))
	for _, p := range dirtyResult.DirtyPackages {
		dirtyPackageSet[p] = true
	}
	for label := range dirtyResult.DirtyStarLabels {
		// Only carry labels that were matching targets in the seed: labels
		// that only appear as dependency edges (source files, external or
		// manual targets) are not part of the persisted target set.
		if _, ok := seedData.TargetHashes[label]; !ok {
			continue
		}
		if dirtyPackageSet[pkg.LabelPackage(label)] {
			continue
		}
		carriedLabels = append(carriedLabels, label)
	}

	universe := pkg.BuildScopedUniverse(dirtyResult.DirtyPackages, carriedLabels)
	scopedPattern, err := pkg.ScopeTargetsPattern(cfg.Targets.String(), universe)
	if err != nil {
		return fallback("unscopable_target_pattern", fmt.Sprintf("cannot scope targets pattern: %v", err))
	}

	phaseStart = time.Now()
	scopedTargets, err := pkg.ParseTargetsList(scopedPattern)
	if err != nil {
		return seededOutcome{}, fmt.Errorf("failed to parse scoped targets: %w", err)
	}

	queryResults, cleanup, err := pkg.LoadIncompleteMetadata(cfg.Context, commitRev, scopedTargets)
	if err != nil {
		cleanup()
		return fallback("scoped_query_error", fmt.Sprintf("scoped query failed: %v", err))
	}
	if queryResults.BazelRelease != seedData.BazelRelease {
		cleanup()
		return fallback("bazel_release_changed", fmt.Sprintf("Bazel release changed from %q to %q", seedData.BazelRelease, queryResults.BazelRelease))
	}
	log.Printf("Phase scoped query+parse completed in %v", time.Since(phaseStart))

	phaseStart = time.Now()
	seedHashes, err := reusableSeedHashes(seedData, dirtyResult.DirtyStarLabels)
	if err != nil {
		cleanup()
		return fallback("seed_hash_invalid", fmt.Sprintf("seed contains invalid hashes: %v", err))
	}

	if err := queryResults.TargetHashCache.SeedHashes(seedHashes); err != nil {
		cleanup()
		return fallback("seed_population_error", fmt.Sprintf("cannot seed hashes: %v", err))
	}
	defer cleanup()
	log.Printf("Seeded %d unchanged hashes in %v", len(seedHashes), time.Since(phaseStart))

	phaseStart = time.Now()
	queryResults.TargetHashCache.HashDebug = cfg.Context.HashDebug
	log.Printf("Computing hashes for %d scoped targets", len(queryResults.MatchingTargets.Labels()))
	if err := queryResults.PrefillCache(); err != nil {
		return seededOutcome{}, fmt.Errorf("failed to compute hashes: %w", err)
	}
	log.Printf("Phase hash completed in %v", time.Since(phaseStart))

	phaseStart = time.Now()
	persistedData, err := mergePersistedData(seedData, dirtyResult.DirtyStarLabels, queryResults, cfg, compatibilityFingerprint)
	if err != nil {
		return seededOutcome{}, err
	}

	if err := pkg.WritePersistedData(cfg.OutputFile, persistedData); err != nil {
		return seededOutcome{}, fmt.Errorf("failed to persist merged hashes: %w", err)
	}
	log.Printf("Phase persist+merge completed in %v", time.Since(phaseStart))

	log.Printf("Successfully persisted %d target hashes to %s (seeded mode, %d recomputed)",
		len(persistedData.TargetHashes), cfg.OutputFile, len(queryResults.MatchingTargets.Labels()))
	outcome.UsedSeed = true
	outcome.RecomputedTargetCount = len(queryResults.MatchingTargets.Labels())
	outcome.TotalTargetCount = len(persistedData.TargetHashes)
	outcome.ReusedTargetCount = outcome.TotalTargetCount - outcome.RecomputedTargetCount
	return outcome, nil
}

// probePruneDirtySet runs a small probe query on just the dirty packages,
// hashes those targets with seed hashes for their dependencies, and returns
// a pruned DirtySetResult where rdeps are only propagated from targets
// whose hash actually changed.
func probePruneDirtySet(
	cfg *config,
	commitRev pkg.LabelledGitRev,
	dirtyResult *pkg.DirtySetResult,
	seedData *pkg.PersistedHashData,
) (*pkg.DirtySetResult, error) {
	phaseStart := time.Now()

	// Build a probe universe covering only the dirty packages (no carried labels).
	probeUniverse := pkg.BuildScopedUniverse(dirtyResult.DirtyPackages, nil)
	probePattern, err := pkg.ScopeTargetsPattern(cfg.Targets.String(), probeUniverse)
	if err != nil {
		return nil, fmt.Errorf("cannot scope probe pattern: %w", err)
	}
	probeTargets, err := pkg.ParseTargetsList(probePattern)
	if err != nil {
		return nil, fmt.Errorf("failed to parse probe targets: %w", err)
	}

	probeResults, probeCleanup, err := pkg.LoadIncompleteMetadata(cfg.Context, commitRev, probeTargets)
	if err != nil {
		probeCleanup()
		return nil, fmt.Errorf("probe query failed: %w", err)
	}

	// Seed the probe hash cache with hashes for all targets NOT in dirty
	// packages, so that dependency hashes resolve without a full query.
	dirtyPkgSet := make(map[string]bool, len(dirtyResult.DirtyPackages))
	for _, p := range dirtyResult.DirtyPackages {
		dirtyPkgSet[p] = true
	}
	probeSeedHashes := make(map[string][]byte)
	for label, configMap := range seedData.TargetHashes {
		if dirtyPkgSet[pkg.LabelPackage(label)] {
			continue
		}
		for configStr, hashHex := range configMap {
			hashBytes, err := hex.DecodeString(hashHex)
			if err != nil {
				probeCleanup()
				return nil, fmt.Errorf("invalid seed hash for %s: %w", label, err)
			}
			probeSeedHashes[label+"\x00"+configStr] = hashBytes
		}
	}
	if err := probeResults.TargetHashCache.SeedHashes(probeSeedHashes); err != nil {
		probeCleanup()
		return nil, fmt.Errorf("cannot seed probe hashes: %w", err)
	}

	if err := probeResults.PrefillCache(); err != nil {
		probeCleanup()
		return nil, fmt.Errorf("probe hashing failed: %w", err)
	}
	probeCleanup()

	probeHashes, err := pkg.ProbeHashesFromQueryResults(probeResults)
	if err != nil {
		return nil, fmt.Errorf("failed to extract probe hashes: %w", err)
	}
	log.Printf("Phase probe completed in %v (%d targets in %d packages)",
		time.Since(phaseStart), len(probeHashes), len(dirtyResult.DirtyPackages))

	pruned := pkg.PruneDirtySet(dirtyResult, seedData.TargetHashes, seedData.TargetEdges, probeHashes)
	return pruned, nil
}

func countDirtySeedTargets(targetHashes map[string]map[string]string, dirtyLabels map[string]bool) int {
	count := 0
	for label := range targetHashes {
		if dirtyLabels[label] {
			count++
		}
	}
	return count
}

func shouldFallbackForRecomputation(recomputedTargets, totalTargets, fallbackPercent int) bool {
	if totalTargets <= 0 || recomputedTargets <= 0 {
		return false
	}
	return recomputedTargets*100 >= totalTargets*fallbackPercent
}

func validateSeed(seedData *pkg.PersistedHashData, expectedSha, expectedFingerprint string) *fallbackReason {
	if seedData.FormatVersion != pkg.CurrentPersistedHashFormatVersion {
		return &fallbackReason{"seed_format_incompatible", fmt.Sprintf("seed format v%d is not supported (need v%d)", seedData.FormatVersion, pkg.CurrentPersistedHashFormatVersion)}
	}
	if seedData.TargetEdges == nil {
		return &fallbackReason{"seed_missing_edges", "seed has no dependency edges"}
	}
	if seedData.GitCommitSha != expectedSha {
		return &fallbackReason{"seed_commit_mismatch", fmt.Sprintf("seed commit %q does not match --seed-sha %q", seedData.GitCommitSha, expectedSha)}
	}
	if seedData.SeedCompatibilityFingerprint == "" {
		return &fallbackReason{"seed_compatibility_mismatch", "seed has no compatibility fingerprint"}
	}
	if seedData.SeedCompatibilityFingerprint != expectedFingerprint {
		return &fallbackReason{"seed_compatibility_mismatch", "seed compatibility fingerprint does not match this invocation"}
	}
	if _, err := reusableSeedHashes(seedData, nil); err != nil {
		return &fallbackReason{"seed_hash_invalid", fmt.Sprintf("seed contains invalid hashes: %v", err)}
	}
	return nil
}

func reusableSeedHashes(seedData *pkg.PersistedHashData, dirtyLabels map[string]bool) (map[string][]byte, error) {
	hashes := make(map[string][]byte)
	for label, configMap := range seedData.TargetHashes {
		if dirtyLabels[label] {
			continue
		}
		for configStr, hashHex := range configMap {
			hashBytes, err := hex.DecodeString(hashHex)
			if err != nil {
				return nil, fmt.Errorf("invalid hash hex for %s: %w", label, err)
			}
			if len(hashBytes) != sha256.Size {
				return nil, fmt.Errorf("invalid hash length for %s: got %d bytes, want %d", label, len(hashBytes), sha256.Size)
			}
			hashes[label+"\x00"+configStr] = hashBytes
		}
	}
	return hashes, nil
}

func mergePersistedData(
	seedData *pkg.PersistedHashData,
	dirtyLabels map[string]bool,
	queryResults *pkg.QueryResults,
	cfg *config,
	compatibilityFingerprint string,
) (*pkg.PersistedHashData, error) {
	// Only persist hashes for scoped matching targets, mirroring full mode.
	freshHashes := make(map[string]map[string]string)
	for _, label := range queryResults.MatchingTargets.Labels() {
		labelStr := label.String()
		for _, configuration := range queryResults.MatchingTargets.ConfigurationsFor(label) {
			hash, err := queryResults.TargetHashCache.Hash(pkg.LabelAndConfiguration{
				Label: label, Configuration: configuration,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get hash for scoped target %s: %w", labelStr, err)
			}
			if freshHashes[labelStr] == nil {
				freshHashes[labelStr] = make(map[string]string)
			}
			freshHashes[labelStr][configuration.String()] = hex.EncodeToString(hash)
		}
	}
	mergedHashes := mergePersistedEntries(seedData.TargetHashes, dirtyLabels, freshHashes)
	persistedData := newPersistedOutput(cfg, queryResults.BazelRelease, mergedHashes, nil, "")
	if !cfg.SeedableOutput {
		return persistedData, nil
	}

	freshEdges := make(map[string][]string)
	if queryResults.TransitiveConfiguredTargets != nil {
		var err error
		freshEdges, err = pkg.ExtractEdges(queryResults)
		if err != nil {
			return nil, fmt.Errorf("failed to extract fresh edges: %w", err)
		}
	}
	mergedEdges := mergePersistedEntries(seedData.TargetEdges, dirtyLabels, freshEdges)
	persistedData.FormatVersion = pkg.CurrentPersistedHashFormatVersion
	persistedData.SeedCompatibilityFingerprint = compatibilityFingerprint
	persistedData.TargetEdges = mergedEdges
	return persistedData, nil
}

// newPersistedOutput builds a complete hash artifact while including the
// dependency graph only when the caller explicitly requests seedable output.
func newPersistedOutput(
	cfg *config,
	bazelRelease string,
	targetHashes map[string]map[string]string,
	targetEdges map[string][]string,
	compatibilityFingerprint string,
) *pkg.PersistedHashData {
	data := &pkg.PersistedHashData{
		GitCommitSha: cfg.CommitSha,
		Timestamp:    time.Now(),
		BazelRelease: bazelRelease,
		TargetHashes: targetHashes,
		Metadata: pkg.HashMetadata{
			TargetsPattern: cfg.Targets.String(),
			WorkspacePath:  cfg.Context.WorkspacePath,
			TotalTargets:   countHashes(targetHashes),
		},
	}
	if cfg.SeedableOutput {
		data.FormatVersion = pkg.CurrentPersistedHashFormatVersion
		data.SeedCompatibilityFingerprint = compatibilityFingerprint
		data.TargetEdges = targetEdges
	}
	return data
}

// mergePersistedEntries retains clean seed entries and overlays entries read
// from the destination revision. A dirty entry absent from fresh is deleted.
func mergePersistedEntries[V any](seed map[string]V, dirty map[string]bool, fresh map[string]V) map[string]V {
	merged := make(map[string]V, len(seed)+len(fresh))
	for label, value := range seed {
		if !dirty[label] {
			merged[label] = value
		}
	}
	for label, value := range fresh {
		merged[label] = value
	}
	return merged
}

func parseFlags() (*hashPersisterFlags, error) {
	var flags hashPersisterFlags
	flags.commonFlags = cli.RegisterCommonFlags()
	flag.StringVar(&flags.outputFile, "output", "", "Output file path for persisted hashes (required)")
	flag.BoolVar(&flags.seedableOutput, "seedable-output", false, "Include dependency edges and compatibility metadata so the output can seed incremental hashing")
	flag.StringVar(&flags.seedFile, "seed-file", "", "Path to a compatible seed hash file for incremental hashing")
	flag.StringVar(&flags.seedSha, "seed-sha", "", "Git commit SHA of the seed file (required with --seed-file)")
	flag.StringVar(&flags.reportFile, "execution-report", "", "Optional path for a machine-readable JSON execution report")
	flag.IntVar(
		&flags.fallbackPercent,
		"incremental-recomputation-fallback-percent",
		defaultRecomputationFallbackPercent,
		"Fall back to full hashing when an incremental run would recompute at least this percentage of seeded targets (1-100)",
	)

	flag.Parse()

	if flags.outputFile == "" {
		return nil, fmt.Errorf("output file is required")
	}

	if flags.seedFile != "" && flags.seedSha == "" {
		return nil, fmt.Errorf("--seed-sha is required when --seed-file is specified")
	}
	if err := validateRecomputationFallbackPercent(flags.fallbackPercent); err != nil {
		return nil, err
	}

	positional := flag.Args()
	if len(positional) != 1 {
		return nil, fmt.Errorf("expected one positional argument, <git-commit-sha>, but got %d", len(positional))
	}
	flags.commitSha = positional[0]

	return &flags, nil
}

func resolveConfig(flags hashPersisterFlags) (*config, error) {
	seedableOutput := flags.seedableOutput
	if err := validateIncrementalBackend(seedableOutput, flags.seedFile != "", *flags.commonFlags.QueryBackend); err != nil {
		return nil, err
	}
	if *flags.commonFlags.QueryBackend == "query" && *flags.commonFlags.AnalysisCacheClearStrategy != "skip" {
		return nil, fmt.Errorf("--analysis-cache-clear-strategy=%s is incompatible with --query-backend=query: bazel query does not use the analysis cache", *flags.commonFlags.AnalysisCacheClearStrategy)
	}

	workingDirectory, err := filepath.Abs(*flags.commonFlags.WorkingDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory from %v: %w", *flags.commonFlags.WorkingDirectory, err)
	}

	currentBranch, err := pkg.GitRevParse(workingDirectory, "HEAD", true)
	if err != nil {
		return nil, fmt.Errorf("failed to get current git revision: %w", err)
	}

	currentRev, err := pkg.NewLabelledGitRev(workingDirectory, currentBranch, "current")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the current git revision: %w", err)
	}

	bazelCmd := pkg.DefaultBazelCmd{
		BazelPath:        *flags.commonFlags.BazelPath,
		BazelStartupOpts: *flags.commonFlags.BazelStartupOpts,
		BazelOpts:        *flags.commonFlags.BazelOpts,
	}

	outputBase, err := pkg.BazelOutputBase(workingDirectory, bazelCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the bazel output base: %w", err)
	}

	context := &pkg.Context{
		WorkspacePath:                          workingDirectory,
		OriginalRevision:                       currentRev,
		BazelCmd:                               bazelCmd,
		BazelOutputBase:                        outputBase,
		DeleteCachedWorktree:                   flags.commonFlags.DeleteCachedWorktree,
		IgnoredFiles:                           *flags.commonFlags.IgnoredFiles,
		BeforeQueryErrorBehavior:               *flags.commonFlags.BeforeQueryErrorBehavior,
		AnalysisCacheClearStrategy:             *flags.commonFlags.AnalysisCacheClearStrategy,
		CompareQueriesAroundAnalysisCacheClear: flags.commonFlags.CompareQueriesAroundAnalysisCacheClear,
		FilterIncompatibleTargets:              flags.commonFlags.FilterIncompatibleTargets,
		EnforceCleanRepo:                       flags.commonFlags.EnforceCleanRepo == cli.EnforceClean,
		QueryBackend:                           *flags.commonFlags.QueryBackend,
		RuleClassFingerprints:                  []pkg.RuleClassFingerprint(*flags.commonFlags.RuleClassFingerprints),
		HashDebug:                              flags.commonFlags.HashDebug,
	}

	targetsList, err := pkg.ParseTargetsList(*flags.commonFlags.TargetsFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to parse targets: %w", err)
	}

	outputDir := filepath.Dir(flags.outputFile)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
		}
	}

	return &config{
		Context:         context,
		CommitSha:       flags.commitSha,
		Targets:         targetsList,
		OutputFile:      flags.outputFile,
		SeedableOutput:  seedableOutput,
		SeedFile:        flags.seedFile,
		SeedSha:         flags.seedSha,
		ReportFile:      flags.reportFile,
		FallbackPercent: flags.fallbackPercent,
	}, nil
}

func validateRecomputationFallbackPercent(percent int) error {
	if percent < 1 || percent > 100 {
		return fmt.Errorf("--incremental-recomputation-fallback-percent must be between 1 and 100 (got %d)", percent)
	}
	return nil
}

func applySeededOutcome(report *executionReport, outcome seededOutcome) {
	if outcome.UsedSeed {
		report.EffectiveMode = "incremental"
	} else if outcome.FallbackCode != "" {
		report.EffectiveMode = "full"
	}
	report.FallbackCode = outcome.FallbackCode
	report.FallbackDetail = outcome.FallbackDetail
	report.ChangedFileCount = outcome.ChangedFileCount
	report.DirtyPackageCount = outcome.DirtyPackageCount
	report.DirtyTargetCount = outcome.DirtyTargetCount
	report.RecomputedTargetCount = outcome.RecomputedTargetCount
	report.ReusedTargetCount = outcome.ReusedTargetCount
	report.TotalTargetCount = outcome.TotalTargetCount
}

func writeExecutionReport(path string, report *executionReport) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create execution report %s: %w", path, err)
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(report); err != nil {
		return fmt.Errorf("failed to encode execution report %s: %w", path, err)
	}
	return nil
}

func validateIncrementalBackend(seedableOutput, seededInput bool, queryBackend string) error {
	if (seedableOutput || seededInput) && queryBackend != "query" {
		return fmt.Errorf("incremental hashing requires --query-backend=query (got %q)", queryBackend)
	}
	return nil
}

func gitCheckout(workingDirectory string, rev pkg.LabelledGitRev) error {
	gitCmd := exec.Command("git", "checkout", rev.GitRevision.Revision)
	gitCmd.Dir = workingDirectory
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to check out %s: %w. Output: %v", rev, err, string(output))
	}
	return nil
}

func gitDiffNameStatus(workingDirectory, fromSha, toSha string) (map[string]string, error) {
	gitCmd := exec.Command("git", "diff", "--name-status", "-z", "--no-renames", fromSha+".."+toSha, "--")
	gitCmd.Dir = workingDirectory
	var stdout, stderr bytes.Buffer
	gitCmd.Stdout = &stdout
	gitCmd.Stderr = &stderr
	if err := gitCmd.Run(); err != nil {
		return nil, fmt.Errorf("git diff --name-status %s..%s failed: %w. Stderr: %s",
			fromSha, toSha, err, stderr.String())
	}

	return parseGitNameStatus(stdout.Bytes())
}

func parseGitNameStatus(output []byte) (map[string]string, error) {
	fields := bytes.Split(output, []byte{0})
	if len(fields) > 0 && len(fields[len(fields)-1]) == 0 {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%2 != 0 {
		return nil, fmt.Errorf("malformed NUL-delimited git name-status output")
	}
	result := make(map[string]string, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		status, path := string(fields[i]), string(fields[i+1])
		if status == "" || path == "" {
			return nil, fmt.Errorf("malformed empty status or path in git name-status output")
		}
		result[path] = status
	}
	return result, nil
}

func countHashes(hashes map[string]map[string]string) int {
	n := 0
	for _, configs := range hashes {
		n += len(configs)
	}
	return n
}
