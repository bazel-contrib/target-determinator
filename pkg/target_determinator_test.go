package pkg

import (
	"testing"

	"github.com/bazel-contrib/target-determinator/common"
)

func Test_stringSliceContainsStartingWith(t *testing.T) {
	type args struct {
		slice   []common.RelPath
		element common.RelPath
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			"containsExact",
			args{
				[]common.RelPath{common.NewRelPath("foo")},
				common.NewRelPath("foo"),
			},
			true,
		},
		{
			"containsDirWithoutTrailingSlash",
			args{
				[]common.RelPath{common.NewRelPath("foo"), common.NewRelPath("bar/baz")},
				common.NewRelPath("foo/"),
			},
			true,
		},
		{
			"containsDirWithTrailingSlashButIsFile",
			args{
				[]common.RelPath{common.NewRelPath("foo/")},
				common.NewRelPath("foo"),
			},
			false,
		},
		{
			"containsPrefix",
			args{
				[]common.RelPath{common.NewRelPath("foo")},
				common.NewRelPath("foo/bar"),
			},
			true,
		},
		{
			"otherIsPrefix",
			args{
				[]common.RelPath{common.NewRelPath("foo/bar")},
				common.NewRelPath("foo"),
			},
			false,
		},
		{
			"doesNotContain",
			args{
				[]common.RelPath{common.NewRelPath("foo"), common.NewRelPath("bar/baz")},
				common.NewRelPath("frob"),
			},
			false,
		},
		{
			"stringPrefixButNotPathPrefix",
			args{
				[]common.RelPath{common.NewRelPath("foo/b")},
				common.NewRelPath("foo/bar"),
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stringSliceContainsStartingWith(tt.args.slice, tt.args.element); got != tt.want {
				t.Errorf("stringSliceContainsStartingWith() with (slice = %v, element = %v) returns %v, want %v",
					tt.args.slice, tt.args.element.String(), got, tt.want)
			}
		})
	}
}

func Test_ParseCanonicalLabel(t *testing.T) {
	n := Normalizer{}
	for _, tt := range []string{
		"@//label",
		"@//label:package",
		"//label:package",
		":package",
		"@rules_python~0.21.0~pip~pip_boto3//:pkg",
	} {
		_, err := n.ParseCanonicalLabel(tt)
		if err != nil {
			t.Errorf("ParseCanonicalLabel() with (label=%s) produces error %s", tt, err)
		}
	}
}

func TestReleaseConfiguredTargetsKeepsPrefilledHashes(t *testing.T) {
	const bazelRelease = "release 5.1.1"
	labelAndConfiguration := LabelAndConfiguration{
		Label:         mustParseLabel("//HelloWorld:HelloWorld"),
		Configuration: NormalizeConfiguration(configurationChecksum),
	}

	_, cqueryResult := layoutProject(t)
	normalizer := Normalizer{}
	configuredTargets, err := ParseCqueryResult(cqueryResult.Results, &normalizer)
	if err != nil {
		t.Fatalf("Failed to parse cquery result: %v", err)
	}
	queryInfo := &QueryResults{
		MatchingTargets:             makeMatchingTargets(labelAndConfiguration.Label, labelAndConfiguration.Configuration),
		TransitiveConfiguredTargets: configuredTargets,
		TargetHashCache:             NewTargetHashCache(configuredTargets, &normalizer, bazelRelease),
		BazelRelease:                bazelRelease,
	}
	if err := queryInfo.PrefillCache(); err != nil {
		t.Fatalf("PrefillCache: %v", err)
	}
	want, err := queryInfo.TargetHashCache.Hash(labelAndConfiguration)
	if err != nil {
		t.Fatalf("Hash before release: %v", err)
	}

	queryInfo.releaseConfiguredTargets()

	if queryInfo.TransitiveConfiguredTargets != nil || queryInfo.TargetHashCache.context != nil {
		t.Fatalf("configured targets are still referenced after release")
	}
	got, err := queryInfo.TargetHashCache.Hash(labelAndConfiguration)
	if err != nil {
		t.Fatalf("Hash after release: %v", err)
	}
	if !areHashesEqual(want, got) {
		t.Errorf("hash changed after release: %x != %x", want, got)
	}
}
