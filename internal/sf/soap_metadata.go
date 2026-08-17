package sf

// SOAP Metadata API — readMetadata.

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const soapReadBatchMax = 10

// soapReadTimeout bounds a single readMetadata POST. Generous because a
// batch of large components (Profiles) can be tens of MB.
const soapReadTimeout = 4 * time.Minute

func soapBatchSizeFor(metadataType string) int {
	switch metadataType {
	case "Profile":
		return 1
	case "PermissionSet":
		return 3
	default:
		return soapReadBatchMax
	}
}

// soapEnvelope wraps a readMetadata call. sessionId is the access token.
func soapReadEnvelope(token, metadataType string, names []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:met="http://soap.sforce.com/2006/04/metadata">`)
	b.WriteString(`<soapenv:Header><met:SessionHeader><met:sessionId>`)
	b.WriteString(xmlEscape(token))
	b.WriteString(`</met:sessionId></met:SessionHeader></soapenv:Header>`)
	b.WriteString(`<soapenv:Body><met:readMetadata><met:type>`)
	b.WriteString(xmlEscape(metadataType))
	b.WriteString(`</met:type>`)
	for _, n := range names {
		b.WriteString(`<met:fullNames>`)
		b.WriteString(xmlEscape(n))
		b.WriteString(`</met:fullNames>`)
	}
	b.WriteString(`</met:readMetadata></soapenv:Body></soapenv:Envelope>`)
	return b.String()
}

// ReadMetadata calls the SOAP readMetadata operation for up to
// soapReadBatchMax names of one metadata type, returning fullName → the
// component's XML (the inner content of its <records> block). Names that
// don't exist are simply absent from the result (readMetadata nils
// them). Returns an error only on transport/SOAP-fault failure.
func (c *Client) ReadMetadata(metadataType string, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return map[string]string{}, nil
	}
	if len(names) > soapReadBatchMax {
		return nil, fmt.Errorf("readMetadata: %d names exceeds max %d", len(names), soapReadBatchMax)
	}
	raw, err := c.doSOAPWithRetry("readMetadata", "readMetadata "+metadataType, soapReadTimeout,
		func(token string) string { return soapReadEnvelope(token, metadataType, names) })
	if err != nil {
		return nil, fmt.Errorf("readMetadata %s: %w", metadataType, err)
	}
	text := string(raw)
	if fault := soapFault(text); fault != "" {
		return nil, fmt.Errorf("readMetadata %s: %s", metadataType, fault)
	}
	return parseReadMetadataRecords(text), nil
}

const soapListMaxQueries = 3

func soapListEnvelope(token, asOfVersion string, types []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:met="http://soap.sforce.com/2006/04/metadata">`)
	b.WriteString(`<soapenv:Header><met:SessionHeader><met:sessionId>`)
	b.WriteString(xmlEscape(token))
	b.WriteString(`</met:sessionId></met:SessionHeader></soapenv:Header>`)
	b.WriteString(`<soapenv:Body><met:listMetadata>`)
	for _, t := range types {
		b.WriteString(`<met:queries><met:type>`)
		b.WriteString(xmlEscape(t))
		b.WriteString(`</met:type></met:queries>`)
	}
	b.WriteString(`<met:asOfVersion>`)
	b.WriteString(xmlEscape(asOfVersion))
	b.WriteString(`</met:asOfVersion>`)
	b.WriteString(`</met:listMetadata></soapenv:Body></soapenv:Envelope>`)
	return b.String()
}

// ListMetadata calls the SOAP listMetadata operation for up to
// soapListMaxQueries types in one call and returns type → its components
// (as MetadataItem, mirroring MetadataListByType's CLI output). This is
// the HTTP twin of `sf org list metadata`: same fullNames, no subprocess.
func (c *Client) ListMetadata(types []string) (map[string][]MetadataItem, error) {
	if len(types) == 0 {
		return map[string][]MetadataItem{}, nil
	}
	if len(types) > soapListMaxQueries {
		return nil, fmt.Errorf("listMetadata: %d types exceeds max %d", len(types), soapListMaxQueries)
	}
	raw, err := c.doSOAPWithRetry("listMetadata", "listMetadata", 0,
		func(token string) string {
			ver := c.soapAPIVersion()
			return soapListEnvelope(token, ver, types)
		})
	if err != nil {
		return nil, fmt.Errorf("listMetadata: %w", err)
	}
	text := string(raw)
	if fault := soapFault(text); fault != "" {
		return nil, fmt.Errorf("listMetadata: %s", fault)
	}
	return parseListMetadataResult(text), nil
}

