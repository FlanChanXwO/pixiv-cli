// List execution and output shared by Pixiv data commands: paging flags, the
// single-array JSON spool, and the three entity runners.

package pixivcmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// Format is a list command's output format, parsed from --json/--ndjson. The
// two flags are mutually exclusive.
type Format int

const (
	FormatText Format = iota
	FormatJSON
	FormatNDJSON
)

// JSONKey is the top-level key of a list JSON document. These are published
// output contracts and cannot be renamed.
type JSONKey string

const (
	JSONKeyIllusts JSONKey = "illusts"
	JSONKeyManga   JSONKey = "manga"
	JSONKeyNovels  JSONKey = "novels"
	JSONKeyUsers   JSONKey = "user_previews"
)

// ArtworkList describes one artwork list read and how to present it.
type ArtworkList struct {
	// Heading is evaluated after the pooled account is known.
	Heading func() string
	// Key defaults to JSONKeyIllusts; a few commands keep a different key.
	Key JSONKey
	// Fetch reads one upstream batch.
	Fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error)
	// PostFilter narrows the resolved window locally; it never scans further.
	PostFilter func([]pixiv.Artwork) []pixiv.Artwork
	Print      func(items []pixiv.Artwork, start int) error
}

// NovelList describes one novel list read.
type NovelList struct {
	Heading string
	Fetch   func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error)
	Print   func(items []pixiv.Novel) error
}

// UserList describes one user list read.
type UserList struct {
	Heading func() string
	Fetch   func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error)
	Print   func(items []pixiv.UserPreview) error
}

// Static wraps a fixed heading.
func Static(heading string) func() string { return func() string { return heading } }

// BindNDJSONFlag adds the record-stream switch owned by a list command.
func BindNDJSONFlag(cmd *cobra.Command, value *bool) {
	cmd.Flags().BoolVar(value, "ndjson", false, "print one Pixiv entity record as JSON per line")
}

// BindListFlags adds logical pagination flags to one list command.
func BindListFlags(cmd *cobra.Command, limit, page *int) {
	flags := cmd.Flags()
	flags.SetOutput(cmd.ErrOrStderr())
	flags.IntVarP(limit, "limit", "l", 0, "maximum results; omitted returns one upstream batch; 0 returns all results")
	flags.IntVarP(page, "page", "p", 0, "1-based logical page (requires --limit > 0)")
}

// RunArtworks executes one paged artwork read under the pool's replay boundary.
// NDJSON commits per record, JSON after the whole document reaches stdout, and
// text after the first visible batch. A 429 therefore only switches accounts
// before any result is exposed.
func (r Runner) RunArtworks(ctx context.Context, request Request, plan command.Plan, format Format, list ArtworkList) error {
	key := list.Key
	if key == "" {
		key = JSONKeyIllusts
	}
	return runList(ctx, r, request, plan, format, key, list.Heading, list.Fetch, list.PostFilter, list.Print, record.RecordFromArtwork)
}

// RunNovels executes one paged novel read under the same replay boundary.
func (r Runner) RunNovels(ctx context.Context, request Request, plan command.Plan, format Format, list NovelList) error {
	return runList(ctx, r, request, plan, format, JSONKeyNovels, Static(list.Heading), list.Fetch, nil, func(items []pixiv.Novel, _ int) error {
		return list.Print(items)
	}, record.RecordFromNovel)
}

// RunUsers executes one paged user-preview read. The heading is evaluated only
// before the first visible output so identity reads stay replayable.
func (r Runner) RunUsers(ctx context.Context, request Request, plan command.Plan, format Format, list UserList) error {
	return runList(ctx, r, request, plan, format, JSONKeyUsers, list.Heading, list.Fetch, nil, func(items []pixiv.UserPreview, _ int) error {
		return list.Print(items)
	}, record.RecordFromUserPreview)
}

func runList[T any](ctx context.Context, r Runner, request Request, plan command.Plan, format Format, key JSONKey, heading func() string, fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), postFilter func([]T) []T, print func([]T, int) error, toRecord func(T) (record.Record, error)) error {
	var spool *command.JSONArraySpool
	if format == FormatJSON {
		defer func() {
			if spool != nil {
				spool.Close()
			}
		}()
	}
	begin := func() error {
		if format != FormatJSON {
			return nil
		}
		if spool != nil {
			spool.Close()
		}
		var err error
		spool, err = command.NewJSONArraySpool(string(key))
		return err
	}
	position := plan.PagePlan().Skip
	headingWritten := false
	encoder := json.NewEncoder(r.out)
	_, err := pagination.TraverseWith(ctx, r.Execute(request), plan.PagePlan(), begin,
		func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}, func(items []T) (bool, error) {
			if postFilter != nil {
				items = postFilter(items)
			}
			if format == FormatNDJSON {
				committed := false
				for _, item := range items {
					value, err := toRecord(item)
					if err != nil {
						return committed, err
					}
					committed = true
					if err := encoder.Encode(value); err != nil {
						return committed, err
					}
				}
				return committed, nil
			}
			if format == FormatJSON {
				return false, command.AppendJSONArray(spool, items)
			}
			committed := false
			if !headingWritten && heading != nil {
				if text := heading(); text != "" {
					committed = true
					if _, err := fmt.Fprintln(r.out, text); err != nil {
						return committed, err
					}
				}
				headingWritten = true
			}
			if err := print(items, position); err != nil {
				return true, err
			}
			position += len(items)
			return true, nil
		})
	if format == FormatJSON && err == nil {
		return spool.Commit(r.out)
	}
	return err
}

// FormatOf maps the two output flags to one list format.
func FormatOf(jsonOut, ndjson bool) Format {
	if ndjson {
		return FormatNDJSON
	}
	if jsonOut {
		return FormatJSON
	}
	return FormatText
}
