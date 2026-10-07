package sf

import (
	"encoding/json"
	"fmt"
	"sort"
)

// MetadataTypeInfo describes one supported metadata type.
type MetadataTypeInfo struct {
	XMLName       string   `json:"xmlName"`
	DirectoryName string   `json:"directoryName"`
	InFolder      bool     `json:"inFolder"`
	MetaFile      bool     `json:"metaFile"`
	ChildXMLNames []string `json:"childXmlNames"`
}

type describeMetadataWrapper struct {
	Result struct {
		MetadataObjects []MetadataTypeInfo `json:"metadataObjects"`
	} `json:"result"`
}

// DescribeMetadataTypes returns every metadata type the org supports,
// sorted by name. Fast (one describeMetadata call, ~1.5s).
func DescribeMetadataTypes(alias string) ([]MetadataTypeInfo, error) {
	out, err := runSF("org", "list", "metadata-types", "--target-org", alias, "--json")
	if err != nil {
		return nil, err
	}
	var parsed describeMetadataWrapper
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("parse metadata-types: %w", err)
	}
	types := parsed.Result.MetadataObjects
	sort.Slice(types, func(i, j int) bool { return types[i].XMLName < types[j].XMLName })
	return types, nil
}
