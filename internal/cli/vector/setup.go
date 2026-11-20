package vector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/vectormodel"
	"github.com/spf13/cobra"
)

func setupCommand(env environment.Env, store config.Store) *cobra.Command {
	var model, base string
	var yes bool
	var proxy environment.ProxyOptions
	cmd := &cobra.Command{Use: "setup --model NAME", Short: "Probe and enable a cloud embedding model", Args: env.ExactArgs(0, "pixiv vector setup --model NAME [--base-url URL] [--yes]"), RunE: func(cmd *cobra.Command, _ []string) (err error) {
		// 1. 校验服务与模型，再取得覆盖整个配置流程的同步锁。
		if model == "" {
			return env.WrapUsage(errors.New("--model is required"))
		}
		c, err := store.Current()
		if err != nil {
			return err
		}
		if !cmd.Flags().Changed("base-url") {
			base = c.Vector.BaseURL
		}
		base, err = vectormodel.CanonicalBase(base)
		if err != nil {
			return env.WrapUsage(err)
		}
		identity, err := vectormodel.Identifier(model, base)
		if err != nil {
			return env.WrapUsage(err)
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
		// 2. 展示本地记录及上传范围，确认后私下取得鉴权。
		status, err := vectorindex.ReadStatus(cmd.Context(), root, identity)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(env.Err, "Artwork thumbnails (including private bookmarks and R18) and queries will be uploaded to %s and may incur usage charges.\n%d pages currently await this model. Enabling vector search records artwork titles, authors, tags and image URLs locally.\n", base, status.Index.Pending); err != nil {
			return err
		}
		if err = confirm(env, yes); err != nil {
			return err
		}
		key := ""
		oldBase, baseErr := vectormodel.CanonicalBase(c.Vector.BaseURL)
		if baseErr == nil && oldBase == base {
			key = c.Vector.APIKey
		}
		if key == "" {
			if environment.IsTTY(env.In) {
				prompt := env
				prompt.Out = env.Err
				key, err = prompt.PromptSecret("API key")
			} else {
				key, err = environment.ReadValue(env.In)
				key = strings.TrimSpace(key)
			}
			if err != nil {
				return err
			}
		}
		// 3. 文字和图片都试算通过后，一次提交完整配置。
		client, err := modelHTTP(c, cmd, proxy)
		if err != nil {
			return err
		}
		defer client.CloseIdleConnections()
		embedder, err := vectormodel.Open(vectormodel.Options{Name: model, BaseURL: base, APIKey: key}, client)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, embedder.Close()) }()
		if _, err = embedder.Probe(cmd.Context()); err != nil {
			return err
		}
		if err = store.WriteVector(&config.Vector{Model: model, BaseURL: base, APIKey: key}); err != nil {
			return err
		}
		_, err = fmt.Fprintf(env.Out, "Vector search is ready (model %s, cloud).\n", model)
		return err
	}}
	cmd.Flags().StringVar(&model, "model", "", "cloud multimodal model ID")
	cmd.Flags().StringVar(&base, "base-url", "", "Gemini-compatible service URL")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	env.BindProxyFlags(cmd, &proxy)
	env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func modelHTTP(c config.Config, cmd *cobra.Command, options environment.ProxyOptions) (*http.Client, error) {
	proxy := c.Network.HTTPSProxy
	override, err := environment.ProxyOverride(cmd, options)
	if err != nil {
		return nil, err
	}
	if override != nil {
		proxy = *override
	}
	return config.HTTPClient(proxy)
}

func configuredModel(c config.Vector) (string, error) {
	if c.Model == "" {
		return "", nil
	}
	return vectormodel.Identifier(c.Model, c.BaseURL)
}

func openModel(ctx context.Context, c config.Vector, client *http.Client) (*vectormodel.Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Model == "" {
		return nil, errors.New("vector search is not set up; run \"pixiv vector setup\"")
	}
	return vectormodel.Open(vectormodel.Options{Name: c.Model, BaseURL: c.BaseURL, APIKey: c.APIKey}, client)
}
