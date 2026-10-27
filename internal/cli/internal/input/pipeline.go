// Package pipeline owns the CLI's explicit stdin input codecs and their
// resolved per-command state. It does not know any Pixiv or FANBOX business
// semantics.
package input

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Codec identifies the input protocol a command explicitly owns.
type Codec string

const (
	// NoInput keeps stdin reserved for command-specific protocols such as MCP.
	NoInput Codec = "none"
	// TextValue maps the complete stdin stream to one positional value.
	TextValue Codec = "text-value"
	// TextOrRecord distinguishes one raw value from the canonical NDJSON stream.
	TextOrRecord Codec = "text-or-record"
)

// Mode is the result of resolving a TextOrRecord stream.
type Mode string

const (
	TextMode   Mode = "text"
	RecordMode Mode = "record"
)

// InputSpec is the explicit input contract for one Cobra command.
//
// FillPosition is the zero-based positional slot filled by stdin. A command
// with one required or optional value normally uses zero. A command such as
// "config set KEY [VALUE]" uses one, so an explicit KEY remains the first
// argument and only the omitted VALUE is read. MaxArgs is -1 for variadic
// commands.
type InputSpec struct {
	Codec        Codec
	MinArgs      int
	MaxArgs      int
	FillPosition int
	Reader       io.Reader
	UsageError   func(error) error
	Enabled      func(*cobra.Command, []string) bool
	IsTTY        func(io.Reader) bool
}

// AnnotationKey lets architecture checks prove that a command has a declared
// codec without inspecting Use strings or inferring arity from help text.
const AnnotationKey = "pixiv-cli.input-spec"

// State holds one command instance's resolved input. The command constructor
// owns it; nothing is keyed by *cobra.Command or shared between executions.
type State struct {
	resolved *resolvedInput
}

type resolvedInput struct {
	args   []string
	reader io.Reader
	mode   Mode
}

// Args returns the positional arguments after stdin resolution. It falls back
// to the caller's slice when Args has not run, for example when a test invokes
// RunE directly.
func (s *State) Args(fallback []string) []string {
	if s == nil || s.resolved == nil {
		return fallback
	}
	return s.resolved.args
}

// Reader returns the replayable reader selected by the codec. Commands that do
// not read stdin keep the configured reader.
func (s *State) Reader(fallback io.Reader) io.Reader {
	if s == nil || s.resolved == nil || s.resolved.reader == nil {
		return fallback
	}
	return s.resolved.reader
}

// Mode reports how the TextOrRecord stream was classified.
func (s *State) Mode() Mode {
	if s == nil || s.resolved == nil {
		return TextMode
	}
	return s.resolved.mode
}

// Bind attaches an InputSpec to cmd and returns the state resolved for this
// command instance. It wraps the existing positional validator and RunE so
// business code observes the resolved positional slice, while record consumers
// can retrieve the replayable reader from State.Reader.
func Bind(cmd *cobra.Command, spec InputSpec) *State {
	if cmd == nil {
		return nil
	}
	state := &State{}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[AnnotationKey] = annotationValue(spec)
	if spec.Codec == NoInput {
		// NoInput is a declaration for architecture checks and command-tree
		// inventory only. Protocol owners such as MCP keep their original Args
		// and RunE so the generic resolver never touches their input stream.
		return state
	}

	validate := cmd.Args
	if validate == nil {
		validate = cobra.ArbitraryArgs
	}
	run := cmd.RunE
	cmd.Args = func(command *cobra.Command, args []string) error {
		resolved, err := resolve(command, args, spec)
		if err != nil {
			return inputError(spec, err)
		}
		state.resolved = &resolved
		return validate(command, resolved.args)
	}
	if run != nil {
		cmd.RunE = func(command *cobra.Command, args []string) error {
			return run(command, state.Args(args))
		}
	}
	return state
}

func annotationValue(spec InputSpec) string {
	return string(spec.Codec) + ":" + strconv.Itoa(spec.MinArgs) + ":" + strconv.Itoa(spec.MaxArgs) + ":" + strconv.Itoa(spec.FillPosition)
}

func inputError(spec InputSpec, err error) error {
	if err == nil || spec.UsageError == nil {
		return err
	}
	return spec.UsageError(err)
}

