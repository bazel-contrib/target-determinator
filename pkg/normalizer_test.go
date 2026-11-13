package pkg

import (
	"testing"

	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/build"
)

const (
	NonCanonicalPackage = "org_golang_x_text"
	CanonicalPackage    = "gazelle++go_deps+org_golang_x_text"

	NonCanonicalLabel = "@org_golang_x_text//pkg:target"
	CanonicalLabel    = "@@gazelle++go_deps+org_golang_x_text//pkg:target"

	DummyLabel = "@dummy//pkg:target"
)

func TestParseCanonicalLabel(t *testing.T) {
	n := Normalizer{
		Mapping: map[string]string{
			NonCanonicalPackage: CanonicalPackage,
		},
	}

	label, err := n.ParseCanonicalLabel(NonCanonicalLabel)

	if err != nil {
		t.Fatalf("Error parsing label: %v", err)
	}

	if label.String() != CanonicalLabel {
		t.Fatalf("Expected label to be %s, got %v", CanonicalLabel, label.String())
	}
}

func TestParseCanonicalLabelSameApparentAndCanonicalName(t *testing.T) {
	n := Normalizer{
		Mapping: map[string]string{
			"platforms": "platforms",
		},
	}

	apparent, err := n.ParseCanonicalLabel("@platforms//os:linux")
	if err != nil {
		t.Fatalf("Error parsing label: %v", err)
	}
	canonical, err := n.ParseCanonicalLabel("@@platforms//os:linux")
	if err != nil {
		t.Fatalf("Error parsing label: %v", err)
	}

	if apparent != canonical {
		t.Fatalf("Expected %v and %v to be equal", apparent, canonical)
	}
	if apparent.String() != "@@platforms//os:linux" {
		t.Fatalf("Expected label to be @@platforms//os:linux, got %v", apparent.String())
	}
}

func TestParseCanonicalLabelMainRepo(t *testing.T) {
	n := Normalizer{
		Mapping: map[string]string{
			"my_module": "",
		},
	}

	label, err := n.ParseCanonicalLabel("@my_module//pkg:target")
	if err != nil {
		t.Fatalf("Error parsing label: %v", err)
	}

	if label.String() != "//pkg:target" {
		t.Fatalf("Expected label to be //pkg:target, got %v", label.String())
	}
}

func toPtr[T any](x T) *T {
	return &x
}

func equal[S ~[]E, E comparable](s1, s2 S) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := range s1 {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

func TestNormalizeAttributes(t *testing.T) {
	n := Normalizer{
		Mapping: map[string]string{
			NonCanonicalPackage: CanonicalPackage,
		},
	}

	testCases := map[string]func(*testing.T){
		"nodep_string": func(t *testing.T) {
			attr := &build.Attribute{
				Name:        toPtr("visibility"),
				Type:        toPtr(build.Attribute_STRING),
				Nodep:       toPtr(true),
				StringValue: toPtr(NonCanonicalLabel),
			}

			norm := n.NormalizeAttribute(attr)

			equality := *norm.StringValue == CanonicalLabel

			if !equality {
				t.Fatalf("Expected string value to be %v, got %v", CanonicalLabel, *norm.StringValue)
			}
		}, "nodep_string_list": func(t *testing.T) {
			values := []string{
				NonCanonicalLabel,
				DummyLabel,
			}

			expectedValues := []string{CanonicalLabel, DummyLabel}

			attr := &build.Attribute{
				Name:            toPtr("visibility"),
				Type:            toPtr(build.Attribute_STRING_LIST),
				Nodep:           toPtr(true),
				StringListValue: values,
			}

			norm := n.NormalizeAttribute(attr)

			equality := equal(
				norm.StringListValue,
				expectedValues,
			)

			if !equality {
				t.Fatalf("Expected string list values to be %v, got %v", expectedValues, values)
			}
		},
		"label_list": func(t *testing.T) {
			values := []string{
				NonCanonicalLabel,
				DummyLabel,
			}

			expectedValues := []string{CanonicalLabel, DummyLabel}

			attr := &build.Attribute{
				Name:            toPtr("label_list"),
				Type:            toPtr(build.Attribute_LABEL_LIST),
				StringListValue: values,
			}

			norm := n.NormalizeAttribute(attr)

			equality := equal(
				norm.StringListValue,
				expectedValues,
			)

			if !equality {
				t.Fatalf("Expected string list values to be %v, got %v", expectedValues, values)
			}
		},
		"output_list": func(t *testing.T) {
			values := []string{
				NonCanonicalLabel,
				DummyLabel,
			}

			expectedValues := []string{CanonicalLabel, DummyLabel}

			attr := &build.Attribute{
				Name:            toPtr("output_list"),
				Type:            toPtr(build.Attribute_OUTPUT_LIST),
				StringListValue: values,
			}

			norm := n.NormalizeAttribute(attr)

			equality := equal(
				norm.StringListValue,
				expectedValues,
			)

			if !equality {
				t.Fatalf("Expected string list values to be %v, got %v", expectedValues, values)
			}
		},
		"label_dict_unary": func(t *testing.T) {
			attr := &build.Attribute{
				Name: toPtr("label_dict_unary"),
				Type: toPtr(build.Attribute_LABEL_DICT_UNARY),
				LabelDictUnaryValue: []*build.LabelDictUnaryEntry{
					{
						Key:   toPtr("key"),
						Value: toPtr(NonCanonicalLabel),
					},
				},
			}

			norm := n.NormalizeAttribute(attr)
			value := *norm.GetLabelDictUnaryValue()[0].Value
			if value != CanonicalLabel {
				t.Fatalf("Expected value to be %v, got %v", CanonicalLabel, value)
			}
		},
		"label_list_dict": func(t *testing.T) {
			values := []string{
				NonCanonicalLabel,
				DummyLabel,
			}
			expectedValues := []string{CanonicalLabel, DummyLabel}

			attr := &build.Attribute{
				Name: toPtr("label_list_dict"),
				Type: toPtr(build.Attribute_LABEL_LIST_DICT),
				LabelListDictValue: []*build.LabelListDictEntry{
					{
						Key:   toPtr("key"),
						Value: values,
					},
				},
			}

			norm := n.NormalizeAttribute(attr)
			equality := equal(
				norm.LabelListDictValue[0].Value,
				expectedValues,
			)

			if !equality {
				t.Fatalf("Expected label list dict values to be %v, got %v", expectedValues, values)
			}
		},
		"label_keyed_string_dict": func(t *testing.T) {
			attr := &build.Attribute{
				Name: toPtr("label_keyed_string_list"),
				Type: toPtr(build.Attribute_LABEL_KEYED_STRING_DICT),
				LabelKeyedStringDictValue: []*build.LabelKeyedStringDictEntry{
					{
						Key:   toPtr(NonCanonicalLabel),
						Value: toPtr("value"),
					},
				},
			}

			norm := n.NormalizeAttribute(attr)
			value := *norm.GetLabelKeyedStringDictValue()[0].Key
			if value != CanonicalLabel {
				t.Fatalf("Expected value to be %v, got %v", CanonicalLabel, value)
			}
		},
	}

	for test, tt := range testCases {
		t.Run(test, tt)
	}

}
