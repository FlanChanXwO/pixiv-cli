// List execution and output shared by Pixiv data commands: paging flags, the
// single-array JSON spool, and the three entity runners.

package pixivcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

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

// Plan is the resolved logical page selection.
type Plan struct {
	limit    int
	skip     int
	oneBatch bool
}

// PagePlan exposes the resolved selection to commands that drive shared
// pagination directly.
func (p Plan) PagePlan() pagination.PagePlan {
	return pagination.PagePlan{Skip: p.skip, Limit: p.limit, OneBatch: p.oneBatch}
}

// PushDownSkip hands a logical skip to an upstream offset only when raw and
// logical results correspond one-to-one.
func (p Plan) PushDownSkip() (Plan, int) {
	offset := p.skip
	p.skip = 0
	return p, offset
}

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

// ParsePlan validates a command's logical pagination settings.
func ParsePlan(cmd *cobra.Command, limit, page int) (Plan, error) {
	if limit < 0 {
		return Plan{}, errors.New("limit must be zero or a positive integer")
	}
	if cmd.Flags().Changed("page") && page <= 0 {
		return Plan{}, errors.New("page must be a positive integer")
	}
	if page > 0 {
		if limit <= 0 {
			return Plan{}, errors.New("--page requires --limit to be a positive integer")
		}
		if page-1 > math.MaxInt/limit {
			return Plan{}, errors.New("page and limit overflow the logical result offset")
		}
		return Plan{limit: limit, skip: (page - 1) * limit}, nil
	}
	return Plan{limit: limit, oneBatch: !cmd.Flags().Changed("limit")}, nil
}

// RunArtworks executes one paged artwork read under the pool's replay boundary.
// NDJSON commits per record, JSON after the whole document reaches stdout, and
// text after the first visible batch. A 429 therefore only switches accounts
// before any result is exposed.
func (r Runner) RunArtworks(ctx context.Context, request Request, plan Plan, format Format, list ArtworkList) error {
	key := list.Key
	if key == "" {
		key = JSONKeyIllusts
	}
	return runList(ctx, r, request, plan, format, key, list.Heading, list.Fetch, list.PostFilter, list.Print, record.RecordFromArtwork)
}

// RunNovels executes one paged novel read under the same replay boundary.
func (r Runner) RunNovels(ctx context.Context, request Request, plan Plan, format Format, list NovelList) error {
	return runList(ctx, r, request, plan, format, JSONKeyNovels, Static(list.Heading), list.Fetch, nil, func(items []pixiv.Novel, _ int) error {
		return list.Print(items)
	}, record.RecordFromNovel)
}

// RunUsers executes one paged user-preview read. The heading is evaluated only
// before the first visible output so identity reads stay replayable.
func (r Runner) RunUsers(ctx context.Context, request Request, plan Plan, format Format, list UserList) error {
	return runList(ctx, r, request, plan, format, JSONKeyUsers, list.Heading, list.Fetch, nil, func(items []pixiv.UserPreview, _ int) error {
		return list.Print(items)
	}, record.RecordFromUserPreview)
}

func runList[T any](ctx context.Context, r Runner, request Request, plan Plan, format Format, key JSONKey, heading func() string, fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), postFilter func([]T) []T, print func([]T, int) error, toRecord func(T) (record.Record, error)) error {
	var spool *jsonArraySpool
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
		spool, err = newJSONArraySpool(key)
		return err
	}
	position := plan.skip
	headingWritten := false
	encoder := json.NewEncoder(r.out)
	err := runPages(ctx, r.Execute(request), plan.PagePlan(), begin,
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
				return false, appendJSONArray(spool, items)
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

func runPages[C any, T any](ctx context.Context, execute pagination.Execute[C], plan pagination.PagePlan, begin func() error, fetch func(context.Context, C, sdk.Cursor) ([]T, sdk.Cursor, error), consume func([]T) (bool, error)) error {
	if execute == nil {
		return sdk.NewError("pixiv", "PagedRead", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}
	_, err := pagination.TraverseWith(ctx, execute, plan, begin, fetch, consume)
	return err
}

// PageItems hands a CLI page plan to shared pagination for commands that run
// their own inline read.
func PageItems[T any](ctx context.Context, plan Plan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), consume func([]T) error) error {
	_, err := pagination.TraversePages(ctx, plan.PagePlan(), fetch, consume)
	return err
}

// jsonArraySpool writes each confirmed element to a private temporary file.
// Network or encoding failures leave stdout untouched; success then transfers
// one complete JSON document.
type jsonArraySpool struct {
	file  *os.File
	first bool
	key   string
}

func newJSONArraySpool(key JSONKey) (*jsonArraySpool, error) {
	file, err := os.CreateTemp("", "pixiv-cli-json-*.tmp")
	if err != nil {
		return nil, err
	}
	spool := &jsonArraySpool{file: file, first: true, key: string(key)}
	if _, err := fmt.Fprintf(file, "{\n  %q: [", key); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}
	return spool, nil
}

func appendJSONArray[T any](s *jsonArraySpool, items []T) error {
	for _, item := range items {
		if s.first {
			s.first = false
		} else if _, err := io.WriteString(s.file, ","); err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(item, "    ", "  ")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(s.file, "\n    %s", encoded); err != nil {
			return err
		}
	}
	return nil
}

func (s *jsonArraySpool) Commit(out io.Writer) error {
	if _, err := io.WriteString(s.file, "\n  ]\n}\n"); err != nil {
		return err
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(out, s.file)
	return err
}

func (s *jsonArraySpool) Close() {
	if s == nil || s.file == nil {
		return
	}
	name := s.file.Name()
	_ = s.file.Close()
	_ = os.Remove(name)
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
