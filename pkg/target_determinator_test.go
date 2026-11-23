package pkg

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/bazel-contrib/target-determinator/common"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/analysis"
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/build"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/proto"
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

// fakeCqueryBazelCmd writes output as the cquery's stdout.
type fakeCqueryBazelCmd struct {
	fakeBazelCmd
	output []byte
}

func (f fakeCqueryBazelCmd) Cquery(_ string, config BazelCmdConfig, _ []string, _ ...string) (int, error) {
	if _, err := config.Stdout.Write(f.output); err != nil {
		return 1, err
	}
	return 0, nil
}

func streamedCqueryOutput(t *testing.T, names ...[]string) []byte {
	t.Helper()
	var output bytes.Buffer
	for _, batch := range names {
		var result analysis.CqueryResult
		for _, name := range batch {
			result.Results = append(result.Results, &analysis.ConfiguredTarget{
				Target: &build.Target{
					Type: build.Target_RULE.Enum(),
					Rule: &build.Rule{Name: proto.String(name), RuleClass: proto.String("genrule")},
				},
			})
		}
		if _, err := protodelim.MarshalTo(&output, &result); err != nil {
			t.Fatalf("MarshalTo: %v", err)
		}
	}
	return output.Bytes()
}

func TestRunToCqueryResultDecodesStreamedProto(t *testing.T) {
	context := &Context{BazelCmd: fakeCqueryBazelCmd{
		output: streamedCqueryOutput(t, []string{"//a:a", "//b:b"}, []string{"//c:c"}),
	}}
	targets, err := runToCqueryResult(context, "//...", false, "release 8.2.0")
	if err != nil {
		t.Fatalf("runToCqueryResult: %v", err)
	}
	var got []string
	for _, target := range targets {
		got = append(got, target.GetTarget().GetRule().GetName())
	}
	if want := []string{"//a:a", "//b:b", "//c:c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got targets %v, want %v", got, want)
	}
}

func TestRunToCqueryResultFailsOnMalformedStreamedProto(t *testing.T) {
	// A length prefix larger than the rest of the output, followed by more output than
	// a pipe write can hand over at once.
	output := append([]byte{0xff, 0xff, 0xff, 0x7f}, make([]byte, 1<<20)...)
	context := &Context{BazelCmd: fakeCqueryBazelCmd{output: output}}
	if _, err := runToCqueryResult(context, "//...", false, "release 8.2.0"); err == nil {
		t.Fatal("expected an error for malformed cquery output")
	}
}
