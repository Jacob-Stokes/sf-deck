package sf

import "encoding/json"

// ApexLogRow is one row from `sf apex log list`.
type ApexLogRow struct {
	ID          string `json:"Id"`
	Application string `json:"Application"`
	DurationMs  int    `json:"DurationMilliseconds"`
	LogLength   int    `json:"LogLength"`
	Operation   string `json:"Operation"`
	Status      string `json:"Status"`
	StartTime   string `json:"StartTime"`
	LogUser     struct {
		Name string `json:"Name"`
	} `json:"LogUser"`
}

type apexLogsResult struct {
	Result []ApexLogRow `json:"result"`
}

// ApexLogs shells out to `sf apex log list -o <target> --json`. Read-only.
func ApexLogs(target string) ([]ApexLogRow, error) {
	out, err := runSF("apex", "log", "list", "-o", target, "--json")
	if err != nil {
		return nil, err
	}
	var parsed apexLogsResult
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}
	return parsed.Result, nil
}
