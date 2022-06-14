package pkg_test

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bazel-contrib/target-determinator/pkg"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/analysis"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/build"
	gazelle_label "github.com/bazelbuild/bazel-gazelle/label"
	"github.com/otiai10/copy"
	"google.golang.org/protobuf/proto"
)

const configurationChecksum = "eed618a573b916b7c6c94b04a4aef1da8c0ebce4c6312065c8b0360fedd8deb9"

func TestAbsolutifiesSourceFileInBuildDirBazel4(t *testing.T) {
	target := build.Target{
		Type: build.Target_SOURCE_FILE.Enum(),
		SourceFile: &build.SourceFile{
			Name:            proto.String("//java/example/simple:Dep.java"),
			Location:        proto.String("/some/path/to/java/example/simple/BUILD.bazel:11:20"),
			VisibilityLabel: []string{"//visibility:private"},
		},
	}
	const want = "/some/path/to/java/example/simple/Dep.java"

	got := pkg.AbsolutePath(&target)

	if want != got {
		t.Errorf("Wrong absolute path: want %v got %v", want, got)
	}
}

func TestAbsolutifiesSourceFileInNestedDirBazel4(t *testing.T) {
	target := build.Target{
		Type: build.Target_SOURCE_FILE.Enum(),
		SourceFile: &build.SourceFile{
			Name:            proto.String("//java/example/simple:just/a/File.java"),
			Location:        proto.String("/some/path/to/java/example/simple/BUILD.bazel:11:20"),
			VisibilityLabel: []string{"//visibility:private"},
		},
	}
	const want = "/some/path/to/java/example/simple/just/a/File.java"

	got := pkg.AbsolutePath(&target)

	if want != got {
		t.Errorf("Wrong absolute path: want %v got %v", want, got)
	}
}

func TestAbsolutifiesSourceFileInBuildDirBazel5(t *testing.T) {
	target := build.Target{
		Type: build.Target_SOURCE_FILE.Enum(),
		SourceFile: &build.SourceFile{
			Name:            proto.String("//java/example/simple:Dep.java"),
			Location:        proto.String("/some/path/to/java/example/simple/Dep.java:1:1"),
			VisibilityLabel: []string{"//visibility:private"},
		},
	}
	const want = "/some/path/to/java/example/simple/Dep.java"

	got := pkg.AbsolutePath(&target)

	if want != got {
		t.Errorf("Wrong absolute path: want %v got %v", want, got)
	}
}

func TestAbsolutifiesSourceFileInNestedDirBazel5(t *testing.T) {
	target := build.Target{
		Type: build.Target_SOURCE_FILE.Enum(),
		SourceFile: &build.SourceFile{
			Name:            proto.String("//java/example/simple:just/a/File.java"),
			Location:        proto.String("/some/path/to/java/example/simple/just/a/File.java:1:1"),
			VisibilityLabel: []string{"//visibility:private"},
		},
	}
	const want = "/some/path/to/java/example/simple/just/a/File.java"

	got := pkg.AbsolutePath(&target)

	if want != got {
		t.Errorf("Wrong absolute path: want %v got %v", want, got)
	}
}

// Before Bazel 5, BUILD.bazel files didn't have line and column information in their Locations.
// Test that we handle this ok.
func TestAbsolutifiesBuildFile(t *testing.T) {
	target := build.Target{
		Type: build.Target_SOURCE_FILE.Enum(),
		SourceFile: &build.SourceFile{
			Name:            proto.String("//java/example/simple:BUILD.bazel"),
			Location:        proto.String("/some/path/to/BUILD.bazel"),
			VisibilityLabel: []string{"//visibility:private"},
		},
	}
	const want = "/some/path/to/BUILD.bazel"

	got := pkg.AbsolutePath(&target)

	if want != got {
		t.Errorf("Wrong absolute path: want %v got %v", want, got)
	}
}

func TestDigestsSingleSourceFile(t *testing.T) {
	_, cqueryResult := layoutProject(t)
	thc := parseResult(t, cqueryResult)

	hash, err := thc.Hash(pkg.LabelAndConfiguration{
		Label: mustParseLabel("//HelloWorld:HelloWorld.java"),
	})
	if err != nil {
		t.Fatalf("Error hashing file: %v", err)
	}
	const want = "344b8720292ecb15157962017a16417b30ed3463b7787c55d411be08bf780a3b"
	got := hex.EncodeToString(hash)
	if want != got {
		t.Errorf("Wrong hash: want %v got %v", want, got)
	}
}

