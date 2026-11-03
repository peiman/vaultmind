package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIdentitySignKeeperRegistered(t *testing.T) {
	command, _, err := RootCmd.Find([]string{"identity", "sign-keeper"})
	require.NoError(t, err)
	require.Equal(t, "sign-keeper", command.Name())
	require.Equal(t, identityCmd, command.Parent())
	for _, name := range []string{"file", "signer-socket", "stamp"} {
		require.NotNil(t, command.Flags().Lookup(name), name)
	}
	require.Equal(t, "false", command.Flags().Lookup("stamp").DefValue)
}

func TestIdentitySignKeeperUnreachable(t *testing.T) {
	const doc = `{"alg_version":1,"kind":"keeper.control","keeper":"worker","action":"pause","from":"owner","key_epoch":1}`
	for _, fromFile := range []bool{false, true} {
		t.Run(map[bool]string{false: "stdin", true: "file"}[fromFile], func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
			t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
			args := []string{"identity", "sign-keeper", "--signer-socket", filepath.Join(dir, "absent.sock"), "--stamp"}
			if fromFile {
				path := filepath.Join(dir, "request.json")
				require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))
				args = append(args, "--file", path)
			} else {
				RootCmd.SetIn(strings.NewReader(doc))
				t.Cleanup(func() { RootCmd.SetIn(os.Stdin) })
			}
			out, _, err := runRootCmd(t, args...)
			require.ErrorContains(t, err, "signer: unreachable")
			require.Empty(t, out.String())
		})
	}
}

type keeperErrorReader struct{ err error }

func (r keeperErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestIdentitySignKeeperReadErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	out, _, err := runRootCmd(t, "identity", "sign-keeper", "--file", filepath.Join(dir, "missing.json"))
	require.ErrorContains(t, err, "reading keeper document file")
	require.Empty(t, out.String())
	readErr := errors.New("stdin read failed")
	RootCmd.SetIn(keeperErrorReader{readErr})
	t.Cleanup(func() { RootCmd.SetIn(os.Stdin) })
	out, _, err = runRootCmd(t, "identity", "sign-keeper", "--signer-socket", filepath.Join(dir, "absent.sock"))
	require.ErrorIs(t, err, readErr)
	require.ErrorContains(t, err, "reading keeper document from stdin")
	require.Empty(t, out.String())
}
