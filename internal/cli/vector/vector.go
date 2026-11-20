// Package vector 提供向量索引的配置、同步、搜索、状态和清理命令。
//
// [New] 构造命令树；模型与索引操作由各自领域包负责。
package vector

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/vectormodel"
	"github.com/spf13/cobra"
)

// New 构造 vector 命令组，配置存储由组合根注入。
func New(env environment.Env, store config.Store, open OpenClient, search SearchFunc) *cobra.Command {
	cmd := &cobra.Command{Use: "vector", Short: "Search artwork by image content", RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	environment.Bind(cmd, environment.Execution{})
	env.BindNoInput(cmd)
	cmd.AddCommand(setupCommand(env, store), syncCommand(env, store, open), searchCommand(env, search), statusCommand(env, store), clearCommand(env, store))
	return cmd
}

func statusCommand(env environment.Env, store config.Store) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{Use: "status", Short: "Show local vector index status", Args: env.ExactArgs(0, "pixiv vector status"), RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := store.Current()
		if err != nil {
			return err
		}
		root, err := config.AppDataDir()
		if err != nil {
			return err
		}
		identity, err := configuredModel(c.Vector)
		if err != nil {
			return err
		}
		result, err := vectorindex.ReadStatus(cmd.Context(), root, identity)
		if err != nil {
			return err
		}
		if jsonOut {
			return env.PrintJSON(result)
		}
		state := "not set up"
		if result.Enabled {
			state = "enabled"
		}
		_, err = fmt.Fprintf(env.Out, "Vector search: %s\nModel: %s (%s)\nIndex: %d artworks, %d pages embedded, %d pending, %d unavailable, %d failed\nRecorded, not merged yet: %d\nBookmarks: %v\nDisk (bytes): %v\nModels in the index: %v\n", state, result.Model, c.Vector.Device, result.Index.Artworks, result.Index.Pages, result.Index.Pending, result.Index.Unavailable, result.Index.Failed, result.Recorded, result.Bookmarks, result.Disk, result.Models)
		return err
	}}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	env.BindNoInput(cmd)
	environment.SetOffline(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func clearCommand(env environment.Env, store config.Store) *cobra.Command {
	var unused, index, all, yes bool
	cmd := &cobra.Command{Use: "clear", Short: "Remove vector data", Args: env.ExactArgs(0, "pixiv vector clear (--unused | --index | --all) [--yes]"), RunE: func(cmd *cobra.Command, _ []string) (err error) {
		selected := 0
		for _, v := range []bool{unused, index, all} {
			if v {
				selected++
			}
		}
		if selected != 1 {
			return env.WrapUsage(errors.New("choose exactly one of --unused, --index, --all"))
		}
		root, err := config.AppDataDir()
		if err != nil {
			return err
		}
		release, acquired, err := vectorindex.TrySyncLock(root)
		if err != nil {
			return err
		}
		if !acquired {
			return errors.New("vector sync is busy")
		}
		defer func() { err = errors.Join(err, release()) }()
		path := vectorindex.Directory(root)
		size, err := vectorindex.DiskUsage(path)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(env.Err, "Remove vector data under %s (%d bytes total).\n", path, size); err != nil {
			return err
		}
		if index || all {
			if _, err = fmt.Fprintln(env.Err, "Recorded browsing history cannot be recovered; bookmarks can be synchronized again."); err != nil {
				return err
			}
		}
		if err = confirm(env, yes); err != nil {
			return err
		}
		if unused {
			var c config.Config
			c, err = store.Current()
			if err != nil {
				return err
			}
			if _, err = os.Stat(filepath.Join(path, "index.db")); err == nil {
				var db *vectorindex.Store
				db, err = vectorindex.Open(cmd.Context(), root)
				if err != nil {
					return err
				}
				defer func() { err = errors.Join(err, db.Close()) }()
				identity, identityErr := configuredModel(c.Vector)
				if identityErr != nil {
					return identityErr
				}
				if err = db.ClearUnused(cmd.Context(), identity); err != nil {
					return err
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return vectormodel.ClearUnused(path, c.Vector.Model)
		}
		if all {
			if err = store.WriteVector(nil); err != nil {
				return err
			}
		}
		if err = vectorindex.Clear(cmd.Context(), root, all); err != nil {
			if all {
				return fmt.Errorf("vector search disabled; could not remove remaining data: %w", err)
			}
			return err
		}
		_, err = fmt.Fprintln(env.Out, "Vector data cleared.")
		return err
	}}
	cmd.Flags().BoolVar(&unused, "unused", false, "remove other models and their vectors/runtimes")
	cmd.Flags().BoolVar(&index, "index", false, "remove all records and vectors")
	cmd.Flags().BoolVar(&all, "all", false, "remove vector data and disable recording")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	env.BindNoInput(cmd)
	environment.SetOffline(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func confirm(env environment.Env, yes bool) error {
	if yes {
		return nil
	}
	if !env.CanPrompt() {
		return env.WrapUsage(errors.New("non-interactive use requires --yes"))
	}
	ok, err := env.PromptConfirm("Continue?", false)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("canceled")
	}
	return nil
}
