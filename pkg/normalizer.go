package pkg

import (
	"github.com/bazel-contrib/target-determinator/third_party/protobuf/bazel/build"
	"github.com/bazelbuild/bazel-gazelle/label"
)

// Normalizer is a struct that contains a mapping of non-canonical repository names to canonical repository names.
type Normalizer struct {
	Mapping map[string]string
}

// ParseCanonicalLabel parses a label from a string, and removes sources of inconsequential difference which would make comparing two labels fail.
// In particular, it treats @// the same as //
// If the label is not canonical, it will attempt to map the repository to its canonical form coming from `bazel mod dump_repo_mapping ""`.
func (n *Normalizer) ParseCanonicalLabel(s string) (label.Label, error) {
	l, err := label.Parse(s)
	if err != nil {
		return l, err
	}

	if !l.Canonical && l.Repo != "" {
		mappedValue, ok := n.Mapping[l.Repo]
		if ok && l.Repo != mappedValue {
			l.Repo = mappedValue
			l.Canonical = true
		}
	}

	if l.Repo == "@" {
		l.Repo = ""
	}

	return l, nil
}

func (n *Normalizer) NormalizeAttribute(attr *build.Attribute) *build.Attribute {
	attrType := attr.GetType()

	// An attribute with a nodep property can also hold labels
	// It should be handled as an exception, see https://bazelbuild.slack.com/archives/CDCMRLS23/p1742821059464199
	isNoDepAttribute := attrType == build.Attribute_STRING && attr.Nodep != nil && *attr.Nodep

	if attrType == build.Attribute_OUTPUT || attrType == build.Attribute_LABEL || isNoDepAttribute {
		keyLabel, parseErr := n.ParseCanonicalLabel(attr.GetStringValue())

		if parseErr == nil {
			value := keyLabel.String()
			attr.StringValue = &value
		}
	}

	isNoDepListAttribute := attrType == build.Attribute_STRING_LIST && attr.Nodep != nil && *attr.Nodep

	if attrType == build.Attribute_OUTPUT_LIST || attrType == build.Attribute_LABEL_LIST || isNoDepListAttribute {
		for idx, dep := range attr.GetStringListValue() {
			keyLabel, parseErr := n.ParseCanonicalLabel(dep)

			if parseErr == nil {
				attr.StringListValue[idx] = keyLabel.String()
			}
		}
	}

	if attrType == build.Attribute_LABEL_DICT_UNARY {
		for idx, dep := range attr.GetLabelDictUnaryValue() {
			keyLabel, parseErr := n.ParseCanonicalLabel(*dep.Value)

			if parseErr == nil {
				newValue := keyLabel.String()
				attr.GetLabelDictUnaryValue()[idx].Value = &newValue
			}
		}
	}

	if attrType == build.Attribute_LABEL_LIST_DICT {
		for idx, dep := range attr.GetLabelListDictValue() {
			for key, value := range dep.Value {
				l, parseErr := n.ParseCanonicalLabel(value)

				if parseErr == nil {
					attr.GetLabelListDictValue()[idx].Value[key] = l.String()
				}
			}
		}
	}

	if attrType == build.Attribute_LABEL_KEYED_STRING_DICT {
		for idx, dep := range attr.GetLabelKeyedStringDictValue() {
			keyLabel, parseErr := n.ParseCanonicalLabel(*dep.Key)

			if parseErr == nil {
				newKey := keyLabel.String()
				attr.GetLabelKeyedStringDictValue()[idx].Key = &newKey
			}

		}
	}

	return attr
}
