package fanbox

import (
	"errors"
	"math"

	"github.com/spf13/cobra"
)

// listOptions 是 FANBOX 列表命令共享的逻辑分页参数。
type listOptions struct {
	limit int
	page  int
}

type listPlan struct {
	limit    int
	skip     int
	oneBatch bool
}

func bindListFlags(cmd *cobra.Command, options *listOptions) {
	flags := cmd.Flags()
	flags.SetOutput(cmd.ErrOrStderr())
	flags.IntVar(&options.limit, "limit", 0, "maximum results; omitted returns one upstream batch; 0 returns all results")
	flags.IntVar(&options.page, "page", 0, "1-based logical page (requires --limit > 0)")
}

func parseListPlan(cmd *cobra.Command, options listOptions) (listPlan, error) {
	if options.limit < 0 {
		return listPlan{}, errors.New("limit must be zero or a positive integer")
	}
	if cmd.Flags().Changed("page") && options.page <= 0 {
		return listPlan{}, errors.New("page must be a positive integer")
	}
	if options.page > 0 {
		if options.limit <= 0 {
			return listPlan{}, errors.New("--page requires --limit to be a positive integer")
		}
		if options.page-1 > math.MaxInt/options.limit {
			return listPlan{}, errors.New("page and limit overflow the logical result offset")
		}
		return listPlan{limit: options.limit, skip: (options.page - 1) * options.limit}, nil
	}
	return listPlan{limit: options.limit, oneBatch: !cmd.Flags().Changed("limit")}, nil
}
