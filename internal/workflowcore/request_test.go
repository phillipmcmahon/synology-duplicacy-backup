package workflowcore

import "testing"

func TestRequestErrors(t *testing.T) {
	err := NewRequestError("bad %s", "input")
	if err.Error() != "bad input" {
		t.Fatalf("RequestError = %q", err.Error())
	}
	if err.ShowUsage {
		t.Fatalf("NewRequestError ShowUsage = true")
	}

	usageErr := NewUsageRequestError("unknown %s", "flag")
	if usageErr.Error() != "unknown flag" {
		t.Fatalf("UsageRequestError = %q", usageErr.Error())
	}
	if !usageErr.ShowUsage {
		t.Fatalf("NewUsageRequestError ShowUsage = false")
	}
}

func TestRequestAndConfigPlanProjection(t *testing.T) {
	req := &Request{
		Source:               "homes",
		RequestedStorageName: "onsite-usb",
		ConfigDir:            "/config",
		SecretsDir:           "/secrets",
	}

	if got := req.Target(); got != "onsite-usb" {
		t.Fatalf("Target = %q", got)
	}
	if got := (*Request)(nil).Target(); got != "" {
		t.Fatalf("nil Target = %q", got)
	}

	projected := NewConfigPlanRequest(req)
	if projected.Label != "homes" || projected.Target() != "onsite-usb" || projected.ConfigDir != "/config" || projected.SecretsDir != "/secrets" {
		t.Fatalf("NewConfigPlanRequest = %+v", projected)
	}
	if got := NewConfigPlanRequest(nil); got != (ConfigPlanRequest{}) {
		t.Fatalf("NewConfigPlanRequest(nil) = %+v", got)
	}
}
