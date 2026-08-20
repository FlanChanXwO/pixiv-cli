package commands

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleIsStoredOnCommandAndInheritedByChildren(t *testing.T) {
	root := &cobra.Command{Use: "pixiv"}
	group := &cobra.Command{Use: "fanbox"}
	leaf := &cobra.Command{Use: "posts"}
	root.AddCommand(group)
	group.AddCommand(leaf)

	Bind(group, FanboxData())

	assert.Equal(t, FanboxData(), For(leaf))
	require.Contains(t, group.Annotations, LifecycleAnnotationKey)
}

func TestLifecycleOverrideChangesOnlyTargetCommand(t *testing.T) {
	group := &cobra.Command{Use: "auth"}
	leaf := &cobra.Command{Use: "import"}
	group.AddCommand(leaf)
	Bind(group, AuthAccount())

	Override(leaf, AuthBundleImport())

	assert.Equal(t, AuthBundleImport(), For(leaf))
	assert.Equal(t, AuthAccount(), For(group))
}

func TestLifecycleDefaultsToStartup(t *testing.T) {
	assert.Equal(t, Execution{StartupHooks: true}, For(&cobra.Command{Use: "unknown"}))
}
