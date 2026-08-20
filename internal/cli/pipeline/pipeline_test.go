package pipeline_test

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindTextValueReadsOneMissingValueWithoutSplittingOrTrimming(t *testing.T) {
	var got []string
	cmd := &cobra.Command{
		Use:  "search WORD",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			got = append([]string(nil), args...)
			return nil
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextValue,
		MinArgs:      1,
		MaxArgs:      1,
		Reader:       strings.NewReader("  alpha beta  \r\n"),
		UsageError:   func(err error) error { return err },
		FillPosition: 0,
	})

	cmd.SetArgs(nil)
	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{"  alpha beta  "}, got)
}

func TestBindTextValueDoesNotReadWhenExplicitValueIsPresent(t *testing.T) {
	reader := &mustNotReadReader{err: errors.New("stdin must not be read")}
	var got []string
	cmd := &cobra.Command{
		Use:  "search WORD",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			got = append([]string(nil), args...)
			return nil
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextValue,
		MinArgs:      1,
		MaxArgs:      1,
		Reader:       reader,
		UsageError:   func(err error) error { return err },
		FillPosition: 0,
	})

	cmd.SetArgs([]string{"explicit"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{"explicit"}, got)
	assert.Zero(t, reader.calls)
}

func TestBindTextValueReadsAnOmittedOptionalPosition(t *testing.T) {
	var got []string
	cmd := &cobra.Command{
		Use: "set KEY [VALUE]",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 2 {
				return errors.New("value is required")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			got = append([]string(nil), args...)
			return nil
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextValue,
		MinArgs:      1,
		MaxArgs:      2,
		Reader:       strings.NewReader("value\n"),
		UsageError:   func(err error) error { return err },
		FillPosition: 1,
	})

	cmd.SetArgs([]string{"key"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{"key", "value"}, got)
}

func TestBindTextValueReadsTheCompleteOSPipeValue(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	t.Cleanup(func() { _ = writer.Close() })

	var got string
	cmd := &cobra.Command{
		Use:  "detail ID_OR_URL",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			got = args[0]
			return nil
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextValue,
		MinArgs:      1,
		MaxArgs:      1,
		Reader:       reader,
		UsageError:   func(err error) error { return err },
		FillPosition: 0,
	})

	_, err = writer.Write([]byte("https://example.invalid/artworks/42\r\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	cmd.SetArgs(nil)
	require.NoError(t, cmd.Execute())
	assert.Equal(t, "https://example.invalid/artworks/42", got)
}

func TestBindTextValueKeepsAnEmptyOptionalInputOmitted(t *testing.T) {
	var got []string
	cmd := &cobra.Command{
		Use:  "list [USER_ID]",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			got = append([]string(nil), args...)
			return nil
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextValue,
		MinArgs:      0,
		MaxArgs:      1,
		Reader:       strings.NewReader("\n"),
		UsageError:   func(err error) error { return err },
		FillPosition: 0,
	})

	cmd.SetArgs(nil)
	require.NoError(t, cmd.Execute())
	assert.Empty(t, got)
}

func TestBindDoesNotReadWhenMoreThanOneRequiredValueIsMissing(t *testing.T) {
	reader := &mustNotReadReader{err: errors.New("stdin must not be read")}
	called := false
	cmd := &cobra.Command{
		Use:  "command FIRST SECOND",
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, _ []string) error {
			called = true
			return nil
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextValue,
		MinArgs:      2,
		MaxArgs:      2,
		Reader:       reader,
		UsageError:   func(err error) error { return err },
		FillPosition: 0,
	})

	cmd.SetArgs(nil)
	require.Error(t, cmd.Execute())
	assert.False(t, called)
	assert.Zero(t, reader.calls)
}

func TestBindTextOrRecordReplaysTheClassifyingBytes(t *testing.T) {
	input := " \n\t{\"id\":\"1\",\"type\":\"illust\",\"url\":\"https://example.invalid/1\"}\n"
	var got string
	var gotMode pipeline.Mode
	cmd := &cobra.Command{
		Use:  "download [SRC]",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			gotMode = pipeline.ModeOf(c)
			assert.Empty(t, args)
			body, err := io.ReadAll(pipeline.Reader(c, strings.NewReader("wrong")))
			got = string(body)
			return err
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextOrRecord,
		MinArgs:      1,
		MaxArgs:      1,
		Reader:       strings.NewReader(input),
		UsageError:   func(err error) error { return err },
		FillPosition: 0,
	})

	cmd.SetArgs(nil)
	require.NoError(t, cmd.Execute())
	assert.Equal(t, pipeline.RecordMode, gotMode)
	assert.Equal(t, input, got)
}

func TestBindTextOrRecordUsesOneRawTextValueWhenInputIsNotRecord(t *testing.T) {
	var got []string
	var gotMode pipeline.Mode
	cmd := &cobra.Command{
		Use:  "download [SRC]",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			gotMode = pipeline.ModeOf(c)
			got = append([]string(nil), args...)
			return nil
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextOrRecord,
		MinArgs:      1,
		MaxArgs:      1,
		Reader:       strings.NewReader(" 123  \n"),
		UsageError:   func(err error) error { return err },
		FillPosition: 0,
	})

	cmd.SetArgs(nil)
	require.NoError(t, cmd.Execute())
	assert.Equal(t, pipeline.TextMode, gotMode)
	assert.Equal(t, []string{" 123  "}, got)
}

func TestBindTextOrRecordDoesNotClassifyExplicitOpaqueValue(t *testing.T) {
	reader := &mustNotReadReader{err: errors.New("stdin must not be read")}
	var got []string
	cmd := &cobra.Command{
		Use:  "download [SRC]",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			assert.Equal(t, pipeline.TextMode, pipeline.ModeOf(c))
			got = append([]string(nil), args...)
			return nil
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextOrRecord,
		MinArgs:      1,
		MaxArgs:      1,
		Reader:       reader,
		UsageError:   func(err error) error { return err },
		FillPosition: 0,
	})

	cmd.SetArgs([]string{"{opaque-token"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{"{opaque-token"}, got)
	assert.Zero(t, reader.calls)
}

func TestBindTreatsDashAsAnExplicitTextValue(t *testing.T) {
	reader := &mustNotReadReader{err: errors.New("stdin must not be read")}
	var got []string
	cmd := &cobra.Command{
		Use:  "detail ID",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			got = append([]string(nil), args...)
			return nil
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextValue,
		MinArgs:      1,
		MaxArgs:      1,
		Reader:       reader,
		UsageError:   func(err error) error { return err },
		FillPosition: 0,
	})

	cmd.SetArgs([]string{"-"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{"-"}, got)
	assert.Zero(t, reader.calls)
}

func TestBindCanDisableImplicitInputForAZeroPositionCommand(t *testing.T) {
	reader := &mustNotReadReader{err: errors.New("stdin must not be read")}
	called := false
	cmd := &cobra.Command{
		Use:  "mcp",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			called = true
			return nil
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:      pipeline.NoInput,
		MinArgs:    0,
		MaxArgs:    0,
		Reader:     reader,
		UsageError: func(err error) error { return err },
	})

	cmd.SetArgs(nil)
	require.NoError(t, cmd.Execute())
	assert.True(t, called)
	assert.Zero(t, reader.calls)
}

type mustNotReadReader struct {
	err   error
	calls int
}

func (r *mustNotReadReader) Read([]byte) (int, error) {
	r.calls++
	return 0, r.err
}

var _ io.Reader = (*mustNotReadReader)(nil)
