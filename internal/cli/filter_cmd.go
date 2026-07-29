package cli

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/spf13/cobra"
)

type filterOptions struct {
	id       string
	typ      string
	tags     []string
	minViews int64
	minPages int64
	onError  string
}

func (a app) newFilterCommand() *cobra.Command {
	opts := filterOptions{onError: "skip"}
	cmd := &cobra.Command{
		Use:   "filter",
		Short: "Filter Pixiv NDJSON entity records from standard input",
		Args:  requireExactArgs(0, "pixiv filter [options]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runFilter(cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.id, "id", "", "only keep this record ID")
	flags.StringVar(&opts.typ, "type", "", "only keep this record type")
	flags.StringArrayVar(&opts.tags, "tag", nil, "require this exact tag (repeatable)")
	flags.Int64Var(&opts.minViews, "min-views", 0, "minimum total views")
	flags.Int64Var(&opts.minPages, "min-pages", 0, "minimum page count")
	flags.StringVar(&opts.onError, "on-error", opts.onError, "record failure strategy: skip or fail-fast")
	return cmd
}

func (a app) runFilter(cmd *cobra.Command, opts filterOptions) error {
	filter, failFast, err := filterSettings(cmd, opts)
	if err != nil {
		return err
	}
	return consumeNDJSONRecords(cmd.Context(), a.in, a.errOut, "filter", failFast, func(_ context.Context, record application.Record) error {
		if !record.Matches(filter) {
			return nil
		}
		if err := json.NewEncoder(a.out).Encode(record); err != nil {
			return fatalRecordPipeline(err)
		}
		return nil
	})
}

func filterSettings(cmd *cobra.Command, opts filterOptions) (application.RecordFilter, bool, error) {
	failFast, err := recordFailureStrategy(opts.onError)
	if err != nil {
		return application.RecordFilter{}, false, err
	}
	filter := application.RecordFilter{ID: opts.id, Type: opts.typ, Tags: opts.tags}
	if cmd.Flags().Changed("min-views") {
		if opts.minViews < 0 {
			return application.RecordFilter{}, false, newUsageError(errors.New("min-views must be zero or positive"))
		}
		value := opts.minViews
		filter.MinViews = &value
	}
	if cmd.Flags().Changed("min-pages") {
		if opts.minPages < 0 {
			return application.RecordFilter{}, false, newUsageError(errors.New("min-pages must be zero or positive"))
		}
		value := opts.minPages
		filter.MinPageCount = &value
	}
	return filter, failFast, nil
}
