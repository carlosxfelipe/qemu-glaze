package main

import (
	"strings"
	"testing"

	"qemu-glaze/store"
	"qemu-glaze/vm"

	"github.com/crgimenes/glaze"
)

func TestAssetName(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"app://home/", "index.html"},
		{"app://home/index.html", "index.html"},
		{"app://home/theme.css", "theme.css"},
		{"app://home/app.js", "app.js"},
		{"app://home/icon.png", "icon.png"},
		{"app://home/../secret", "secret"},
	}
	for _, tc := range cases {
		if got := assetName(tc.url); got != tc.want {
			t.Errorf("assetName(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestServeAsset(t *testing.T) {
	resp := serveAsset(&glaze.SchemeRequest{URL: "app://home/index.html"})
	if resp == nil || len(resp.Body) == 0 {
		t.Fatal("expected index.html response body")
	}
	if !strings.Contains(resp.MIMEType, "text/html") {
		t.Errorf("unexpected MIME type for index.html: %q", resp.MIMEType)
	}

	cssResp := serveAsset(&glaze.SchemeRequest{URL: "app://home/theme.css"})
	if cssResp == nil || !strings.Contains(cssResp.MIMEType, "text/css") {
		t.Errorf("expected text/css MIME type, got %+v", cssResp)
	}

	iconResp := serveAsset(&glaze.SchemeRequest{URL: "app://home/icon.png"})
	if iconResp == nil || iconResp.MIMEType != "image/png" {
		t.Errorf("expected image/png MIME type, got %+v", iconResp)
	}

	missing := serveAsset(&glaze.SchemeRequest{URL: "app://home/nonexistent.js"})
	if missing != nil {
		t.Errorf("expected nil response for missing file with extension, got %+v", missing)
	}

	spaFallback := serveAsset(&glaze.SchemeRequest{URL: "app://home/vms"})
	if spaFallback == nil || !strings.Contains(spaFallback.MIMEType, "text/html") {
		t.Errorf("expected SPA fallback to index.html, got %+v", spaFallback)
	}
}

func TestAppAPISaveListDelete(t *testing.T) {
	st, err := store.New("qemu-glaze-api-test", store.Options{MemoryOnly: true})
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	api := &AppAPI{
		mgr:   vm.NewManager(nil),
		store: st,
	}

	// Validation should reject short name
	_, err = api.SaveMachine(vm.SaveVMRequest{
		VM: vm.VMConfig{Name: "X", Arch: "aarch64", Cores: 2, MemoryMB: 2048},
	})
	if err == nil {
		t.Fatal("expected validation error for short VM name")
	}

	saved, err := api.SaveMachine(vm.SaveVMRequest{
		VM: vm.VMConfig{Name: "Alpine Test", Arch: "aarch64", Cores: 2, MemoryMB: 2048},
	})
	if err != nil {
		t.Fatalf("SaveMachine failed: %v", err)
	}
	if saved.ID == "" {
		t.Fatal("expected generated ID on saved VM")
	}

	listed, err := api.ListMachines()
	if err != nil {
		t.Fatalf("ListMachines failed: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].Name != "Alpine Test" {
		t.Fatalf("unexpected list result: %+v", listed.Items)
	}

	if _, err := api.DeleteMachine(vm.DeleteVMRequest{ID: saved.ID}); err != nil {
		t.Fatalf("DeleteMachine failed: %v", err)
	}

	listedAfter, err := api.ListMachines()
	if err != nil {
		t.Fatalf("ListMachines after delete failed: %v", err)
	}
	if len(listedAfter.Items) != 0 {
		t.Fatalf("expected 0 VMs after delete, got %d", len(listedAfter.Items))
	}
}
