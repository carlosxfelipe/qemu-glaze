package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreEncryptedRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vms.json")

	s, err := New("qemu-glaze-test", Options{
		Path:          path,
		EncryptionKey: "test-secret-key",
	})
	if err != nil {
		t.Fatalf("New store failed: %v", err)
	}

	type sample struct {
		Name  string `json:"name"`
		Cores int    `json:"cores"`
	}

	want := []sample{{Name: "Ubuntu", Cores: 4}}
	if err := s.Set("vms", want); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.HasPrefix(string(raw), encryptedFilePrefix) {
		t.Fatalf("expected encrypted prefix, got %q", string(raw))
	}

	s2, err := New("qemu-glaze-test", Options{
		Path:          path,
		EncryptionKey: "test-secret-key",
	})
	if err != nil {
		t.Fatalf("Reopen store failed: %v", err)
	}

	var got []sample
	ok, err := s2.Get("vms", &got)
	if err != nil || !ok {
		t.Fatalf("Get failed: ok=%v err=%v", ok, err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
