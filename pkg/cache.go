package pkg

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"

	ss "github.com/bazel-contrib/target-determinator/common/sorted_set"
	"github.com/bazelbuild/bazel-gazelle/label"
)

var configuredTargetCacheDirname = "results"

// CacheKey represents the inputs used to generate a cache filename.
//
// Known limitations (not included in the key, callers should be aware):
//
//   - User and system .bazelrc files (~/.bazelrc, /etc/bazel.bazelrc, files
//     imported from those) can affect the build configuration without changing
//     the git tree. Target-determinator does not read these files; if they
//     change between invocations, use --nocache_results.
//
//   - The current machine's hardware and OS are not included. Cache entries
//     produced on one machine are not guaranteed to be valid on another (e.g.
//     different CPU architecture may change platform-constrained target sets).
//     Do not share the cache directory across machines.
//
//   - Environment variables forwarded to Bazel (CC, CXX, JAVA_HOME, BAZELRC,
//     etc.) are not included. Changing these between invocations can affect
//     cquery results without invalidating the cache. This is usually the
//     expected behavior but has implications. See README.md.
type CacheKey struct {
	TDBinaryHash  string
	BazelVersion  string
	GitTreeSHA    string
	TargetPattern string
	// Context holds the Context fields that affect the cache key (i.e. those not tagged
	// results_cache_key_ignore:"true"), plus the opaque hash returned by BazelCmd.HashKey().
	// encoding/json marshals map keys alphabetically, ensuring a deterministic serialization.
	Context map[string]interface{}
}

// SerializedQueryResults is the structure that gets saved to disk
type SerializedQueryResults struct {
	// Serialized protobuf of ConfiguredTargets
	MatchingTargetsData []byte
	BazelRelease        string
	NormalizerMapping   map[string]string
	// Key: "<label>\x00<config>", value: raw SHA256 bytes.
	PrecomputedHashes map[string][]byte
}

// ComputeCacheKey generates a unique cache key based on the binary hash, git SHA, and CLI options
func ComputeCacheKey(context *Context, gitSHA string, targetPattern string) (string, error) {
	// Get the binary's hash
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	binaryHash, err := hashFile(execPath)
	if err != nil {
		return "", fmt.Errorf("failed to hash binary: %w", err)
	}

	bazelRelease, err := BazelRelease(context.WorkspacePath, context.BazelCmd)
	if err != nil {
		return "", fmt.Errorf("failed to get bazel release for cache key: %w", err)
	}

	// Collect all cache-affecting context fields.
	contextKey := collectCacheContextFields(context)

	key := CacheKey{
		TDBinaryHash:  binaryHash,
		BazelVersion:  bazelRelease,
		GitTreeSHA:    gitSHA,
		TargetPattern: targetPattern,
		Context:       contextKey,
	}

	// Serialize to JSON for consistent hashing
	keyJSON, err := json.Marshal(key)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cache key: %w", err)
	}

	// Hash the key
	hasher := sha256.New()
	hasher.Write(keyJSON)
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// collectCacheContextFields returns a map of the Context fields that affect the cache key
// (i.e. those not tagged results_cache_key_ignore:"true").
// If new fields are added to Context without the ignore tag, they must be added here too;
// cache_test.go enforces this via a reflection-based cross-check.
func collectCacheContextFields(ctx *Context) map[string]interface{} {
	ignoredFiles := make([]string, len(ctx.IgnoredFiles))
	for i, f := range ctx.IgnoredFiles {
		ignoredFiles[i] = f.String()
	}
	sort.Strings(ignoredFiles)
	return map[string]interface{}{
		"BazelCmd":                  ctx.BazelCmd.HashKey(),
		"IgnoredFiles":              ignoredFiles,
		"FilterIncompatibleTargets": ctx.FilterIncompatibleTargets,
		"QueryBackend":              ctx.QueryBackend,
	}
}

// hashFile computes SHA256 hash of a file
func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// LoadFromCache attempts to load QueryResults from cache.
// The treeSHA argument should represent the hash of a Git Tree object (which doesn't change if only the commit
// metadata, such as the git commit message, changes).
// Returns (results, error). If error is non-nil, the cache was not hit.
func LoadFromCache(context *Context, treeSHA string, targetPattern string) (*QueryResults, error) {
	if context.CacheDirectory == "" {
		return nil, fmt.Errorf("cache directory not configured")
	}

	cacheKey, err := ComputeCacheKey(context, treeSHA, targetPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to compute cache key: %w", err)
	}

	cacheItemPath := filepath.Join(context.CacheDirectory, configuredTargetCacheDirname, cacheKey)

	log.Printf("Attempting to load from cache: %s", cacheItemPath)

	data, err := os.ReadFile(cacheItemPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cache miss")
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var serialized SerializedQueryResults
	if err := json.Unmarshal(data, &serialized); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache data: %w", err)
	}

	// Deserialize matching targets
	matchingTargets, err := deserializeMatchingTargets(serialized.MatchingTargetsData)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize matching targets: %w", err)
	}

	normalizer := Normalizer{Mapping: serialized.NormalizerMapping}

	// TransitiveConfiguredTargets is not stored in cache to save space. Pre-computed hashes mean context
	// is never accessed on a cache hit; nil is safe here.
	queryResults := &QueryResults{
		MatchingTargets:             matchingTargets,
		TransitiveConfiguredTargets: nil,
		TargetHashCache:             NewTargetHashCache(nil, &normalizer, serialized.BazelRelease, false),
		BazelRelease:                serialized.BazelRelease,
	}

	if err := queryResults.TargetHashCache.RestoreHashes(serialized.PrecomputedHashes); err != nil {
		return nil, fmt.Errorf("failed to restore hashes from cache: %w", err)
	}

	log.Printf("Cache hit! Loaded results from cache")
	return queryResults, nil
}

