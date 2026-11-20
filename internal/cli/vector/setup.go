package vector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/vectormodel"
	"github.com/spf13/cobra"
)

func setupCommand(env environment.Env, store config.Store) *cobra.Command {
	var model, base, device string
	var yes bool
	var proxy environment.ProxyOptions
	cmd := &cobra.Command{Use: "setup", Short: "help.install_probe_and_enable_an_embedding_model", Args: env.ExactArgs(0, "pixiv vector setup [--model NAME] [--device cpu|gpu] [--yes]"), RunE: func(cmd *cobra.Command, _ []string) (err error) {
		c, err := store.Current()
		if err != nil {
			return err
		}
		local := vectormodel.IsLocal(model)
		if local {
			if device != "cpu" && device != "gpu" {
				return env.WrapUsage(i18n.Errorf("error.device_must_be_cpu_or_gpu"))
			}
		} else if cmd.Flags().Changed("device") {
			return env.WrapUsage(i18n.Errorf("error.device_applies_to_local_models"))
		}
		if local {
			if cmd.Flags().Changed("base-url") {
				return env.WrapUsage(i18n.Errorf("error.base_url_applies_to_cloud_models"))
			}
			base = ""
		} else {
			if !cmd.Flags().Changed("base-url") {
				base = c.Vector.BaseURL
			}
			base, err = vectormodel.CanonicalBase(base)
			if err != nil {
				return env.WrapUsage(err)
			}
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
			return i18n.Errorf("error.vector_sync_is_busy")
		}
		defer func() { err = errors.Join(err, release()) }()
		status, err := vectorindex.ReadStatus(cmd.Context(), root, identity)
		if err != nil {
			return err
		}
		if local {
			_, err = fmt.Fprintf(env.Err, env.Text("output.tokenizer_model_onnx_vision_model_onnx_and_onnx"), model, vectorindex.Directory(root))
		} else {
			_, err = fmt.Fprintf(env.Err, env.Text("output.artwork_thumbnails_including_private_bookmarks_and_r18_and"), base)
		}
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(env.Err, env.Text("output.pages_currently_await_this_model_enabling_vector_search"), status.Index.Pending); err != nil {
			return err
		}
		if err = confirm(env, yes); err != nil {
			return err
		}
		key := ""
		if !local {
			oldBase, baseErr := vectormodel.CanonicalBase(c.Vector.BaseURL)
			if baseErr == nil && oldBase == base {
				key = c.Vector.APIKey
			}
			if key == "" {
				if environment.IsTTY(env.In) {
					prompt := env
					prompt.Out = env.Err
					key, err = prompt.PromptSecret(env.Text("prompt.api_key"))
				} else {
					key, err = environment.ReadValue(env.In)
					key = strings.TrimSpace(key)
				}
				if err != nil {
					return err
				}
			}
		}
		client, err := modelHTTP(c, cmd, proxy)
		if err != nil {
			return err
		}
		defer client.CloseIdleConnections()
		options := vectormodel.Options{Name: model, BaseURL: base, APIKey: key, Directory: vectorindex.Directory(root)}
		activeDevice := "cloud"
		selected := config.VectorDevice("")
		if local {
			activeDevice = device
			selected = config.VectorDevice(device)
			options.Device = device
			if err = vectormodel.Install(cmd.Context(), options, client, func(message string) error {
				_, err := fmt.Fprintf(env.Err, env.Text("model.downloading")+"\n", strings.TrimPrefix(message, "Downloading "))
				return err
			}); err != nil {
				return err
			}
		}
		embedder, err := vectormodel.Open(options, client)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, embedder.Close()) }()
		if _, err = embedder.Probe(cmd.Context()); err != nil {
			return err
		}
		if local && device == "gpu" {
			score, probeErr := embedder.ProbeGPU(cmd.Context())
			if probeErr != nil {
				return probeErr
			}
			if _, err = fmt.Fprintf(env.Err, env.Text("output.cpu_gpu_probe_cosine"), score); err != nil {
				return err
			}
		}
		if err = store.WriteVector(&config.Vector{Model: model, Device: selected, BaseURL: base, APIKey: key}); err != nil {
			return err
		}
		_, err = fmt.Fprintf(env.Out, env.Text("output.vector_search_is_ready_model"), model, activeDevice)
		return err
	}}
	cmd.Flags().StringVar(&model, "model", vectormodel.DefaultModel, "flag.built_in_local_model_or_cloud_multimodal_model")
	cmd.Flags().StringVar(&device, "device", "cpu", "flag.local_inference_device_cpu_or_gpu")
	cmd.Flags().StringVar(&base, "base-url", "", "flag.gemini_compatible_service_url")
	cmd.Flags().BoolVar(&yes, "yes", false, "flag.skip_confirmation")
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
		return nil, i18n.Errorf("error.vector_search_is_not_set_up_run_pixiv")
	}
	root, err := config.AppDataDir()
	if err != nil {
		return nil, err
	}
	return vectormodel.Open(vectormodel.Options{Name: c.Model, BaseURL: c.BaseURL, APIKey: c.APIKey, Device: string(c.Device), Directory: vectorindex.Directory(root)}, client)
}