func resolve(command *cobra.Command, args []string, spec InputSpec) (resolvedInput, error) {
	resolved := resolvedInput{
		args:   append([]string(nil), args...),
		reader: spec.Reader,
		mode:   modeForCodec(spec.Codec),
	}
	if spec.Codec == NoInput || !enabled(command, spec, args) || !shouldRead(command, args, spec) {
		return resolved, nil
	}
	if spec.Reader == nil {
		return resolvedInput{}, errors.New("stdin input reader is not configured")
	}

	switch spec.Codec {
	case TextValue:
		value, err := ReadValue(spec.Reader)
		if err != nil {
			return resolvedInput{}, fmt.Errorf("read stdin value: %w", err)
		}
		if value != "" {
			resolved.args = append(resolved.args, value)
		}
		resolved.mode = TextMode
		resolved.reader = bytes.NewReader(nil)
		return resolved, nil
	case TextOrRecord:
		mode, reader, value, err := resolveTextOrRecord(spec.Reader)
		if err != nil {
			return resolvedInput{}, fmt.Errorf("read stdin input: %w", err)
		}
		resolved.mode = mode
		resolved.reader = reader
		if mode == TextMode && value != "" {
			resolved.args = append(resolved.args, value)
		}
		return resolved, nil
	default:
		return resolvedInput{}, fmt.Errorf("unsupported stdin codec %q", spec.Codec)
	}
}

func enabled(command *cobra.Command, spec InputSpec, args []string) bool {
	return spec.Enabled == nil || spec.Enabled(command, args)
}

func shouldRead(command *cobra.Command, args []string, spec InputSpec) bool {
	if len(args) != spec.FillPosition {
		return false
	}
	if spec.MaxArgs >= 0 && len(args) >= spec.MaxArgs {
		return false
	}
	// If the command is missing two or more required positional values, keep
	// Cobra's original usage error and do not guess which value stdin represents.
	if spec.MinArgs > spec.FillPosition+1 {
		return false
	}
	checker := spec.IsTTY
	if checker == nil {
		checker = IsTTY
	}
	return !checker(spec.Reader)
}

func resolveTextOrRecord(reader io.Reader) (Mode, io.Reader, string, error) {
	prefix := make([]byte, 0, 1)
	for {
		var one [1]byte
		n, err := reader.Read(one[:])
		if n > 0 {
			prefix = append(prefix, one[0])
			if !isJSONWhitespace(one[0]) {
				if one[0] == '{' {
					return RecordMode, io.MultiReader(bytes.NewReader(prefix), reader), "", nil
				}
				body, readErr := io.ReadAll(io.MultiReader(bytes.NewReader(prefix), reader))
				if readErr != nil {
					return TextMode, nil, "", readErr
				}
				return TextMode, bytes.NewReader(nil), string(StripOneTrailingLineEnding(body)), nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return TextMode, bytes.NewReader(nil), string(StripOneTrailingLineEnding(prefix)), nil
			}
			return TextMode, nil, "", err
		}
		if n == 0 {
			continue
		}
	}
}

func modeForCodec(codec Codec) Mode {
	if codec == TextOrRecord {
		return TextMode
	}
	return TextMode
}

// IsTTY recognizes only an actual terminal file. In-memory and pipe readers
// are intentionally treated as non-TTY so embedded callers get pipe semantics.
func IsTTY(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

// ReadValue reads the complete stream and removes exactly one trailing LF or
// CRLF. It does not trim whitespace, split words, or impose a size limit.
func ReadValue(reader io.Reader) (string, error) {
	if reader == nil {
		return "", errors.New("stdin input reader is nil")
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(StripOneTrailingLineEnding(body)), nil
}

// FirstNonWhitespace returns the first byte that is not JSON whitespace. The
// record codec only needs the JSON-defined leading whitespace set; keeping the
// check byte-oriented avoids decoding or rewriting opaque input bytes.
func FirstNonWhitespace(body []byte) (byte, bool) {
	for _, one := range body {
		if !isJSONWhitespace(one) {
			return one, true
		}
	}
	return 0, false
}

func isJSONWhitespace(one byte) bool {
	switch one {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

// StripOneTrailingLineEnding preserves all input except one final LF or CRLF.
func StripOneTrailingLineEnding(body []byte) []byte {
	if bytes.HasSuffix(body, []byte("\r\n")) {
		return body[:len(body)-2]
	}
	if bytes.HasSuffix(body, []byte("\n")) {
		return body[:len(body)-1]
	}
	return body
}
