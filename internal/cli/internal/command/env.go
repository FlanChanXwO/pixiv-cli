// Package command owns the execution environment and lifecycle declarations
// shared by CLI command owners.

package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Env is the execution environment shared by every command owner. The root
// builds it once; command packages receive it as a constructor argument.
type Env struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
	// WrapUsage marks argument or input-contract errors with the CLI usage exit
	// code. Nil keeps the error unchanged.
	WrapUsage func(error) error
	// ResolveJSON returns the effective JSON setting; a nil override reads it
	// from runtime configuration.
	ResolveJSON func(*bool) (bool, error)
}

// ProxyOptions holds one command's --proxy/--no-proxy values.
type ProxyOptions struct {
	Proxy   string
	NoProxy bool
}

// Options are the values of the flags shared by data commands.
type Options struct {
	ProxyOptions
	JSON bool
}

// UsageError wraps a validation failure with the CLI usage exit code.
func (e Env) UsageError(err error) error {
	if err == nil || e.WrapUsage == nil {
		return err
	}
	return e.WrapUsage(err)
}

// ExactArgs, MinArgs, and MaxArgs report positional mismatches as ordinary
// command failures. A usage exit code is reserved for unknown options and
// explicit input-contract violations.
func (e Env) ExactArgs(count int, usage string) cobra.PositionalArgs {
	return requireArgs(usage, func(length int) bool { return length == count })
}

func (e Env) MinArgs(count int, usage string) cobra.PositionalArgs {
	return requireArgs(usage, func(length int) bool { return length >= count })
}

func (e Env) MaxArgs(count int, usage string) cobra.PositionalArgs {
	return requireArgs(usage, func(length int) bool { return length <= count })
}

func requireArgs(usage string, valid func(int) bool) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if !valid(len(args)) {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

// PrintJSON writes one indented JSON document followed by a newline.
func (e Env) PrintJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return err
	}
	_, err = io.WriteString(e.Out, out.String()+"\n")
	return err
}

// JSONOverride returns an explicit --json value, or nil to keep the configured
// setting.
func (e Env) JSONOverride(cmd *cobra.Command, opts Options) *bool {
	if !cmd.Flags().Changed("json") {
		return nil
	}
	value := opts.JSON
	return &value
}

// JSONOut resolves the configured JSON setting for one command execution.
func (e Env) JSONOut(override *bool) (bool, error) {
	if e.ResolveJSON == nil {
		if override != nil {
			return *override, nil
		}
		return false, errors.New("JSON output resolver is not configured")
	}
	return e.ResolveJSON(override)
}

// ProxyOverride parses the shared proxy flags. It returns nil when neither flag
// was given.
func (e Env) ProxyOverride(cmd *cobra.Command, opts ProxyOptions) (*string, error) {
	return proxyOverrideFromFlags(cmd, opts)
}

func proxyOverrideFromFlags(cmd *cobra.Command, opts ProxyOptions) (*string, error) {
	proxyChanged := cmd.Flags().Changed("proxy")
	noProxyChanged := cmd.Flags().Changed("no-proxy")
	if proxyChanged && noProxyChanged {
		return nil, errors.New("use either --proxy or --no-proxy, not both")
	}
	if noProxyChanged && opts.NoProxy {
		empty := ""
		return &empty, nil
	}
	if proxyChanged {
		return &opts.Proxy, nil
	}
	return nil, nil
}

// BindCommonFlags adds --json with its short form and the proxy overrides.
func (e Env) BindCommonFlags(cmd *cobra.Command, opts *Options) {
	cmd.Flags().BoolVarP(&opts.JSON, "json", "j", false, "print JSON")
	e.BindProxyFlags(cmd, &opts.ProxyOptions)
}

// BindActionFlags adds only the proxy overrides for commands without --json.
func (e Env) BindActionFlags(cmd *cobra.Command, opts *ProxyOptions) {
	e.BindProxyFlags(cmd, opts)
}

func (e Env) BindProxyFlags(cmd *cobra.Command, opts *ProxyOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.Proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	flags.BoolVar(&opts.NoProxy, "no-proxy", false, "clear the configured proxy for this command")
}

func (e Env) BindTextValue(cmd *cobra.Command, minArgs, maxArgs, fillPosition int) {
	input.Bind(cmd, input.InputSpec{
		Codec:        input.TextValue,
		MinArgs:      minArgs,
		MaxArgs:      maxArgs,
		FillPosition: fillPosition,
		Reader:       e.In,
		UsageError:   e.UsageError,
	})
}

func (e Env) BindTextValueWhen(cmd *cobra.Command, minArgs, maxArgs, fillPosition int, enabled func(*cobra.Command, []string) bool) {
	input.Bind(cmd, input.InputSpec{
		Codec:        input.TextValue,
		MinArgs:      minArgs,
		MaxArgs:      maxArgs,
		FillPosition: fillPosition,
		Reader:       e.In,
		UsageError:   e.UsageError,
		Enabled:      enabled,
	})
}

func (e Env) BindTextOrRecord(cmd *cobra.Command, minArgs, maxArgs, fillPosition int) {
	input.Bind(cmd, input.InputSpec{
		Codec:        input.TextOrRecord,
		MinArgs:      minArgs,
		MaxArgs:      maxArgs,
		FillPosition: fillPosition,
		Reader:       e.In,
		UsageError:   e.UsageError,
	})
}

func (e Env) BindNoInput(cmd *cobra.Command) {
	input.Bind(cmd, input.InputSpec{
		Codec:      input.NoInput,
		MinArgs:    0,
		MaxArgs:    0,
		Reader:     e.In,
		UsageError: e.UsageError,
	})
}

// ActionInputArgs is the shared positional contract for a TextOrRecord action.
func (e Env) ActionInputArgs(usage string) cobra.PositionalArgs {
	return input.ActionInputArgs(e.In, usage, e.UsageError)
}

// ConsumeActionRecords keeps canonical record parsing and line diagnostics in
// input. The caller owns the allowed entity types and invoked mutation.
func (e Env) ConsumeActionRecords(cmd *cobra.Command, operation, onError string, allowedTypes map[string]struct{}, invoke func(context.Context, int64) error) error {
	return input.ConsumeActionRecords(cmd.Context(), input.Reader(cmd, e.In), e.Err, operation, onError, allowedTypes, invoke, e.UsageError)
}

// AutoNDJSON keeps each list command's existing automatic record-stream rule:
// an explicit --json or resolved JSON mode wins, otherwise piped stdout streams
// records unless --ndjson was already requested.
func (e Env) AutoNDJSON(cmd *cobra.Command, ndjson, jsonOut bool) bool {
	if ndjson || jsonOut || cmd.Flags().Changed("json") {
		return ndjson
	}
	file, ok := e.Out.(interface{ Fd() uintptr })
	return ok && !term.IsTerminal(int(file.Fd()))
}

// OutputIsTTY reports whether stdout is a terminal.
func (e Env) OutputIsTTY() bool {
	file, ok := e.Out.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(file.Fd()))
}

// TextBool renders a boolean for line-oriented terminal output.
func TextBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
