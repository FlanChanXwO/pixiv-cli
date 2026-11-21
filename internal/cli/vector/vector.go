// Package vector 提供向量索引的配置、同步、添加、搜索、状态和清理命令。
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
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/vectormodel"
	"github.com/spf13/cobra"
)

// New 构造 vector 命令组，配置存储由组合根注入。
func New(env environment.Env, store config.Store, open OpenClient, search SearchFunc) *cobra.Command {
	cmd := &cobra.Command{Use: "vector", Short: "help.search_artwork_by_image_content", RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	environment.Bind(cmd, environment.Execution{})
	env.BindNoInput(cmd)
	cmd.AddCommand(setupCommand(env, store), syncCommand(env, store, open), addCommand(env, store, open), searchCommand(env, search), statusCommand(env, store), clearCommand(env, store))
	return cmd
}

func statusCommand(env environment.Env, store config.Store) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{Use: "status", Short: "help.show_local_vector_index_status", Args: env.ExactArgs(0, "pixiv vector status"), RunE: func(cmd *cobra.Command, _ []string) error {
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
		state := env.Text("state.not_set_up")
		if result.Enabled {
			state = env.Text("state.enabled")
		}
		_, err = fmt.Fprintf(env.Out, env.Text("output.vector_search_model_index_artworks_pages_embedded_pending"), state, result.Model, c.Vector.Device, result.Index.Artworks, result.Index.Pages, result.Index.Pending, result.Index.Unavailable, result.Index.Failed, result.Recorded, result.Bookmarks, result.Disk, result.Models)
		return err
	}}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "flag.print_json")
	env.BindNoInput(cmd)
	environment.SetOffline(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func clearCommand(env environment.Env, store config.Store) *cobra.Command {
	var unused, index, all, yes bool
	cmd := &cobra.Command{Use: "clear", Short: "help.remove_vector_data", Args: env.ExactArgs(0, "pixiv vector clear (--unused | --index | --all) [--yes]"), RunE: func(cmd *cobra.Command, _ []string) (err error) {
		selected := 0
		for _, v := range []bool{unused, index, all} {
			if v {
				selected++
			}
		}
		if selected != 1 {
			return env.WrapUsage(i18n.Errorf("error.choose_exactly_one_of_unused_index_all"))
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
			return i18n.Errorf("error.vector_sync_is_busy")
		}
		defer func() { err = errors.Join(err, release()) }()
		path := vectorindex.Directory(root)
		size, err := vectorindex.DiskUsage(path)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(env.Err, env.Text("output.remove_vector_data_under_bytes_total"), path, size); err != nil {
			return err
		}
		if index || all {
			if _, err = fmt.Fprintln(env.Err, env.Text("output.recorded_browsing_history_cannot_be_recovered_bookmarks_can")); err != nil {
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
			return vectormodel.ClearUnused(path, c.Vector.Model, string(c.Vector.Device))
		}
		if all {
			if err = store.WriteVector(nil); err != nil {
				return err
			}
		}
		if err = vectorindex.Clear(cmd.Context(), root, all); err != nil {
			if all {
				return i18n.Errorf("error.vector_search_disabled_could_not_remove_remaining_data", err)
			}
			return err
		}
		_, err = fmt.Fprintln(env.Out, env.Text("output.vector_data_cleared"))
		return err
	}}
	cmd.Flags().BoolVar(&unused, "unused", false, "flag.remove_other_models_and_their_vectors_runtimes")
	cmd.Flags().BoolVar(&index, "index", false, "flag.remove_all_records_and_vectors")
	cmd.Flags().BoolVar(&all, "all", false, "flag.remove_vector_data_and_disable_recording")
	cmd.Flags().BoolVar(&yes, "yes", false, "flag.skip_confirmation")
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
		return env.WrapUsage(i18n.Errorf("error.non_interactive_use_requires_yes"))
	}
	ok, err := env.PromptConfirm(env.Text("prompt.continue"), false)
	if err != nil {
		return err
	}
	if !ok {
		return i18n.Errorf("error.vector_operation_canceled")
	}
	return nil
}