// Labels may be referred to without existing, and at loading time these are assumed
// to be input files, even if no such file exists.
// https://github.com/bazelbuild/bazel/issues/14611
func TestDigestingMissingSourceFileIsNotError(t *testing.T) {
	_, cqueryResult := layoutProject(t)
	thc := parseResult(t, cqueryResult)

	_, err := thc.Hash(pkg.LabelAndConfiguration{
		Label: mustParseLabel("//HelloWorld:ThereIsNoWorld.java"),
	})
	if err != nil {
		t.Fatalf("Error hashing file: %v", err)
	}
}

// Directories (spuriously) listed in srcs show up a SOURCE_FILEs.
// We don't error on this, as Bazel doesn't, but we also don't manually walk the
// directory (as globs should have been used in the BUILD file if this was the intent).
// When this gets mixed into other hashes, that mixing in includes the target name, so
// this sentinel "empty hash" vaguely indicates that a directory occurred.
// We may want to do something more structured here at some point.
// See https://github.com/bazelbuild/bazel/issues/14678
func TestDigestingDirectoryIsNotError(t *testing.T) {
	_, cqueryResult := layoutProject(t)
	thc := parseResult(t, cqueryResult)

	_, err := thc.Hash(pkg.LabelAndConfiguration{
		Label: mustParseLabel("//HelloWorld:InhabitedPlanets"),
	})
	if err != nil {
		t.Fatalf("Error hashing directory: %v", err)
	}
}

func TestDigestTree(t *testing.T) {
	//  HelloWorld -> GreetingLib -> Greeting.java
	//       |
	//       v
	// HelloWorld.java

	label := mustParseLabel("//HelloWorld:HelloWorld")

	labelAndConfiguration := pkg.LabelAndConfiguration{
		Label:         label,
		Configuration: configurationChecksum,
	}

	_, cqueryResult := layoutProject(t)
	thc := parseResult(t, cqueryResult)

	originalHash, err := thc.Hash(labelAndConfiguration)
	if err != nil {
		t.Fatalf("Failed to get original hash: %v", err)
	}

	_, cqueryResult = layoutProject(t)
	thc = parseResult(t, cqueryResult)

	differentDirHash, err := thc.Hash(labelAndConfiguration)
	if err != nil {
		t.Fatalf("Failed to get different dir hash: %v", err)
	}

	if !areHashesEqual(originalHash, differentDirHash) {
		t.Errorf("Wanted original hash and different dir hash to different but were same: %v", hex.EncodeToString(originalHash))
	}

	projectDir, cqueryResult := layoutProject(t)
	thc = parseResult(t, cqueryResult)
	if err := ioutil.WriteFile(filepath.Join(projectDir, "HelloWorld.java"), []byte("Not valid java!"), 0o666); err != nil {
		t.Fatalf("Failed to write changed HelloWorld.java: %v", err)
	}

	changedDirectFileHash, err := thc.Hash(labelAndConfiguration)
	if err != nil {
		t.Fatalf("Failed to get changed direct file hash: %v", err)
	}

	if areHashesEqual(originalHash, changedDirectFileHash) {
		t.Errorf("Wanted original hash and changed direct file hash to be different but were same: %v", hex.EncodeToString(originalHash))
	}

	projectDir, cqueryResult = layoutProject(t)
	thc = parseResult(t, cqueryResult)
	if err := ioutil.WriteFile(filepath.Join(projectDir, "Greeting.java"), []byte("Also not valid java!"), 0o666); err != nil {
		t.Fatalf("Failed to write changed Greeting.java: %v", err)
	}

	changedTransitiveFileHash, err := thc.Hash(labelAndConfiguration)
	if err != nil {
		t.Fatalf("Failed to get changed transitive file hash: %v", err)
	}

	if areHashesEqual(originalHash, changedTransitiveFileHash) {
		t.Errorf("Wanted original hash and changed transitive file hash to be different but were same: %v", hex.EncodeToString(originalHash))
	}

	// Remove dep on GreetingLib
	projectDir, cqueryResult = layoutProject(t)
	cqueryResult.Results[0].GetTarget().GetRule().RuleInput = []string{"//HelloWorld:HelloWorld.java"}
	thc = parseResult(t, cqueryResult)

	removedDepFileHash, err := thc.Hash(labelAndConfiguration)
	if err != nil {
		t.Fatalf("Failed to get removed dep file hash: %v", err)
	}

	// Still no dep on GreetingLib
	if err := ioutil.WriteFile(filepath.Join(projectDir, "Greeting.java"), []byte("Also not valid java!"), 0o666); err != nil {
		t.Fatalf("Failed to write changed Greeting.java: %v", err)
	}
	thc = parseResult(t, cqueryResult)

	changedTransitiveFileFromRemovedDepHash, err := thc.Hash(labelAndConfiguration)
	if err != nil {
		t.Fatalf("Failed to get changed transitive file hash: %v", err)
	}

	if !areHashesEqual(removedDepFileHash, changedTransitiveFileFromRemovedDepHash) {
		t.Errorf("Wanted removed dep hash and changed transitive file from removed dep hash to be the same (because file is no longer depended on), but were different. Removed dep hash: %v, Changed transitive file hash: %v", hex.EncodeToString(removedDepFileHash), hex.EncodeToString(changedTransitiveFileFromRemovedDepHash))
	}
}

