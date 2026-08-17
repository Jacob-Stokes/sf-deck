package sf

// Metadata API (REST) — the deploy surface for entities Tooling can't
// PATCH. See README_API_ROUTING.md for the routing cheatsheet.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"mime/multipart"
	"net/textproto"
	"os"
	"strings"
	"time"
)

func dlogf(format string, args ...any) {
	if os.Getenv("SFDECK_DEBUG_DEPLOY") != "1" {
		return
	}
	home, _ := os.UserHomeDir()
	f, err := os.OpenFile(home+"/.sf-deck/deploy.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	log.New(f, "", log.LstdFlags|log.Lmicroseconds).Printf(format, args...)
}

// MetadataFile is one file to ship inside the deploy ZIP. Path is
// relative to the ZIP root and must match the conventions Salesforce
// expects (e.g. "objects/MyObject__c.object" or
// "objects/MyObject__c/fields/MyField__c.field-meta.xml"). Body is
// the raw XML for that file.
type MetadataFile struct {
	Path string
	Body []byte
}

// PackageMember is one <types><members>…</members><name>…</name></types>
// entry in package.xml — tells the deploy which metadata items are
// being shipped so Salesforce scopes the deploy correctly.
type PackageMember struct {
	Type    string   // e.g. "CustomObject"
	Members []string // e.g. ["MyObject__c"]
}

// DeployOptions mirrors the subset of DeployOptions we actually use.
// Defaults are conservative: no test run, no check-only, no purge on
// delete. Callers can override per-deploy if needed.
type DeployOptions struct {
	CheckOnly       bool `json:"checkOnly"`
	RollbackOnError bool `json:"rollbackOnError"`
	SinglePackage   bool `json:"singlePackage"`
}

func defaultDeployOptions() DeployOptions {
	return DeployOptions{
		CheckOnly:       false,
		RollbackOnError: true,
		SinglePackage:   true,
	}
}

// DeployResult is the summary returned to callers after a deploy has
// reached a terminal state. Success flag + any first-error message.
type DeployResult struct {
	ID         string
	Success    bool
	Status     string   // "Succeeded", "Failed", "Canceled", …
	Messages   []string // human-readable component-level messages
	FirstError string   // the most useful single-line summary
}

// DeployMetadata is the one-call primitive: build the ZIP from the
// given files + manifest, submit, poll to completion, return result.
//
//	target   — sf org alias
//	version  — API version for package.xml ("65.0")
//	members  — one PackageMember per metadata type included
//	files    — the files to ship; paths relative to package root
//
// Typical usage for an object-label edit:
//
//	xml := buildCustomObjectXML(...)
//	res, err := DeployMetadata(alias, "65.0",
//	    []PackageMember{{Type: "CustomObject", Members: []string{"MyObject__c"}}},
//	    []MetadataFile{{Path: "objects/MyObject__c.object", Body: xml}},
//	)
func DeployMetadata(target, version string, members []PackageMember, files []MetadataFile) (*DeployResult, error) {
	dlogf("DeployMetadata enter target=%s v=%s members=%v", target, version, members)
	c, err := RESTClient(target)
	if err != nil {
		dlogf("DeployMetadata RESTClient err=%v", err)
		return nil, err
	}
	if version == "" {
		version = c.apiVersion
	}
	dlogf("DeployMetadata using version=%s instanceURL=%s", version, c.instanceURL)

	zipBytes, err := buildDeployZip(version, members, files)
	if err != nil {
		dlogf("DeployMetadata buildDeployZip err=%v", err)
		return nil, fmt.Errorf("build zip: %w", err)
	}
	dlogf("DeployMetadata zip built: %d bytes", len(zipBytes))

	id, err := submitDeploy(c, version, zipBytes)
	if err != nil {
		dlogf("DeployMetadata submitDeploy err=%v", err)
		return nil, err
	}
	dlogf("DeployMetadata submitted id=%s; starting poll", id)

	res, err := pollDeploy(c, version, id)
	dlogf("DeployMetadata done id=%s res=%+v err=%v", id, res, err)
	return res, err
}

func buildDeployZip(version string, members []PackageMember, files []MetadataFile) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	manifest := buildPackageXML(version, members)
	f, err := w.Create("package.xml")
	if err != nil {
		return nil, err
	}
	if _, err := f.Write([]byte(manifest)); err != nil {
		return nil, err
	}

	for _, mf := range files {
		f, err := w.Create(mf.Path)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(mf.Body); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildPackageXML(version string, members []PackageMember) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString("\n")
	b.WriteString(`<Package xmlns="http://soap.sforce.com/2006/04/metadata">`)
	b.WriteString("\n")
	for _, m := range members {
		b.WriteString("  <types>\n")
		for _, mem := range m.Members {
			b.WriteString("    <members>")
			b.WriteString(xmlEscape(mem))
			b.WriteString("</members>\n")
		}
		b.WriteString("    <name>")
		b.WriteString(xmlEscape(m.Type))
		b.WriteString("</name>\n")
		b.WriteString("  </types>\n")
	}
	b.WriteString("  <version>")
	b.WriteString(xmlEscape(version))
	b.WriteString("</version>\n")
	b.WriteString("</Package>\n")
	return b.String()
}

func submitDeploy(c *Client, version string, zipBytes []byte) (string, error) {
	opts := struct {
		DeployOptions DeployOptions `json:"deployOptions"`
	}{DeployOptions: defaultDeployOptions()}
	optsJSON, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	optsHeader := textproto.MIMEHeader{}
	optsHeader.Set("Content-Disposition", `form-data; name="entity_content"`)
	optsHeader.Set("Content-Type", "application/json")
	optsPart, err := mw.CreatePart(optsHeader)
	if err != nil {
		return "", err
	}
	if _, err := optsPart.Write(optsJSON); err != nil {
		return "", err
	}

	fileHeader := textproto.MIMEHeader{}
	fileHeader.Set("Content-Disposition", `form-data; name="file"; filename="deploy.zip"`)
	fileHeader.Set("Content-Type", "application/zip")
	filePart, err := mw.CreatePart(fileHeader)
	if err != nil {
		return "", err
	}
	if _, err := filePart.Write(zipBytes); err != nil {
		return "", err
	}

	if err := mw.Close(); err != nil {
		return "", err
	}

	path := "/services/data/v" + version + "/metadata/deployRequest"
	contentType := mw.FormDataContentType()
	dlogf("submit: POST %s (zip=%d bytes, content-type=%s)",
		path, len(zipBytes), contentType)
	raw, err := c.postMultipart(path, contentType, body.Bytes())
	if err != nil {
		dlogf("submit: error: %v", err)
		return "", upgradeToSFError(err)
	}
	dlogf("submit: response body: %s", string(raw))

	var resp struct {
		ID           string         `json:"id"`
		DeployResult map[string]any `json:"deployResult"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("decode deployRequest response: %w; body=%s",
			err, string(raw))
	}
	if resp.ID == "" {
		if nested, ok := resp.DeployResult["id"].(string); ok && nested != "" {
			dlogf("submit: id from nested deployResult.id: %s", nested)
			return nested, nil
		}
		return "", fmt.Errorf("deploy submit returned no id; body=%s", string(raw))
	}
	dlogf("submit: id=%s", resp.ID)
	return resp.ID, nil
}

func pollDeploy(c *Client, version, id string) (*DeployResult, error) {
	path := "/services/data/v" + version + "/metadata/deployRequest/" + id + "?includeDetails=true"
	maxWait := cfgDeployDeadline()
	deadline := time.Now().Add(maxWait)
	interval := 500 * time.Millisecond
	steady := cfgDeployPoll()
	for attempts := 0; ; attempts++ {
		raw, err := c.get(path, nil)
		if err != nil {
			dlogf("poll[%d]: error: %v", attempts, err)
			return nil, upgradeToSFError(err)
		}
		dlogf("poll[%d]: body: %s", attempts, string(raw))
		var body struct {
			DeployResult struct {
				ID           string `json:"id"`
				Done         bool   `json:"done"`
				Status       string `json:"status"`
				Success      bool   `json:"success"`
				ErrorMessage string `json:"errorMessage"`
				Details      struct {
					ComponentFailures []struct {
						ProblemType string `json:"problemType"`
						Problem     string `json:"problem"`
						FileName    string `json:"fileName"`
						FullName    string `json:"fullName"`
					} `json:"componentFailures"`
				} `json:"details"`
			} `json:"deployResult"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, fmt.Errorf("decode deploy status: %w", err)
		}
		dr := body.DeployResult
		if dr.Done {
			out := &DeployResult{
				ID:      dr.ID,
				Success: dr.Success,
				Status:  dr.Status,
			}
			if dr.ErrorMessage != "" {
				out.FirstError = dr.ErrorMessage
			}
			for _, f := range dr.Details.ComponentFailures {
				msg := f.Problem
				if f.FullName != "" {
					msg = f.FullName + ": " + msg
				}
				out.Messages = append(out.Messages, msg)
				if out.FirstError == "" {
					out.FirstError = msg
				}
			}
			return out, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("deploy %s timed out after %s (last status: %s)",
				id, maxWait, dr.Status)
		}
		time.Sleep(interval)
		if attempts >= 2 {
			interval = steady
		}
	}
}

// xmlEscape minimally escapes the five XML special characters. We
// don't need a full encoder here — the package.xml content is
// developer-authored ascii identifiers plus the version string.
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}
