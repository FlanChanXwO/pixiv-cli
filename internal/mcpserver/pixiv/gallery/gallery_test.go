package gallery_test

import (
	"os/exec"
	"testing"
)

// The browser-independent fixture uses only Node built-ins, with no npm install.
func TestGalleryBridgeMock(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable; run node gallery_test.mjs to verify the app bridge")
	}
	output, err := exec.CommandContext(t.Context(), node, "gallery_test.mjs").CombinedOutput()
	if err != nil {
		t.Fatalf("bridge mock failed: %v\n%s", err, output)
	}
}
