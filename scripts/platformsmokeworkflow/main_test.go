package main

import "testing"

func TestPlatformSmokeWorkflowPolicy(t *testing.T) {
	if err := Validate("../../.github/workflows/platform-smoke.yml"); err != nil {
		t.Fatal(err)
	}
}

func TestQualityWorkflowPolicy(t *testing.T) {
	if err := ValidateQuality("../../.github/workflows/ci.yml"); err != nil {
		t.Fatal(err)
	}
}
