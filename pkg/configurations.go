package pkg

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/wI2L/jsondiff"
)

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
		configuration := Configuration(c.ConfigHash)
		if _, ok := m[configuration]; ok {
			return nil, fmt.Errorf("saw duplicate configuration for %q", configuration)
		}
		m[configuration] = c
	}
	return m, nil
}
