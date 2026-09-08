package sf

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestReconcilePendingBootstrap(t *testing.T) {
	for _, repointed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "repointed"}[repointed], func(t *testing.T) {
			InvalidateRESTClients()
			defer InvalidateRESTClients()
			logPath := installRESTBootstrapFakeSF(t, `#!/bin/sh
echo called >> "$SF_FAKE_LOG"
while [ ! -f "$SF_FAKE_LOG.ready" ]; do sleep 0.01; done
printf '{"result":{"accessToken":"TEST_TOKEN","instanceUrl":"https://example.test","apiVersion":"65.0"}}'
`)
			type result struct {
				client *Client
				err    error
			}
			done := make(chan result, 1)
			go func() { c, err := RESTClient("test-org"); done <- result{c, err} }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(logPath); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("fake CLI did not start")
				}
				time.Sleep(time.Millisecond)
			}
			want := "https://example.test"
			if repointed {
				want = "https://other.test"
			}
			ReconcileRESTClients(map[string]string{"test-org": want})
			if err := os.WriteFile(logPath+".ready", nil, 0o600); err != nil {
				t.Fatal(err)
			}
			first := <-done
			if repointed {
				if first.err == nil || clientCached("test-org") {
					t.Fatal("repointed org retained stale authentication")
				}
				return
			}
			if first.err != nil {
				t.Fatal(first.err)
			}
			second, err := RESTClient("test-org")
			if err != nil {
				t.Fatal(err)
			}
			if second != first.client || strings.Count(readFileString(t, logPath), "called") != 1 {
				t.Fatal("unchanged org refresh duplicated authentication")
			}
		})
	}
}
