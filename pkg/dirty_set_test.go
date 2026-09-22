package pkg

import (
	"sort"
	"testing"
)

func TestComputeDirtySetSourceFileChange(t *testing.T) {
	edges := map[string][]string{
		"//pkg:rule_a": {"//pkg:src.java"},
		"//app:binary": {"//pkg:rule_a"},
	}
	allLabels := CollectAllLabels(edges, nil)

	changedFiles := map[string]string{
		"pkg/src.java": "M",
	}

	result := ComputeDirtySet(changedFiles, edges, allLabels, nil)

	if result.NeedsFallback {
		t.Fatal("unexpected fallback")
	}
	if !result.DirtyLabels["//pkg:rule_a"] {
		t.Error("expected //pkg:rule_a in DirtyLabels")
	}
	if !result.DirtyLabels["//pkg:src.java"] {
		t.Error("expected //pkg:src.java in DirtyLabels")
	}
	if !result.DirtyStarLabels["//app:binary"] {
		t.Error("expected //app:binary in DirtyStarLabels (rdep of //pkg:rule_a)")
	}
}

func TestComputeDirtySetBUILDFileChange(t *testing.T) {
	edges := map[string][]string{
		"//pkg:rule_a": {"//pkg:src.java"},
		"//pkg:rule_b": {"//other:dep"},
		"//other:dep":  {},
	}
	allLabels := CollectAllLabels(edges, nil)

	changedFiles := map[string]string{
		"pkg/BUILD.bazel": "M",
	}

	result := ComputeDirtySet(changedFiles, edges, allLabels, nil)

	if result.NeedsFallback {
		t.Fatal("unexpected fallback")
	}
	if !result.DirtyLabels["//pkg:rule_a"] {
		t.Error("expected //pkg:rule_a dirty")
	}
	if !result.DirtyLabels["//pkg:rule_b"] {
		t.Error("expected //pkg:rule_b dirty")
	}
	if result.DirtyLabels["//other:dep"] {
		t.Error("//other:dep should not be dirty")
	}
}

func TestComputeDirtySetMainRootDoesNotDirtyExternalRootLabels(t *testing.T) {
	edges := map[string][]string{
		"//:main": {"//:root.txt"},
		"@@bazel_tools+remote_coverage_tools_extension+remote_coverage_tools//:coverage_report_generator": {},
		"@bazel_tools//tools/test:coverage_report_generator": {
			"@@bazel_tools+remote_coverage_tools_extension+remote_coverage_tools//:coverage_report_generator",
		},
		"//.agents/skills/migrate-mma-dashboards:migrate-mma-test": {
			"@bazel_tools//tools/test:coverage_report_generator",
		},
	}
	allLabels := CollectAllLabels(edges, nil)

	result := ComputeDirtySet(
		map[string]string{"root.txt": "M"}, edges, allLabels, nil,
	)

	if !result.DirtyLabels["//:main"] {
		t.Error("expected main-workspace root target to be dirty")
	}
	for _, label := range []string{
		"@@bazel_tools+remote_coverage_tools_extension+remote_coverage_tools//:coverage_report_generator",
		"@bazel_tools//tools/test:coverage_report_generator",
		"//.agents/skills/migrate-mma-dashboards:migrate-mma-test",
	} {
		if result.DirtyStarLabels[label] {
			t.Errorf("unrelated label %s must not become dirty", label)
		}
	}
}

func TestComputeDirtySetMainPackageDoesNotDirtyExternalPackageLabels(t *testing.T) {
	edges := map[string][]string{
		"//shared:main":                  {"//shared:source.txt"},
		"@@canonical_repo//shared:other": {},
		"@apparent_repo//shared:other":   {},
	}
	allLabels := CollectAllLabels(edges, nil)

	result := ComputeDirtySet(
		map[string]string{"shared/source.txt": "M"}, edges, allLabels, nil,
	)

	if !result.DirtyLabels["//shared:main"] {
		t.Error("expected main-workspace target to be dirty")
	}
	for _, label := range []string{
		"@@canonical_repo//shared:other",
		"@apparent_repo//shared:other",
	} {
		if result.DirtyLabels[label] {
			t.Errorf("external label %s must not be directly dirty", label)
		}
	}
}

