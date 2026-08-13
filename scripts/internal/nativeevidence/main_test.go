package nativeevidence

import (
	"os"
	"testing"
)

const testEvidenceCommit = "0123456789abcdef0123456789abcdef01234567"

func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "version" && os.Args[2] == "--json" {
		_, _ = os.Stdout.WriteString(`{"version":"v0.1.0-native-evidence.test","commit":"fixture-commit","build_date":"2026-07-12T00:00:00Z"}` + "\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
