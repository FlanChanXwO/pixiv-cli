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
	for _, mode := range []string{"supported", "without-download"} {
		t.Run(mode, func(t *testing.T) {
			args := []string{"gallery_test.mjs"}
			if mode == "without-download" {
				args = append(args, "--without-download")
			}
			output, err := exec.CommandContext(t.Context(), node, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("bridge mock failed: %v\n%s", err, output)
			}
		})
	}
}
