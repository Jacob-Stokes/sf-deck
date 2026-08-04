package sf

// High-level metadata retrieval for the /compare feature, built on the
// fast paths (NOT the slow `sf project retrieve` fan-out):
//
//   - RetrieveViaSOAP: list names + parallel batched readMetadata. For
//     object-rooted types it also extracts the object's nested children
//     (fields, validation rules, record types, list views, field sets,
//     and the other CustomObject childXmlNames) so one object read yields
//     the full object-rooted surface.
//   - BulkApexBodies: ONE Tooling query for every Apex body (readMetadata
//     rejects Apex; a bulk query is faster than anything anyway).

import (
	"fmt"
	"sync"
)

// soapRetrieveWorkers bounds concurrent readMetadata calls.
const soapRetrieveWorkers = 12

// SOAPSnapshot is type → (component key → XML), the shape the compare
// snapshot consumes.
type SOAPSnapshot map[string]map[string]string

// RetrieveViaSOAP retrieves a metadata type via parallel batched
// readMetadata and returns its components keyed by fullName. For the
// object-rooted types (CustomObject and its children), pass
// metadataType="CustomObject" and set extractChildren=true: the result
// includes the CustomObject bucket PLUS every child bucket parsed out of
// each object, keyed "<Object>.<Name>".
//
// names are pre-listed (via MetadataListByType) so we only request real
// components — a nonexistent name nils its readMetadata slot.
func RetrieveViaSOAP(alias, metadataType string, names []string, extractChildren bool) (SOAPSnapshot, error) {
	return RetrieveViaSOAPGated(alias, metadataType, names, extractChildren, nil, nil)
}

// RetrieveViaSOAPGated is RetrieveViaSOAP with an optional external gate
// around every readMetadata API call. The /compare runner passes its
// run-level semaphore here so the configured concurrency caps actual
// Salesforce requests rather than whole metadata-type lanes.
func RetrieveViaSOAPGated(alias, metadataType string, names []string, extractChildren bool, acquire, release func()) (SOAPSnapshot, error) {
	c, err := RESTClient(alias)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return SOAPSnapshot{metadataType: {}}, nil
	}

	// Batch by the per-type size (heavy types like Profile go small so a
	// single response isn't tens of MB); run in parallel.
	bsize := soapBatchSizeFor(metadataType)
	var batches [][]string
	for i := 0; i < len(names); i += bsize {
		end := i + bsize
		if end > len(names) {
			end = len(names)
		}
		batches = append(batches, names[i:end])
	}

	type result struct {
		recs map[string]string
		err  error
	}
	results := make([]result, len(batches))
	sem := make(chan struct{}, soapRetrieveWorkers)
	var wg sync.WaitGroup
	for bi, b := range batches {
		bi, b := bi, b
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if acquire != nil {
				acquire()
				if release != nil {
					defer release()
				}
			}
			recs, e := c.ReadMetadata(metadataType, b)
			results[bi] = result{recs: recs, err: e}
		}()
	}
	wg.Wait()

	snap := SOAPSnapshot{metadataType: {}}
	if extractChildren {
		for _, child := range objectChildTagMap {
			snap[child.typeLabel] = map[string]string{}
		}
	}
	var firstErr error
	for _, r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		for name, xml := range r.recs {
			snap[metadataType][name] = xml
			if extractChildren {
				extractObjectChildren(name, xml, snap)
			}
		}
	}
	// Surface ANY batch failure, not just total failure. A partial result
	// (some batches errored, some succeeded) is incomplete and must not be
	// presented as authoritative: in the compare flow, a missing component
	// from a dropped batch would otherwise be misclassified as
	// "only-on-the-other-side" drift rather than "we failed to fetch it".
	// Returning the error lets the caller mark the type retrieve-failed and
	// surface it, instead of silently diffing a truncated set.
	if firstErr != nil {
		return nil, firstErr
	}
	return snap, nil
}

// objectChildTagMap maps each CustomObject child metadata type to the
// XML element it nests under inside the object's readMetadata payload.
// One object read yields ALL of these for free (they're inline), so we
// never list/retrieve them separately. Covers every CustomObject child
// SF reports in describeMetadata.childXmlNames.
var objectChildTagMap = []struct {
	tag, typeLabel string
}{
	{"fields", "CustomField"},
	{"validationRules", "ValidationRule"},
	{"recordTypes", "RecordType"},
	{"compactLayouts", "CompactLayout"},
	{"webLinks", "WebLink"},
	{"listViews", "ListView"},
	{"fieldSets", "FieldSet"},
	{"indexes", "Index"},
	{"businessProcesses", "BusinessProcess"},
	{"sharingReasons", "SharingReason"},
}

// extractObjectChildren pulls every nested child component out of one
// CustomObject's XML into the snapshot, keyed "<Object>.<Name>".
func extractObjectChildren(object, objectXML string, snap SOAPSnapshot) {
	for _, child := range objectChildTagMap {
		for _, block := range extractBlocks(objectXML, child.tag) {
			name := innerText(block, "fullName")
			if name == "" {
				continue
			}
			if snap[child.typeLabel] == nil {
				snap[child.typeLabel] = map[string]string{}
			}
			snap[child.typeLabel][object+"."+name] = block
		}
	}
}

// BulkApexBodies fetches every unmanaged Apex body for the given type in
// ONE Tooling query (ApexClass or ApexTrigger). Returns name → body.
// Far faster than readMetadata (which rejects Apex) or per-class fetch.
func BulkApexBodies(alias, apexType string) (map[string]string, error) {
	// apexType is interpolated into SOQL as an identifier — allowlist
	// it rather than trusting callers. Only these two entities carry
	// a Body column anyway, so anything else is a caller bug.
	if apexType != "ApexClass" && apexType != "ApexTrigger" {
		return nil, fmt.Errorf("bulk apex bodies: unsupported type %q", apexType)
	}
	c, err := RESTClient(alias)
	if err != nil {
		return nil, err
	}
	soql := fmt.Sprintf("SELECT Name, Body FROM %s WHERE NamespacePrefix = null", apexType)
	res, err := c.QueryREST(soql, true) // tooling
	if err != nil {
		return nil, fmt.Errorf("bulk %s bodies: %w", apexType, err)
	}
	out := make(map[string]string, len(res.Records))
	for _, rec := range res.Records {
		name := asString(rec["Name"])
		body := asString(rec["Body"])
		if name != "" {
			out[name] = body
		}
	}
	return out, nil
}
