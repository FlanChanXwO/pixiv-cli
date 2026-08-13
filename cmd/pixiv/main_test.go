package main_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBinaryResolvesConfigKeyFromPipeBeforeStartup(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	binaryName := "pixiv-input-pipeline"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/pixiv")
	build.Dir = repositoryRoot
	buildOutput, err := build.CombinedOutput()
	require.NoError(t, err, "go build ./cmd/pixiv: %s", buildOutput)

	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	t.Cleanup(func() { _ = writer.Close() })

	var stdout, stderr bytes.Buffer
	command := exec.Command(binaryPath, "config", "get")
	command.Dir = repositoryRoot
	command.Stdin = reader
	command.Stdout = &stdout
	command.Stderr = &stderr
	require.NoError(t, command.Start())
	_, err = writer.Write([]byte("not-a-real-config-key\r\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	err = command.Wait()
	require.Error(t, err)
	require.Contains(t, stderr.String(), `unknown config key "not-a-real-config-key"`)
	require.Empty(t, stdout.String())
}