func TestComputeDirtySetPropagatesThroughExternalLabels(t *testing.T) {
	edges := map[string][]string{
		"//pkg:changed":           {"//pkg:source.txt"},
		"@@repo//bridge:external": {"//pkg:changed"},
		"//consumer:transitive":   {"@@repo//bridge:external"},
		"//consumer:direct":       {"//pkg:changed"},
	}
	allLabels := CollectAllLabels(edges, nil)

	result := ComputeDirtySet(
		map[string]string{"pkg/source.txt": "M"}, edges, allLabels, nil,
	)

	for _, label := range []string{
		"//pkg:changed",
		"@@repo//bridge:external",
		"//consumer:transitive",
		"//consumer:direct",
	} {
		if !result.DirtyStarLabels[label] {
			t.Errorf("expected %s in DirtyStarLabels", label)
		}
	}
	if result.DirtyLabels["@@repo//bridge:external"] {
		t.Error("external label should be reached by propagation, not directly dirtied")
	}
}

func TestComputeDirtySetBzlFallback(t *testing.T) {
	edges := map[string][]string{
		"//pkg:rule_a": {},
	}
	allLabels := CollectAllLabels(edges, nil)

	changedFiles := map[string]string{
		"tools/defs.bzl": "M",
	}

	result := ComputeDirtySet(changedFiles, edges, allLabels, nil)

	if !result.NeedsFallback {
		t.Fatal("expected fallback for .bzl change")
	}
	if result.FallbackReason == "" {
		t.Error("expected non-empty FallbackReason")
	}
	if result.FallbackCode != "unsafe_file_change" {
		t.Errorf("FallbackCode = %q, want unsafe_file_change", result.FallbackCode)
	}
}

func TestComputeDirtySetModuleBazelFallback(t *testing.T) {
	changedFiles := map[string]string{
		"MODULE.bazel": "M",
	}

	result := ComputeDirtySet(changedFiles, nil, nil, nil)

	if !result.NeedsFallback {
		t.Fatal("expected fallback for MODULE.bazel change")
	}
}

func TestComputeDirtySetSubModuleBazelFallback(t *testing.T) {
	changedFiles := map[string]string{
		"tools/modules/rules_java.MODULE.bazel": "M",
	}

	result := ComputeDirtySet(changedFiles, nil, nil, nil)

	if !result.NeedsFallback {
		t.Fatal("expected fallback for *.MODULE.bazel change")
	}
}

func TestComputeDirtySetDeletedBUILDFallsBack(t *testing.T) {
	edges := map[string][]string{
		"//deleted_pkg:target": {"//lib:dep"},
		"//app:binary":         {"//deleted_pkg:target"},
		"//lib:dep":            {},
	}
	allLabels := CollectAllLabels(edges, nil)

	changedFiles := map[string]string{
		"deleted_pkg/BUILD.bazel": "D",
	}

	result := ComputeDirtySet(changedFiles, edges, allLabels, nil)

	if !result.NeedsFallback {
		t.Fatal("expected fallback for deleted BUILD file")
	}
	if result.FallbackCode != "package_boundary_change" {
		t.Errorf("FallbackCode = %q, want package_boundary_change", result.FallbackCode)
	}
}

func TestComputeDirtySetRenamedBUILDFallsBack(t *testing.T) {
	edges := map[string][]string{
		"//pkg:target": {},
	}
	allLabels := CollectAllLabels(edges, nil)

	changedFiles := map[string]string{
		"pkg/BUILD.bazel": "R100",
	}

	result := ComputeDirtySet(changedFiles, edges, allLabels, nil)

	if !result.NeedsFallback {
		t.Fatal("expected fallback for renamed BUILD file")
	}
}

func TestComputeDirtySetNewPackageFallsBack(t *testing.T) {
	edges := map[string][]string{
		"//pkg:target": {},
	}
	allLabels := CollectAllLabels(edges, nil)

	changedFiles := map[string]string{
		"newpkg/BUILD.bazel": "A",
	}

	result := ComputeDirtySet(changedFiles, edges, allLabels, nil)

	if !result.NeedsFallback {
		t.Fatal("expected fallback for BUILD file of package unknown to seed")
	}
}

