package sf

// Deep-detail fetch for one DeployRequest. The tooling SOQL behind
// the /deploys list only carries counters; the REST deployRequest
// endpoint (same one `sf project deploy report` uses) returns the
// per-component and per-test breakdown — file, line, problem text,
// stack traces. That's what the /deploy drill renders.

import (
	"encoding/json"
	"net/url"
)

// DeployComponentMessage is one row of componentFailures /
// componentSuccesses from the deployRequest details payload.
type DeployComponentMessage struct {
	ComponentType string `json:"componentType"`
	FileName      string `json:"fileName"`
	FullName      string `json:"fullName"`
	Problem       string `json:"problem"`
	ProblemType   string `json:"problemType"` // "Error" / "Warning"
	LineNumber    int    `json:"lineNumber"`
	ColumnNumber  int    `json:"columnNumber"`
	Created       bool   `json:"created"`
	Changed       bool   `json:"changed"`
	Deleted       bool   `json:"deleted"`
	Success       bool   `json:"success"`
	Warning       bool   `json:"warning"`
}

// DeployTestFailure is one failing test method from runTestResult.
//
// Salesforce emits Time as a number of milliseconds (sometimes
// fractional). json.Number captures it without forcing a numeric
// decode that would lose precision — callers stringify when
// rendering. Was string-typed; broke on any deploy with test
// failures because the platform JSON had `"time": 123`, not
// `"time": "123"`.
type DeployTestFailure struct {
	Name       string      `json:"name"`
	MethodName string      `json:"methodName"`
	Message    string      `json:"message"`
	StackTrace string      `json:"stackTrace"`
	Time       json.Number `json:"time"`
}

// DeployCodeCoverageWarning surfaces SF's coverage complaints (the
// "Average test coverage across all Apex Classes and Triggers is
// 72%" style messages that block prod deploys).
type DeployCodeCoverageWarning struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// DeployDetail is the drill payload for one deploy.
type DeployDetail struct {
	ID        string
	Status    string
	Failures  []DeployComponentMessage
	Successes []DeployComponentMessage
	TestFails []DeployTestFailure
	Coverage  []DeployCodeCoverageWarning
	TestsRun  int
	TestTime  string
}

// deployRequestEnvelope mirrors the slice of the REST response we
// care about.
type deployRequestEnvelope struct {
	DeployResult struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Details struct {
			ComponentFailures  []DeployComponentMessage `json:"componentFailures"`
			ComponentSuccesses []DeployComponentMessage `json:"componentSuccesses"`
			RunTestResult      struct {
				NumTestsRun int                         `json:"numTestsRun"`
				NumFailures int                         `json:"numFailures"`
				TotalTime   json.Number                 `json:"totalTime"`
				Failures    []DeployTestFailure         `json:"failures"`
				Warnings    []DeployCodeCoverageWarning `json:"codeCoverageWarnings"`
			} `json:"runTestResult"`
		} `json:"details"`
	} `json:"deployResult"`
}

// FetchDeployDetail pulls the full component/test breakdown for one
// deploy via GET metadata/deployRequest/<id>?includeDetails=true.
func FetchDeployDetail(target, deployID string) (DeployDetail, error) {
	c, err := RESTClient(target)
	if err != nil {
		return DeployDetail{}, err
	}
	q := url.Values{}
	q.Set("includeDetails", "true")
	body, err := c.get(c.APIPath("metadata/deployRequest/"+deployID), q)
	if err != nil {
		return DeployDetail{}, err
	}
	var env deployRequestEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return DeployDetail{}, err
	}
	res := env.DeployResult
	out := DeployDetail{
		ID:        res.ID,
		Status:    res.Status,
		Failures:  res.Details.ComponentFailures,
		Successes: res.Details.ComponentSuccesses,
		TestFails: res.Details.RunTestResult.Failures,
		Coverage:  res.Details.RunTestResult.Warnings,
		TestsRun:  res.Details.RunTestResult.NumTestsRun,
		TestTime:  res.Details.RunTestResult.TotalTime.String(),
	}
	return out, nil
}
