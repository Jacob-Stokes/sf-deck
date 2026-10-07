package sf

// Metadata API retrieve — the project-free path used by the /compare
// "Metadata API" route (and the Auto-route fallback for types Tooling
// can't serve).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MetadataItem is one component as reported by `sf org list metadata`.
type MetadataItem struct {
	FullName         string `json:"fullName"`
	Type             string `json:"type"`
	NamespacePrefix  string `json:"namespacePrefix"`
	LastModifiedDate string `json:"lastModifiedDate"`
}

type listMetadataWrapper struct {
	Status int            `json:"status"`
	Result []MetadataItem `json:"result"`
}

// MetadataListByType enumerates every component of one metadata type in
// the org (project-free; shells `sf org list metadata`). Managed-package
// components are included; callers filter on NamespacePrefix if wanted.
func MetadataListByType(alias, metadataType string) ([]MetadataItem, error) {
	out, err := runSF("org", "list", "metadata",
		"--metadata-type", metadataType,
		"--target-org", alias,
		"--json",
	)
	if err != nil {
		return nil, err
	}
	var parsed listMetadataWrapper
	if err := json.Unmarshal(out, &parsed); err != nil {
		var bare []MetadataItem
		if err2 := json.Unmarshal(out, &bare); err2 == nil {
			return bare, nil
		}
		return nil, fmt.Errorf("parse list metadata: %w", err)
	}
	return parsed.Result, nil
}

// RetrieveMetadataXML retrieves the source for the named members of one
// metadata type and returns a map of member fullName → raw file
// contents (XML / source). Project-free: writes a minimal sfdx project
// to a temp dir, runs `sf project retrieve`, reads the files back, and
// removes the temp dir.
//
// members empty → retrieves the whole type ("Type" wildcard). For large
// types prefer passing explicit members (the inventory already knows
// them) to bound the retrieve.
func RetrieveMetadataXML(alias, metadataType string, members []string) (map[string]string, error) {
	dir, err := os.MkdirTemp("", "sfdeck-retrieve-*")
	if err != nil {
		return nil, fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := writeMinimalSFDXProject(dir, alias); err != nil {
		return nil, err
	}

	args := []string{"project", "retrieve", "start", "--target-org", alias, "--json"}
	if len(members) == 0 {
		args = append(args, "--metadata", metadataType)
	} else {
		for _, mem := range members {
			args = append(args, "--metadata", metadataType+":"+mem)
		}
	}
	if _, err := runSFInDir(dir, alias, args...); err != nil {
		return nil, fmt.Errorf("retrieve %s: %w", metadataType, err)
	}

	root := filepath.Join(dir, "force-app")
	out := map[string]string{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil || info.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		key := metadataComponentKey(info.Name())
		if key != "" {
			out[key] = string(data)
		}
		return nil
	})
	return out, nil
}

func metadataComponentKey(filename string) string {
	switch {
	case strings.HasSuffix(filename, ".cls-meta.xml"),
		strings.HasSuffix(filename, ".trigger-meta.xml"),
		strings.HasSuffix(filename, ".page-meta.xml"),
		strings.HasSuffix(filename, ".component-meta.xml"):
		return "" // sidecar of a code file — primary captured separately
	case strings.HasSuffix(filename, ".cls"):
		return strings.TrimSuffix(filename, ".cls")
	case strings.HasSuffix(filename, ".trigger"):
		return strings.TrimSuffix(filename, ".trigger")
	case strings.HasSuffix(filename, ".page"):
		return strings.TrimSuffix(filename, ".page")
	case strings.HasSuffix(filename, ".component"):
		return strings.TrimSuffix(filename, ".component")
	}
	base := filename
	if i := strings.Index(base, "-meta.xml"); i >= 0 {
		base = base[:i]
	}
	if dot := strings.Index(base, "."); dot >= 0 {
		base = base[:dot]
	}
	if base == "" {
		return ""
	}
	return base
}

func writeMinimalSFDXProject(dir, alias string) error {
	// Same priority as the REST client: a user-forced version wins;
	// otherwise resolve the org-reported version via APIVersionForAlias,
	// which also falls back to defaultAPIVersion.
	apiVer := cfgAPIVersion()
	if apiVer == "" {
		apiVer = APIVersionForAlias(alias)
	}
	proj := `{"packageDirectories":[{"path":"force-app","default":true}],"sourceApiVersion":"` +
		apiVer + `"}`
	if err := os.WriteFile(filepath.Join(dir, "sfdx-project.json"), []byte(proj), 0o644); err != nil {
		return fmt.Errorf("write sfdx-project.json: %w", err)
	}
	return os.MkdirAll(filepath.Join(dir, "force-app"), 0o755)
}
