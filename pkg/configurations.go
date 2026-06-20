package pkg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wI2L/jsondiff"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Configuration struct {
	inner string
}

func NormalizeConfiguration(c string) Configuration {
	if c == "null" {
		return Configuration{
			inner: "",
		}
	}
	return Configuration{
		inner: c,
	}
}

func (c *Configuration) String() string {
	return c.inner
}

func (c *Configuration) ForHashing() []byte {
	return []byte(c.inner)
}

func ConfigurationLess(l, r Configuration) bool {
	return l.inner < r.inner
}

func diffConfigurations(l, r singleConfigurationOutput) (string, error) {
	patch, err := jsondiff.Compare(l, r)
	if err != nil {
		return "", fmt.Errorf("failed to diff configurations %v and %v: %w", l.ConfigHash, r.ConfigHash, err)
	}
	v, err := json.Marshal(patch)
	if err != nil {
		return "", fmt.Errorf("failed to marshal patch diffing configurations %v and %v: %w", l.ConfigHash, r.ConfigHash, err)
	}
	return string(v), nil
}

// singleConfigurationOutput is a JSON-deserializing struct based on the observed output of `bazel config`.
// There are a few extra fields we don't represent, but they don't seem relevant to how we currently interpret the data.
// Feel free to add more in the future!
type singleConfigurationOutput struct {
	ConfigHash      string
	Fragments       json.RawMessage
	FragmentOptions json.RawMessage
}

func getConfigurationDetails(context *Context) (map[Configuration]singleConfigurationOutput, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	returnVal, err := context.BazelCmd.Execute(
		BazelCmdConfig{Dir: context.WorkspacePath, Stdout: &stdout, Stderr: &stderr},
		[]string{"--output_base", context.BazelOutputBase}, "config", "--output=json", "--dump_all")

	if returnVal != 0 || err != nil {
		return nil, fmt.Errorf("failed to run bazel config --output=json --dump_all: %w. Stderr:\n%v", err, stderr.String())
	}

	content := stdout.Bytes()

	var configurations []singleConfigurationOutput
	if err := json.Unmarshal(content, &configurations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config stdout: %w", err)
	}
	m := make(map[Configuration]singleConfigurationOutput)
	for _, c := range configurations {
		configuration := NormalizeConfiguration(c.ConfigHash)
		if _, ok := m[configuration]; ok {
			return nil, fmt.Errorf("saw duplicate configuration for %q", configuration)
		}
		m[configuration] = c
	}
	return m, nil
}

func normalizeConfigurationDetails(context *Context, configurations map[Configuration]singleConfigurationOutput) (map[Configuration]Configuration, map[Configuration]singleConfigurationOutput, error) {
	oldToNew := make(map[Configuration]Configuration, len(configurations))
	normalizedConfigurations := make(map[Configuration]singleConfigurationOutput, len(configurations))
	replacements := rootPathReplacements(context)

	for oldConfiguration, configurationOutput := range configurations {
		normalizedOutput, err := normalizeConfigurationOutput(configurationOutput, replacements)
		if err != nil {
			return nil, nil, err
		}
		configurationID, err := semanticConfigurationID(normalizedOutput)
		if err != nil {
			return nil, nil, err
		}
		newConfiguration := NormalizeConfiguration(configurationID)
		normalizedOutput.ConfigHash = configurationID

		if existing, ok := normalizedConfigurations[newConfiguration]; ok {
			existingJSON, _ := json.Marshal(existing)
			normalizedJSON, _ := json.Marshal(normalizedOutput)
			if !bytes.Equal(existingJSON, normalizedJSON) {
				return nil, nil, fmt.Errorf("configuration normalization collision for %s", configurationID)
			}
		}

		oldToNew[oldConfiguration] = newConfiguration
		normalizedConfigurations[newConfiguration] = normalizedOutput
	}

	return oldToNew, normalizedConfigurations, nil
}

func normalizeConfigurationOutput(configurationOutput singleConfigurationOutput, replacements []rootPathReplacement) (singleConfigurationOutput, error) {
	normalized := configurationOutput
	normalized.ConfigHash = ""

	var err error
	normalized.Fragments, err = normalizeRawJSONStrings(configurationOutput.Fragments, replacements)
	if err != nil {
		return singleConfigurationOutput{}, fmt.Errorf("failed to normalize configuration fragments: %w", err)
	}
	normalized.FragmentOptions, err = normalizeRawJSONStrings(configurationOutput.FragmentOptions, replacements)
	if err != nil {
		return singleConfigurationOutput{}, fmt.Errorf("failed to normalize configuration fragment options: %w", err)
	}

	return normalized, nil
}