func layoutProject(t *testing.T) (string, *analysis.CqueryResult) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("Failed to create temporary directory to layout project: %v", err)
	}

	pwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting working directory to layout project: %v", err)
	}

	if err := copy.Copy(filepath.Join(pwd, "testdata/HelloWorld"), dir, copy.Options{
		OnSymlink: func(name string) copy.SymlinkAction {
			return copy.Deep
		},
		PermissionControl: copy.DoNothing,
	}); err != nil {
		t.Fatalf("Error copying project to temporary directory: %v", err)
	}

	configuration := &analysis.Configuration{
		Checksum: configurationChecksum,
	}

	cqueryResult := analysis.CqueryResult{
		Results: []*analysis.ConfiguredTarget{
			{
				Target: &build.Target{
					Type: build.Target_RULE.Enum(),
					Rule: &build.Rule{
						Name:      proto.String("//HelloWorld:HelloWorld"),
						RuleClass: proto.String("java_binary"),
						Location:  proto.String(fmt.Sprintf("%s/BUILD.bazel:1:12", dir)),
						RuleInput: []string{
							"//HelloWorld:GreetingLib",
							"//HelloWorld:HelloWorld.java",
						},
					},
				},
				Configuration: configuration,
			},
			{
				Target: &build.Target{
					Type: build.Target_RULE.Enum(),
					Rule: &build.Rule{
						Name:      proto.String("//HelloWorld:GreetingLib"),
						RuleClass: proto.String("java_library"),
						Location:  proto.String(fmt.Sprintf("%s/BUILD.bazel:8:13", dir)),
						RuleInput: []string{
							"//HelloWorld:Greeting.java",
						},
					},
				},
				Configuration: configuration,
			},
			{
				Target: &build.Target{
					Type: build.Target_SOURCE_FILE.Enum(),
					SourceFile: &build.SourceFile{
						Name:     proto.String("//HelloWorld:HelloWorld.java"),
						Location: proto.String(fmt.Sprintf("%s/BUILD.bazel:1:12", dir)),
					},
				},
			},
			{
				Target: &build.Target{
					Type: build.Target_SOURCE_FILE.Enum(),
					SourceFile: &build.SourceFile{
						Name:     proto.String("//HelloWorld:Greeting.java"),
						Location: proto.String(fmt.Sprintf("%s/BUILD.bazel:8:13", dir)),
					},
				},
			},
			{
				Target: &build.Target{
					Type: build.Target_SOURCE_FILE.Enum(),
					SourceFile: &build.SourceFile{
						Name:     proto.String("//HelloWorld:InhabitedPlanets"),
						Location: proto.String(fmt.Sprintf("%s/HelloWorld/ThereIsNoFile.java:1:1", dir)),
					},
				},
			},
			{
				Target: &build.Target{
					Type: build.Target_SOURCE_FILE.Enum(),
					SourceFile: &build.SourceFile{
						Name:     proto.String("//HelloWorld:ThereIsNoWorld.java"),
						Location: proto.String(fmt.Sprintf("%s/HelloWorld/ThereIsNoWorld.java:1:1", dir)),
					},
				},
			},
		},
	}

	return dir, &cqueryResult
}

func parseResult(t *testing.T, result *analysis.CqueryResult) *pkg.TargetHashCache {
	cqueryResult, err := pkg.ParseCqueryResult(result)
	if err != nil {
		t.Fatalf("Failed to parse cquery result: %v", err)
	}
	return pkg.NewTargetHashCache(cqueryResult)
}

func areHashesEqual(left, right []byte) bool {
	return reflect.DeepEqual(left, right)
}

func mustParseLabel(s string) gazelle_label.Label {
	l, err := gazelle_label.Parse(s)
	if err != nil {
		panic(err)
	}
	return l
}
