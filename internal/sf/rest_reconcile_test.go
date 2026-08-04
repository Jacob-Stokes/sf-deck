package sf

import "testing"

// seedClient installs a fake bootstrapped client for alias at
// instanceURL and returns a cleanup. Mirrors the describe_test pattern:
// mark once done so the entry is treated as live.
func seedClient(alias, instanceURL string) func() {
	clientsMu.Lock()
	e := &clientEntry{client: &Client{alias: alias, accessToken: "t", instanceURL: instanceURL}}
	e.once.Do(func() {})
	clients[alias] = e
	clientsMu.Unlock()
	return func() {
		clientsMu.Lock()
		delete(clients, alias)
		clientsMu.Unlock()
	}
}

func clientCached(alias string) bool {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	_, ok := clients[alias]
	return ok
}

// The whole point of ReconcileRESTClients: a routine orgs refetch that
// changed nothing must NOT drop a still-valid token (that was the
// slowness — every refresh re-bootstrapped). An unchanged alias survives.
func TestReconcileKeepsUnchangedClient(t *testing.T) {
	defer seedClient("keep-me", "https://acme.my.salesforce.com")()

	ReconcileRESTClients(map[string]string{
		"keep-me": "https://acme.my.salesforce.com",
	})

	if !clientCached("keep-me") {
		t.Fatal("reconcile dropped a client whose alias→instanceURL was unchanged; this reintroduces the slow re-bootstrap on every refresh")
	}
}

// Cosmetic URL variance (trailing slash, scheme) must not count as a
// repoint — otherwise every refetch would drop the client anyway.
func TestReconcileToleratesCosmeticURLVariance(t *testing.T) {
	defer seedClient("cosmetic", "https://acme.my.salesforce.com")()

	ReconcileRESTClients(map[string]string{
		"cosmetic": "acme.my.salesforce.com/", // no scheme, trailing slash
	})

	if !clientCached("cosmetic") {
		t.Fatal("reconcile treated a cosmetically-different but same-host URL as a repoint and dropped the client")
	}
}

// An alias repointed to a genuinely different org (via `sf alias set`
// elsewhere) MUST be dropped so the next call re-bootstraps against the
// new org — the correctness case the blanket invalidate guarded.
func TestReconcileDropsRepointedClient(t *testing.T) {
	defer seedClient("repoint", "https://old.my.salesforce.com")()

	ReconcileRESTClients(map[string]string{
		"repoint": "https://new.my.salesforce.com",
	})

	if clientCached("repoint") {
		t.Fatal("reconcile kept a client whose alias now points at a different org; it would serve stale data under the new label")
	}
}

// An alias that vanished from the org list (logged out) is dropped.
func TestReconcileDropsVanishedClient(t *testing.T) {
	defer seedClient("gone", "https://x.my.salesforce.com")()

	ReconcileRESTClients(map[string]string{
		"other": "https://y.my.salesforce.com",
	})

	if clientCached("gone") {
		t.Fatal("reconcile kept a client whose alias is no longer in the org list")
	}
}

// An empty want-instanceURL means "unknown, don't second-guess" — a
// working client with no URL to compare against survives.
func TestReconcileKeepsWhenWantURLUnknown(t *testing.T) {
	defer seedClient("no-url", "https://z.my.salesforce.com")()

	ReconcileRESTClients(map[string]string{
		"no-url": "", // present in list but URL not carried
	})

	if !clientCached("no-url") {
		t.Fatal("reconcile dropped a client when the org list carried no instanceURL to compare; should keep working clients")
	}
}