func TestComputeDirtySetOwningPackageWalkUp(t *testing.T) {
	// The file lives several directories below the package's BUILD file, as
	// is typical for Maven-layout Java packages.
	edges := map[string][]string{
		"//svc:lib":    {"//svc:src/main/java/App.java"},
		"//app:binary": {"//svc:lib"},
	}
	allLabels := CollectAllLabels(edges, nil)

	changedFiles := map[string]string{
		"svc/src/main/java/App.java": "M",
	}

	result := ComputeDirtySet(changedFiles, edges, allLabels, nil)

	if result.NeedsFallback {
		t.Fatalf("unexpected fallback: %s", result.FallbackReason)
	}
	if !result.DirtyLabels["//svc:lib"] {
		t.Error("expected //svc:lib dirty via owning-package walk-up")
	}
	if !result.DirtyStarLabels["//app:binary"] {
		t.Error("expected //app:binary in DirtyStarLabels")
	}
	if len(result.DirtyPackages) != 1 || result.DirtyPackages[0] != "//svc" {
		t.Errorf("expected DirtyPackages=[//svc], got %v", result.DirtyPackages)
	}
}

func TestComputeDirtySetUnownedFileIgnored(t *testing.T) {
	edges := map[string][]string{
		"//pkg:target": {},
	}
	allLabels := CollectAllLabels(edges, nil)

	changedFiles := map[string]string{
		"docs/README.md": "M",
	}

	result := ComputeDirtySet(changedFiles, edges, allLabels, nil)

	if result.NeedsFallback {
		t.Fatalf("unexpected fallback: %s", result.FallbackReason)
	}
	if len(result.DirtyLabels) != 0 {
		t.Errorf("expected no dirty labels for unowned file, got %v", result.DirtyLabels)
	}
	if len(result.DirtyPackages) != 0 {
		t.Errorf("expected no dirty packages for unowned file, got %v", result.DirtyPackages)
	}
}

func TestComputeDirtySetDiamondRdeps(t *testing.T) {
	// A→B, A→C, B→D, C→D. Change D → all four dirty*.
	edges := map[string][]string{
		"//pkg:A": {"//pkg:B", "//pkg:C"},
		"//pkg:B": {"//pkg:D"},
		"//pkg:C": {"//pkg:D"},
		"//pkg:D": {"//pkg:d.java"},
	}
	allLabels := CollectAllLabels(edges, nil)

	changedFiles := map[string]string{
		"pkg/d.java": "M",
	}

	result := ComputeDirtySet(changedFiles, edges, allLabels, nil)

	for _, label := range []string{"//pkg:A", "//pkg:B", "//pkg:C", "//pkg:D", "//pkg:d.java"} {
		if !result.DirtyStarLabels[label] {
			t.Errorf("expected %s in DirtyStarLabels", label)
		}
	}
}

func TestComputeDirtySetFingerprintFileFallback(t *testing.T) {
	fingerprints := map[string]bool{
		"tools/modules/rules_java.MODULE.bazel": true,
	}

	changedFiles := map[string]string{
		"tools/modules/rules_java.MODULE.bazel": "M",
	}

	result := ComputeDirtySet(changedFiles, nil, nil, fingerprints)

	if !result.NeedsFallback {
		t.Fatal("expected fallback for fingerprint file change")
	}
}

func TestComputeDirtySetBazelrcFallback(t *testing.T) {
	changedFiles := map[string]string{".bazelrc": "M"}
	result := ComputeDirtySet(changedFiles, nil, nil, nil)
	if !result.NeedsFallback {
		t.Fatal("expected fallback for .bazelrc change")
	}
}

func TestComputeDirtySetBazelVersionFallback(t *testing.T) {
	changedFiles := map[string]string{".bazelversion": "M"}
	result := ComputeDirtySet(changedFiles, nil, nil, nil)
	if !result.NeedsFallback {
		t.Fatal("expected fallback for .bazelversion change")
	}
}

func TestComputeDirtySetBazelIgnoreFallback(t *testing.T) {
	changedFiles := map[string]string{".bazelignore": "M"}
	result := ComputeDirtySet(changedFiles, nil, nil, nil)
	if !result.NeedsFallback {
		t.Fatal("expected fallback for .bazelignore change")
	}
}

