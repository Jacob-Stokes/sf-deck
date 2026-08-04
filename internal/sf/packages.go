package sf

import "encoding/json"

// InstalledPackage is one row from `sf package installed list`.
type InstalledPackage struct {
	ID                             string `json:"Id"`
	SubscriberPackageID            string `json:"SubscriberPackageId"`
	SubscriberPackageName          string `json:"SubscriberPackageName"`
	SubscriberPackageNamespace     string `json:"SubscriberPackageNamespace"`
	SubscriberPackageVersionID     string `json:"SubscriberPackageVersionId"`
	SubscriberPackageVersionName   string `json:"SubscriberPackageVersionName"`
	SubscriberPackageVersionNumber string `json:"SubscriberPackageVersionNumber"`
}

type installedPackagesResult struct {
	Result []InstalledPackage `json:"result"`
}

// InstalledPackages shells out to `sf package installed list -o <target>
// --json`. Read-only.
func InstalledPackages(target string) ([]InstalledPackage, error) {
	out, err := runSF("package", "installed", "list", "-o", target, "--json")
	if err != nil {
		// Some orgs (free dev/scratch) error — return empty rather than fail.
		return nil, nil
	}
	var parsed installedPackagesResult
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}
	return parsed.Result, nil
}
