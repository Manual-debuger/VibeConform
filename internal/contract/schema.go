package contract

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Schema is the vibe.yaml schema a configuration file is written in.
type Schema int

// The schemas vibe audit tells apart (spec 0052, "Dispatch on the
// configuration").
const (
	// SchemaUnknown is returned with an error: the file is neither.
	SchemaUnknown Schema = iota
	// SchemaV1 has a top-level standard key: the removed template-sync
	// system's file, which vibe refuses with the migration pointer.
	SchemaV1
	// SchemaV2 has a top-level contract key and runs the contract audit.
	SchemaV2
)

// ConfigError is an invalid configuration. Its message is
// "<file>: <field path>: <why>", or "<file>: <why>" for an error about
// the file as a whole.
type ConfigError struct {
	// File is the configuration file as the operator named it.
	File string
	// Path is the field path, such as entrypoints.verify[2] or
	// waivers[0].until; empty for the file as a whole.
	Path string
	// Reason says why the field is invalid.
	Reason string
}

func (e *ConfigError) Error() string {
	if e.Path == "" {
		return e.File + ": " + e.Reason
	}
	return e.File + ": " + e.Path + ": " + e.Reason
}

// DetectSchema reads only the top-level keys of a configuration file and
// says which schema it is written in. Both keys, or neither, is an error
// that names the two.
func DetectSchema(file string, data []byte) (Schema, error) {
	top, err := decodeTop(file, data)
	if err != nil {
		return SchemaUnknown, err
	}
	var hasContract, hasStandard bool
	if top != nil {
		for i := 0; i+1 < len(top.Content); i += 2 {
			switch top.Content[i].Value {
			case "contract":
				hasContract = true
			case "standard":
				hasStandard = true
			}
		}
	}
	switch {
	case hasContract && hasStandard:
		return SchemaUnknown, &ConfigError{File: file, Reason: "has both contract (version 2) and standard (version 1); " +
			"keep contract for the contract audit or standard for the old audit"}
	case hasContract:
		return SchemaV2, nil
	case hasStandard:
		return SchemaV1, nil
	default:
		return SchemaUnknown, &ConfigError{File: file, Reason: "has neither contract (version 2) nor standard (version 1); " +
			"declare the contract version, such as contract: 1.0.0"}
	}
}

// decodeTop decodes a configuration file into its top-level mapping node.
// An empty file, or one holding only comments, has none: it returns nil.
func decodeTop(file string, data []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, &ConfigError{File: file, Reason: fmt.Sprintf("not valid YAML: %v", err)}
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, &ConfigError{File: file, Reason: "holds more than one YAML document; keep one"}
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	top := resolve(doc.Content[0])
	if top.Kind == yaml.ScalarNode && top.Tag == "!!null" {
		return nil, nil
	}
	if top.Kind != yaml.MappingNode {
		return nil, &ConfigError{File: file, Reason: "must be a map of fields, such as contract: 1.0.0"}
	}
	return top, nil
}

// resolve follows a YAML alias to the node it names.
func resolve(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}