func semanticConfigurationID(configurationOutput singleConfigurationOutput) (string, error) {
	data, err := json.Marshal(configurationOutput)
	if err != nil {
		return "", fmt.Errorf("failed to marshal normalized configuration: %w", err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

type rootPathReplacement struct {
	from string
	to   string
}

func rootPathReplacements(context *Context) []rootPathReplacement {
	var replacements []rootPathReplacement
	addReplacement := func(from string, to string) {
		if from == "" {
			return
		}
		from = filepath.Clean(from)
		for _, replacement := range replacements {
			if replacement.from == from {
				return
			}
		}
		replacements = append(replacements, rootPathReplacement{from: from, to: to})
		if evaluated, err := filepath.EvalSymlinks(from); err == nil && evaluated != from {
			replacements = append(replacements, rootPathReplacement{from: evaluated, to: to})
		}
	}

	addReplacement(context.WorkspacePath, "${TARGET_DETERMINATOR_WORKSPACE}")
	addReplacement(context.BazelOutputBase, "${TARGET_DETERMINATOR_OUTPUT_BASE}")
	return replacements
}

func normalizeRawJSONStrings(raw json.RawMessage, replacements []rootPathReplacement) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}

	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	normalized := normalizeJSONStrings(value, replacements)
	return json.Marshal(normalized)
}

func normalizeJSONStrings(value interface{}, replacements []rootPathReplacement) interface{} {
	switch typed := value.(type) {
	case string:
		return normalizeRootPathsInString(typed, replacements)
	case []interface{}:
		for idx, item := range typed {
			typed[idx] = normalizeJSONStrings(item, replacements)
		}
		return typed
	case map[string]interface{}:
		for key, item := range typed {
			typed[key] = normalizeJSONStrings(item, replacements)
		}
		return typed
	default:
		return value
	}
}

func normalizeRootPathsInString(value string, replacements []rootPathReplacement) string {
	for _, replacement := range replacements {
		value = strings.ReplaceAll(value, replacement.from, replacement.to)
	}
	return value
}

func normalizeProtoStringFields(message proto.Message, replacements []rootPathReplacement) {
	if message == nil || len(replacements) == 0 {
		return
	}

	protoMessage := message.ProtoReflect()
	protoMessage.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		switch {
		case field.IsList():
			normalizeProtoListStrings(value.List(), field, replacements)
		case field.IsMap():
			normalizeProtoMapStrings(value.Map(), field, replacements)
		case field.Kind() == protoreflect.StringKind:
			protoMessage.Set(field, protoreflect.ValueOfString(normalizeRootPathsInString(value.String(), replacements)))
		case field.Kind() == protoreflect.MessageKind || field.Kind() == protoreflect.GroupKind:
			normalizeProtoStringFields(value.Message().Interface(), replacements)
		}
		return true
	})
}

func normalizeProtoListStrings(list protoreflect.List, field protoreflect.FieldDescriptor, replacements []rootPathReplacement) {
	switch field.Kind() {
	case protoreflect.StringKind:
		for idx := 0; idx < list.Len(); idx++ {
			list.Set(idx, protoreflect.ValueOfString(normalizeRootPathsInString(list.Get(idx).String(), replacements)))
		}
	case protoreflect.MessageKind, protoreflect.GroupKind:
		for idx := 0; idx < list.Len(); idx++ {
			normalizeProtoStringFields(list.Get(idx).Message().Interface(), replacements)
		}
	}
}

func normalizeProtoMapStrings(protoMap protoreflect.Map, field protoreflect.FieldDescriptor, replacements []rootPathReplacement) {
	valueField := field.MapValue()
	switch valueField.Kind() {
	case protoreflect.StringKind:
		protoMap.Range(func(key protoreflect.MapKey, value protoreflect.Value) bool {
			protoMap.Set(key, protoreflect.ValueOfString(normalizeRootPathsInString(value.String(), replacements)))
			return true
		})
	case protoreflect.MessageKind, protoreflect.GroupKind:
		protoMap.Range(func(_ protoreflect.MapKey, value protoreflect.Value) bool {
			normalizeProtoStringFields(value.Message().Interface(), replacements)
			return true
		})
	}
}
