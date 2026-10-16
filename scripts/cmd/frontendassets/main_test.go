package main

import (
	"flag"
	"os"
	"os/exec"
	"testing"
)

func TestValidationWithoutGeneratedFiles(t *testing.T) {
	for _, mode := range []string{"default", "check"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestValidationProcess$")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "FRONTEND_VALIDATION_TEST="+mode)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("validation: %v: %s", err, output)
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 0 {
				t.Fatalf("validation wrote files: %v, %v", files, err)
			}
		})
	}
}

func TestValidationProcess(t *testing.T) {
	mode := os.Getenv("FRONTEND_VALIDATION_TEST")
	if mode == "" {
		return
	}
	flag.CommandLine = flag.NewFlagSet("frontendassets", flag.ExitOnError)
	os.Args = os.Args[:1]
	if mode == "check" {
		os.Args = append(os.Args, "--check")
	}
	main()
}
