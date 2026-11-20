package vector

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/spf13/cobra"
)

// SearchFunc 由组合根读取实时配置并载入图片来源，再调用共享搜索流程。
type SearchFunc func(context.Context, vectorindex.SearchInput, string, environment.PixivRequest) (vectorindex.SearchResponse, error)

func searchCommand(env environment.Env, search SearchFunc) *cobra.Command {
	var options environment.Options
	var source string
	var noSync, ndjson bool
	var limit, page int
	cmd := &cobra.Command{Use: "search [QUERY]", Short: "help.search_the_local_artwork_vector_index", Args: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("image") {
			if strings.TrimSpace(source) == "" {
				return env.WrapUsage(i18n.Errorf("error.image_requires_a_source"))
			}
			return env.ExactArgs(0, "pixiv vector search --image PATH_OR_URL")(cmd, args)
		}
		if err := env.ExactArgs(1, "pixiv vector search QUERY")(cmd, args); err != nil {
			return err
		}
		if strings.TrimSpace(args[0]) == "" {
			return env.WrapUsage(i18n.Errorf("error.query_must_not_be_empty"))
		}
		return nil
	}, RunE: func(cmd *cobra.Command, args []string) error {
		input := vectorindex.SearchInput{NoSync: noSync}
		if len(args) > 0 {
			input.Query = args[0]
		}
		if cmd.Flags().Changed("limit") {
			input.Limit = &limit
		}
		if cmd.Flags().Changed("page") {
			input.Page = &page
		}
		plan, err := pagination.ParseInput(input.Page, input.Limit)
		if err != nil {
			return env.WrapUsage(err)
		}
		if ndjson && cmd.Flags().Changed("json") {
			return env.WrapUsage(i18n.Errorf("error.ndjson_cannot_be_used_with_json"))
		}
		jsonOut := false
		if !ndjson {
			jsonOut, err = env.ResolveJSON(env.JSONOverride(cmd, options))
			if err != nil {
				return err
			}
		}
		stream := env.AutoNDJSON(cmd, ndjson, jsonOut)
		request, err := env.Request(cmd, options.ProxyOptions)
		if err != nil {
			return err
		}
		result, err := search(cmd.Context(), input, source, request)
		if err != nil {
			return err
		}
		var warnings []string
		if result.Index.Pending > 0 {
			warnings = append(warnings, fmt.Sprintf(env.Text("format.pages_are_not_embedded_yet_run_pixiv_vector"), result.Index.Pending))
		}
		if result.Index.Error != "" {
			warnings = append(warnings, fmt.Sprintf(env.Text("vector.catchup_stopped"), result.Index.Error))
		}
		if len(warnings) > 0 {
			if _, err = fmt.Fprintln(env.Err, env.Text("output.warning_prefix")+strings.Join(warnings, "; ")); err != nil {
				return err
			}
		}
		if jsonOut {
			return env.PrintJSON(result)
		}
		if len(result.Records) == 0 {
			_, err = fmt.Fprintln(env.Err, env.Text("output.no_results"))
			return err
		}
		if stream {
			for _, r := range result.Records {
				if err = record.WriteJSONLine(env.Out, r); err != nil {
					return err
				}
			}
			return nil
		}
		artworks := make([]record.Artwork, 0, len(result.Records))
		for _, r := range result.Records {
			var snapshot struct {
				Title          string
				User           record.User
				TotalBookmarks int `json:"total_bookmarks"`
			}
			body, err := json.Marshal(r)
			if err != nil {
				return err
			}
			if err = json.Unmarshal(body, &snapshot); err != nil {
				return err
			}
			id, err := strconv.ParseInt(r.ID(), 10, 64)
			if err != nil {
				return err
			}
			artworks = append(artworks, record.Artwork{ID: id, Title: snapshot.Title, User: snapshot.User, TotalBookmarks: snapshot.TotalBookmarks})
		}
		return display.PrintArtworksRanked(env.Out, env.Language, artworks, plan.Skip)
	}}
	cmd.Flags().StringVar(&source, "image", "", "flag.image_file_or_http_s_url")
	cmd.Flags().BoolVar(&noSync, "no-sync", false, "flag.search_existing_vectors_without_catch_up")
	environment.BindListFlags(cmd, &limit, &page)
	env.BindCommonFlags(cmd, &options)
	environment.BindNDJSONFlag(cmd, &ndjson)
	env.BindTextValueWhen(cmd, 1, 1, 0, func(cmd *cobra.Command, _ []string) bool { return !cmd.Flags().Changed("image") })
	environment.SetOffline(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}