func TestComputeDirtySetRepositoryMetadataFallbacks(t *testing.T) {
	for _, path := range []string{
		"WORKSPACE",
		"WORKSPACE.bazel",
		"WORKSPACE.bzlmod",
		"MODULE.bazel.lock",
		"REPO.bazel",
		"VENDOR.bazel",
		"third_party/maven_install.json",
		"third_party/Cargo.lock",
		"web/package-lock.json",
		"python/requirements_lock.txt",
		".gitmodules",
		"config/ci.bazelrc",
		"config/.bazelrc.ci",
		"config/common.rc",
	} {
		t.Run(path, func(t *testing.T) {
			result := ComputeDirtySet(map[string]string{path: "M"}, nil, nil, nil)
			if !result.NeedsFallback {
				t.Fatalf("expected fallback for %s", path)
			}
		})
	}
}

func TestBuildRdeps(t *testing.T) {
	edges := map[string][]string{
		"//a": {"//b", "//c"},
		"//b": {"//c"},
	}

	rdeps := BuildRdeps(edges)

	bRdeps := rdeps["//b"]
	sort.Strings(bRdeps)
	if len(bRdeps) != 1 || bRdeps[0] != "//a" {
		t.Errorf("rdeps of //b: want [//a], got %v", bRdeps)
	}

	cRdeps := rdeps["//c"]
	sort.Strings(cRdeps)
	if len(cRdeps) != 2 || cRdeps[0] != "//a" || cRdeps[1] != "//b" {
		t.Errorf("rdeps of //c: want [//a //b], got %v", cRdeps)
	}
}

func TestCollectAllLabels(t *testing.T) {
	edges := map[string][]string{
		"//a": {"//b"},
	}
	hashes := map[string]map[string]string{
		"//c": {"": "hash"},
	}

	all := CollectAllLabels(edges, hashes)

	for _, lbl := range []string{"//a", "//b", "//c"} {
		if !all[lbl] {
			t.Errorf("expected %s in allLabels", lbl)
		}
	}
}