// SaveToCache saves QueryResults to cache
func SaveToCache(context *Context, gitSHA string, targetPattern string, queryResults *QueryResults) error {
	if context.CacheDirectory == "" {
		return nil // Caching disabled
	}

	cacheKey, err := ComputeCacheKey(context, gitSHA, targetPattern)
	if err != nil {
		return fmt.Errorf("failed to compute cache key: %w", err)
	}

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(context.CacheDirectory, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Serialize matching targets
	matchingTargetsData, err := serializeMatchingTargets(queryResults.MatchingTargets)
	if err != nil {
		return fmt.Errorf("failed to serialize matching targets: %w", err)
	}

	serialized := SerializedQueryResults{
		MatchingTargetsData: matchingTargetsData,
		BazelRelease:        queryResults.BazelRelease,
		NormalizerMapping:   queryResults.TargetHashCache.normalizer.Mapping,
		PrecomputedHashes:   queryResults.TargetHashCache.ExtractHashes(),
	}

	data, err := json.Marshal(serialized)
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	cacheItemDir := filepath.Join(context.CacheDirectory, configuredTargetCacheDirname)
	cacheItemPath := filepath.Join(cacheItemDir, cacheKey)
	if err := os.MkdirAll(cacheItemDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir (%s): %w", cacheItemDir, err)
	}

	tmpFile, err := os.CreateTemp(cacheItemDir, cacheKey+".tmp.*")
	if err != nil {
		return fmt.Errorf("failed to create temp cache file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp cache file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp cache file: %w", err)
	}
	if err := os.Rename(tmpPath, cacheItemPath); err != nil {
		return fmt.Errorf("failed to move temp cache file to final location: %w", err)
	}
	tmpPath = ""

	log.Printf("Saved results to cache: %s", cacheItemPath)
	return nil
}

// Helper types for serialization
type serializedMatchingTargets struct {
	Labels                 []string
	LabelsToConfigurations map[string][]string
}

func serializeMatchingTargets(mt *MatchingTargets) ([]byte, error) {
	labels := make([]string, 0)
	for _, l := range mt.Labels() {
		labels = append(labels, l.String())
	}

	labelsToConfigurations := make(map[string][]string)
	for _, l := range mt.Labels() {
		configs := make([]string, 0)
		for _, c := range mt.ConfigurationsFor(l) {
			configs = append(configs, c.String())
		}
		labelsToConfigurations[l.String()] = configs
	}

	serialized := serializedMatchingTargets{
		Labels:                 labels,
		LabelsToConfigurations: labelsToConfigurations,
	}

	return json.Marshal(serialized)
}

func deserializeMatchingTargets(data []byte) (*MatchingTargets, error) {
	var serialized serializedMatchingTargets
	if err := json.Unmarshal(data, &serialized); err != nil {
		return nil, err
	}

	labels := make([]label.Label, 0)
	labelsToConfigurations := make(map[label.Label][]Configuration)

	for _, labelStr := range serialized.Labels {
		l, err := label.Parse(labelStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse label %s: %w", labelStr, err)
		}
		labels = append(labels, l)

		configs := make([]Configuration, 0)
		for _, configStr := range serialized.LabelsToConfigurations[labelStr] {
			configs = append(configs, NormalizeConfiguration(configStr))
		}
		labelsToConfigurations[l] = configs
	}

	// Convert to SortedSet format using the actual sorted_set package
	labelsSet := ss.NewSortedSetFn(labels, CompareLabels)
	labelsToConfigurationsMap := make(map[label.Label]*ss.SortedSet[Configuration])
	for l, configs := range labelsToConfigurations {
		labelsToConfigurationsMap[l] = ss.NewSortedSetFn(configs, ConfigurationLess)
	}

	return &MatchingTargets{
		labels:                 labelsSet,
		labelsToConfigurations: labelsToConfigurationsMap,
	}, nil
}
