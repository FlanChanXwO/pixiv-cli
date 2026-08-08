package pixiv

import (
	"context"

	clioutput "github.com/FlanChanXwO/pixiv-cli/internal/cli/output"
	"github.com/spf13/cobra"
)

var visualRecordTypes = map[string]struct{}{
	"illust": {},
	"manga":  {},
	"ugoira": {},
}

var userRecordTypes = map[string]struct{}{
	"user": {},
}

func (a controller) actionInputArgs(usage string) cobra.PositionalArgs {
	return clioutput.ActionInputArgs(a.in, usage, a.usageError)
}

func (a controller) consumeActionRecords(cmd *cobra.Command, operation, onError string, allowedTypes map[string]struct{}, invoke func(context.Context, int64) error) error {
	return clioutput.ConsumeActionRecords(cmd.Context(), a.in, a.errOut, operation, onError, allowedTypes, invoke, a.usageError)
}
