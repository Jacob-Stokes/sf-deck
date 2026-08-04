package sf

import "testing"

func TestDemoTargetRegistry(t *testing.T) {
	// Clean slate for isolation.
	UnregisterDemoTargets("demo-alias", "demo@x.example")

	if isDemoTarget("demo-alias") {
		t.Fatal("unregistered target should not be demo")
	}
	RegisterDemoTargets("demo-alias", "demo@x.example")
	if !isDemoTarget("demo-alias") || !IsDemoOrgTarget("demo@x.example") {
		t.Fatal("registered targets should be demo")
	}
	if isDemoTarget("real@org.com") {
		t.Fatal("a real org must never be a demo target")
	}
	UnregisterDemoTargets("demo-alias")
	if isDemoTarget("demo-alias") {
		t.Fatal("unregister should remove the target")
	}
	UnregisterDemoTargets("demo@x.example")
}

func TestRESTClientRefusesDemoTarget(t *testing.T) {
	RegisterDemoTargets("demo-refuse@x.example")
	defer UnregisterDemoTargets("demo-refuse@x.example")
	_, err := RESTClient("demo-refuse@x.example")
	if !IsDemoTargetErr(err) {
		t.Fatalf("RESTClient for a demo target should return ErrDemoTarget, got %v", err)
	}
}
