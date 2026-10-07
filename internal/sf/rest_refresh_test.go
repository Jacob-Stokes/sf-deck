package sf

import (
	"runtime"
	"testing"
)

func TestLookupDuringBootstrap(t *testing.T) {
	InvalidateRESTClients()
	defer InvalidateRESTClients()
	installRESTBootstrapFakeSF(t, `#!/bin/sh
printf '{"result":{"accessToken":"TEST_TOKEN","instanceUrl":"https://example.test","apiVersion":"65.0"}}'
`)
	done := make(chan error, 1)
	go func() { _, err := RESTClient("test-org"); done <- err }()
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			if c, ok := lookupClient("test-org"); !ok || c == nil {
				t.Fatal("completed client missing")
			}
			return
		default:
			lookupClient("test-org")
			runtime.Gosched()
		}
	}
}

func TestRefreshConcurrentVersionReaders(t *testing.T) {
	InvalidateRESTClients()
	defer InvalidateRESTClients()
	installRESTBootstrapFakeSF(t, `#!/bin/sh
printf '{"result":{"accessToken":"TEST_TOKEN","instanceUrl":"https://example.test","apiVersion":"65.0"}}'
`)
	c, err := RESTClient("test-org")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	for range 2 {
		go func() { done <- c.bootstrap() }()
	}
	for completed := 0; completed < 2; {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			completed++
		default:
			if got := c.APIPath("query"); got != "/services/data/v65.0/query" {
				t.Errorf("APIPath = %q", got)
			}
			if got := c.ToolingPath("query"); got != "/services/data/v65.0/tooling/query" {
				t.Errorf("ToolingPath = %q", got)
			}
			if got := APIVersionForAlias("test-org"); got != "65.0" {
				t.Errorf("APIVersionForAlias = %q", got)
			}
			_ = c.soapAPIVersion()
			runtime.Gosched()
		}
	}
}