func TestFileToPackage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"pkg/Foo.java", "//pkg"},
		{"a/b/c/BUILD.bazel", "//a/b/c"},
		{"BUILD.bazel", "//"},
		{"Foo.java", "//"},
	}
	for _, tt := range tests {
		got := fileToPackage(tt.path)
		if got != tt.want {
			t.Errorf("fileToPackage(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestLabelToPackage(t *testing.T) {
	tests := []struct {
		label string
		want  string
	}{
		{"//pkg:target", "//pkg"},
		{"@repo//pkg:target", "//pkg"},
		{"@@canonical_repo//pkg:target", "//pkg"},
		{"//a/b/c:d", "//a/b/c"},
		{"//pkg", "//pkg"},
	}
	for _, tt := range tests {
		got := labelToPackage(tt.label)
		if got != tt.want {
			t.Errorf("labelToPackage(%q) = %q, want %q", tt.label, got, tt.want)
		}
	}
}

func TestPruneDirtySetEliminatesUnchangedRdeps(t *testing.T) {
	// Setup: package //tools/binaries has two aliases. //app:binary depends
	// on //tools/binaries:existing_tool. A BUILD.bazel change makes both
	// aliases directly dirty, propagating to //app:binary.
	edges := map[string][]string{
		"//tools/binaries:existing_tool": {},
		"//tools/binaries:new_tool":      {},
		"//app:binary":                   {"//tools/binaries:existing_tool"},
		"//other:lib":                    {"//app:binary"},
	}
	allLabels := CollectAllLabels(edges, nil)

	changedFiles := map[string]string{
		"tools/binaries/BUILD.bazel": "M",
	}

	original := ComputeDirtySet(changedFiles, edges, allLabels, nil)

	// Verify the unpruned dirty set cascades broadly.
	if !original.DirtyStarLabels["//app:binary"] {
		t.Fatal("expected //app:binary in unpruned DirtyStarLabels")
	}
	if !original.DirtyStarLabels["//other:lib"] {
		t.Fatal("expected //other:lib in unpruned DirtyStarLabels")
	}

	// Seed hashes: existing_tool has hash "aaaa...", new_tool is absent (new target).
	seedHashes := map[string]map[string]string{
		"//tools/binaries:existing_tool": {"": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"//app:binary":                   {"": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		"//other:lib":                    {"": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
	}

	// Probe hashes: existing_tool is UNCHANGED, new_tool is new.
	probeHashes := map[string]string{
		"//tools/binaries:existing_tool\x00": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"//tools/binaries:new_tool\x00":      "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
	}

	pruned := PruneDirtySet(original, seedHashes, edges, probeHashes)

	// Directly dirty labels should be preserved.
	if !pruned.DirtyStarLabels["//tools/binaries:existing_tool"] {
		t.Error("expected //tools/binaries:existing_tool in pruned DirtyStarLabels (still directly dirty)")
	}
	if !pruned.DirtyStarLabels["//tools/binaries:new_tool"] {
		t.Error("expected //tools/binaries:new_tool in pruned DirtyStarLabels (new target, actually changed)")
	}

	// The key assertion: //app:binary and //other:lib should NOT be in the
	// pruned dirty set because existing_tool's hash didn't change.
	// new_tool has no rdeps, so its change doesn't propagate.
	if pruned.DirtyStarLabels["//app:binary"] {
		t.Error("//app:binary should have been pruned — its dep //tools/binaries:existing_tool is unchanged")
	}
	if pruned.DirtyStarLabels["//other:lib"] {
		t.Error("//other:lib should have been pruned — transitive dep unchanged")
	}

	// DirtyPackages should be preserved.
	if len(pruned.DirtyPackages) != 1 || pruned.DirtyPackages[0] != "//tools/binaries" {
		t.Errorf("expected DirtyPackages=[//tools/binaries], got %v", pruned.DirtyPackages)
	}
}

func TestPruneDirtySetPreservesChangedRdeps(t *testing.T) {
	// When a target's hash actually changes, its rdeps must remain dirty.
	edges := map[string][]string{
		"//lib:changed":  {"//lib:src.java"},
		"//lib:same":     {},
		"//app:consumer": {"//lib:changed"},
	}
	allLabels := CollectAllLabels(edges, nil)

	original := ComputeDirtySet(
		map[string]string{"lib/BUILD.bazel": "M"}, edges, allLabels, nil,
	)

	seedHashes := map[string]map[string]string{
		"//lib:changed":  {"": "aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000"},
		"//lib:same":     {"": "bbbb0000bbbb0000bbbb0000bbbb0000bbbb0000bbbb0000bbbb0000bbbb0000"},
		"//lib:src.java": {"": "cccc0000cccc0000cccc0000cccc0000cccc0000cccc0000cccc0000cccc0000"},
		"//app:consumer": {"": "dddd0000dddd0000dddd0000dddd0000dddd0000dddd0000dddd0000dddd0000"},
	}

	// Probe: //lib:changed has a DIFFERENT hash, //lib:same is unchanged.
	probeHashes := map[string]string{
		"//lib:changed\x00":  "ffff0000ffff0000ffff0000ffff0000ffff0000ffff0000ffff0000ffff0000",
		"//lib:same\x00":     "bbbb0000bbbb0000bbbb0000bbbb0000bbbb0000bbbb0000bbbb0000bbbb0000",
		"//lib:src.java\x00": "cccc0000cccc0000cccc0000cccc0000cccc0000cccc0000cccc0000cccc0000",
	}

	pruned := PruneDirtySet(original, seedHashes, edges, probeHashes)

	if !pruned.DirtyStarLabels["//app:consumer"] {
		t.Error("//app:consumer must remain dirty — its dep //lib:changed has a different hash")
	}
}

func TestPruneDirtySetNoOpWhenAllChanged(t *testing.T) {
	edges := map[string][]string{
		"//pkg:a":   {},
		"//app:dep": {"//pkg:a"},
	}
	allLabels := CollectAllLabels(edges, nil)

	original := ComputeDirtySet(
		map[string]string{"pkg/BUILD.bazel": "M"}, edges, allLabels, nil,
	)

	seedHashes := map[string]map[string]string{
		"//pkg:a":   {"": "aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000"},
		"//app:dep": {"": "bbbb0000bbbb0000bbbb0000bbbb0000bbbb0000bbbb0000bbbb0000bbbb0000"},
	}

	// Probe: //pkg:a hash changed.
	probeHashes := map[string]string{
		"//pkg:a\x00": "ffff0000ffff0000ffff0000ffff0000ffff0000ffff0000ffff0000ffff0000",
	}

	pruned := PruneDirtySet(original, seedHashes, edges, probeHashes)

	// Everything should remain dirty — same as unpruned.
	if !pruned.DirtyStarLabels["//app:dep"] {
		t.Error("//app:dep must remain dirty when //pkg:a changed")
	}
}
