package workflow

import "testing"

func TestUsageRequestErrorAndBroadRequestHelpers(t *testing.T) {
	err := NewUsageRequestError("bad %s", "flag")
	if err.Error() != "bad flag" || !err.ShowUsage {
		t.Fatalf("NewUsageRequestError() = %#v", err)
	}

	if (*Request)(nil).Target() != "" {
		t.Fatal("nil Request target should be empty")
	}
	req := &Request{RequestedTarget: "onsite-usb", FixPerms: true}
	if req.Target() != "onsite-usb" {
		t.Fatalf("Target() = %q", req.Target())
	}
	req.DeriveModes()
	if !req.FixPermsOnly {
		t.Fatal("FixPermsOnly should be derived when fix-perms is the only runtime mode")
	}
	req.DoBackup = true
	req.DeriveModes()
	if req.FixPermsOnly {
		t.Fatal("FixPermsOnly should be false when backup is also selected")
	}
}
