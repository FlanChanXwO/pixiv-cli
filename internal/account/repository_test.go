package account_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/stretchr/testify/require"
)

func TestCredentialRevisionRejectsStaleRotation(t *testing.T) {
	store, err := account.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	require.NoError(t, store.SavePixivCredential(t.Context(), account.Pixiv{UserID: 42, Username: "artist", Credential: credential.New([]byte("old"))}))
	first, err := store.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.NoError(t, store.RotatePixivCredentials(t.Context(), 42, first.CredentialRevision, []byte("rotated")))
	err = store.RotatePixivCredentials(t.Context(), 42, first.CredentialRevision, []byte("stale"))
	require.ErrorIs(t, err, account.ErrCredentialConflict)
	saved, err := store.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.Equal(t, first.CredentialRevision+1, saved.CredentialRevision)
	require.Equal(t, "rotated", saved.Credential.Value())
}

func TestFailedSelectionRollsBackExpiredFreeze(t *testing.T) {
	store, err := account.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	require.NoError(t, store.SavePixivCredential(t.Context(), account.Pixiv{UserID: 42, Username: "artist", Credential: credential.New([]byte("token"))}))
	require.NoError(t, store.Freeze(t.Context(), 42, 100))
	before, err := store.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	abort := errors.New("selection aborted")
	_, err = store.SelectPixiv(t.Context(), 100, nil, func(snapshot account.PoolSnapshot) (int64, error) {
		require.Len(t, snapshot.Candidates, 1)
		require.Nil(t, snapshot.Candidates[0].PoolFrozenUntil)
		return 0, abort
	})
	require.ErrorIs(t, err, abort)
	after, err := store.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestTwoProcessesRotateByRevision(t *testing.T) {
	directory := t.TempDir()
	store, err := account.Open(t.Context(), directory)
	require.NoError(t, err)
	require.NoError(t, store.SavePixivCredential(t.Context(), account.Pixiv{UserID: 42, Credential: credential.New([]byte("initial"))}))
	var commands []*exec.Cmd
	var inputs []io.WriteCloser
	var outputs []*bufio.Reader
	for i := range 2 {
		cmd, input, output := startRotationProcess(t, directory, i)
		commands = append(commands, cmd)
		inputs = append(inputs, input)
		outputs = append(outputs, output)
	}
	for _, input := range inputs {
		_, err := io.WriteString(input, "go\n")
		require.NoError(t, err)
		require.NoError(t, input.Close())
	}
	successes, conflicts := 0, 0
	for i, cmd := range commands {
		output, err := io.ReadAll(outputs[i])
		require.NoError(t, err)
		require.NoError(t, cmd.Wait())
		if strings.Contains(string(output), "saved\n") {
			successes++
		}
		if strings.Contains(string(output), "conflict\n") {
			conflicts++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	saved, err := store.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.EqualValues(t, 2, saved.CredentialRevision)
}

func TestRotationProcess(t *testing.T) {
	directory := os.Getenv("S04_ROTATION_DIRECTORY")
	if directory == "" {
		return
	}
	store, err := account.Open(t.Context(), directory)
	require.NoError(t, err)
	snapshot, err := store.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	fmt.Println("ready")
	_, err = bufio.NewReader(os.Stdin).ReadString('\n')
	require.NoError(t, err)
	err = store.RotatePixivCredentials(t.Context(), 42, snapshot.CredentialRevision, []byte("process-"+os.Getenv("S04_ROTATION_VALUE")))
	if errors.Is(err, account.ErrCredentialConflict) {
		fmt.Println("conflict")
		return
	}
	require.NoError(t, err)
	fmt.Println("saved")
}

func startRotationProcess(t *testing.T, directory string, index int) (*exec.Cmd, io.WriteCloser, *bufio.Reader) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(executable, "-test.run=^TestRotationProcess$")
	cmd.Env = append(os.Environ(), "S04_ROTATION_DIRECTORY="+directory, "S04_ROTATION_VALUE="+strconv.Itoa(index))
	input, err := cmd.StdinPipe()
	require.NoError(t, err)
	output, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	reader := bufio.NewReader(output)
	ready, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready\n", ready)
	return cmd, input, reader
}