func (c *Client) doSOAPWithRetry(action, logLabel string, timeout time.Duration, envelope func(token string) string) ([]byte, error) {
	raw, err := c.doSOAPOnce(action, logLabel, timeout, envelope)
	if err == nil && !soapSessionExpired(raw) {
		return raw, nil
	}
	if err == nil {
		err = &sfHTTPError{Status: http.StatusUnauthorized, Body: raw}
	}
	if !isSessionExpired(err) {
		return nil, err
	}
	if berr := c.bootstrap(); berr != nil {
		return nil, fmt.Errorf("re-auth failed: %w (original: %v)", berr, err)
	}
	raw, err = c.doSOAPOnce(action, logLabel, timeout, envelope)
	if err != nil {
		return nil, err
	}
	if soapSessionExpired(raw) {
		return nil, &sfHTTPError{Status: http.StatusUnauthorized, Body: raw}
	}
	return raw, nil
}

func (c *Client) doSOAPOnce(action, logLabel string, timeout time.Duration, envelope func(token string) string) (out []byte, err error) {
	c.mu.Lock()
	token := c.accessToken
	base := c.instanceURL
	ver := c.apiVersion
	c.mu.Unlock()
	if ver == "" {
		ver = defaultAPIVersion
	}
	endpoint := strings.TrimRight(base, "/") + "/services/Soap/m/" + ver
	body := envelope(token)

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=UTF-8")
	req.Header.Set("SOAPAction", action)
	req.Header.Set("User-Agent", "sf-deck/0.1")

	httpc := c.http
	if timeout > 0 && c.http.Timeout < timeout {
		httpc = &http.Client{Timeout: timeout, Transport: c.http.Transport}
	}
	started := time.Now()
	defer func() {
		fireOnCall(c.alias, []string{"POST", "/services/Soap/m " + logLabel}, err, time.Since(started))
	}()

	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := readBodyLimited(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, &sfHTTPError{Status: resp.StatusCode, Body: raw}
	}
	return raw, nil
}

func (c *Client) soapAPIVersion() string {
	c.mu.Lock()
	ver := c.apiVersion
	c.mu.Unlock()
	if ver == "" {
		return defaultAPIVersion
	}
	return ver
}

func soapSessionExpired(raw []byte) bool {
	fault := soapFault(string(raw))
	return strings.Contains(fault, "INVALID_SESSION_ID") ||
		strings.Contains(fault, "Session expired")
}

func parseListMetadataResult(xml string) map[string][]MetadataItem {
	out := map[string][]MetadataItem{}
	for _, rec := range extractBlocks(xml, "result") {
		name := innerText(rec, "fullName")
		if name == "" {
			continue
		}
		typ := innerText(rec, "type")
		out[typ] = append(out[typ], MetadataItem{
			FullName:         name,
			Type:             typ,
			NamespacePrefix:  innerText(rec, "namespacePrefix"),
			LastModifiedDate: innerText(rec, "lastModifiedDate"),
		})
	}
	return out
}

func soapFault(xml string) string {
	if !strings.Contains(xml, "Fault>") {
		return ""
	}
	if s := innerText(xml, "faultstring"); s != "" {
		return s
	}
	return "SOAP fault"
}

func parseReadMetadataRecords(xml string) map[string]string {
	out := map[string]string{}
	for _, rec := range extractBlocks(xml, "records") {
		name := innerText(rec, "fullName")
		if name == "" {
			continue // nil / unnamed record
		}
		out[name] = rec
	}
	return out
}

func extractBlocks(s, tag string) []string {
	var out []string
	openPrefix := "<" + tag
	closeTag := "</" + tag + ">"
	i := 0
	for {
		start := indexOpen(s, openPrefix, i)
		if start < 0 {
			break
		}
		gt := strings.IndexByte(s[start:], '>')
		if gt < 0 {
			break
		}
		openEnd := start + gt + 1
		if gt > 0 && s[start+gt-1] == '/' {
			i = openEnd
			continue
		}
		depth := 1
		j := openEnd
		for depth > 0 {
			nextOpen := indexOpen(s, openPrefix, j)
			nextClose := strings.Index(s[j:], closeTag)
			if nextClose < 0 {
				return out // malformed; bail with what we have
			}
			nextClose += j
			if nextOpen >= 0 && nextOpen < nextClose {
				ngt := strings.IndexByte(s[nextOpen:], '>')
				if ngt > 0 && s[nextOpen+ngt-1] != '/' {
					depth++
				}
				j = nextOpen + 1
			} else {
				depth--
				if depth == 0 {
					out = append(out, s[openEnd:nextClose])
					i = nextClose + len(closeTag)
				}
				j = nextClose + len(closeTag)
			}
		}
	}
	return out
}

func indexOpen(s, openPrefix string, from int) int {
	for from <= len(s)-len(openPrefix) {
		idx := strings.Index(s[from:], openPrefix)
		if idx < 0 {
			return -1
		}
		idx += from
		after := idx + len(openPrefix)
		if after < len(s) {
			c := s[after]
			if c == '>' || c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '/' {
				return idx
			}
		}
		from = idx + len(openPrefix)
	}
	return -1
}

func innerText(s, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	a := strings.Index(s, open)
	if a < 0 {
		return ""
	}
	a += len(open)
	b := strings.Index(s[a:], close)
	if b < 0 {
		return ""
	}
	return s[a : a+b]
}
