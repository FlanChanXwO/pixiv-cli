package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type filterFailingWriter struct{ err error }

func (w filterFailingWriter) Write([]byte) (int, error) { return 0, w.err }

var _ io.Writer = filterFailingWriter{}

func TestFilterMatchesRecordsAndSkipsInvalidInputWithNDJSONDiagnostic(t *testing.T) {
	useTempPaths(t)
	input := strings.Join([]string{
		`{"id":71,"type":"illust","url":"https://www.pixiv.net/artworks/71","tags":["keep","other"],"total_view":500,"page_count":2,"unknown":{"retain":true,"version":"remove"},"schema":"remove"}`,
		`not-json`,
		`{"id":"72","type":"illust","url":"https://www.pixiv.net/artworks/72","tags":["keep"],"total_view":499,"page_count":3}`,
	}, "\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "filter", "--id", "71", "--type", "illust", "--tag", "keep", "--min-views", "500", "--min-pages", "2"}, strings.NewReader(input), &stdout, &stderr)
	assert.Equal(t, 1, code)
	var record map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &record))
	assert.Equal(t, "71", record["id"])
	assert.Equal(t, true, record["unknown"].(map[string]any)["retain"])
	assert.NotContains(t, string(stdout.Bytes()), "version")
	assert.NotContains(t, string(stdout.Bytes()), "schema")

	var diagnostic map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &diagnostic))
	assert.Equal(t, "record_error", diagnostic["kind"])
	assert.Equal(t, "filter", diagnostic["operation"])
	assert.Equal(t, float64(2), diagnostic["line"])
	assert.Equal(t, "invalid_record", diagnostic["code"])
	assert.NotContains(t, string(stderr.Bytes()), "error:")
}

func TestFilterFailFastAndEmptyInput(t *testing.T) {
	useTempPaths(t)
	valid := `{"id":"81","type":"illust","url":"https://www.pixiv.net/artworks/81"}`
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "filter", "--on-error", "fail-fast"}, strings.NewReader("not-json\n"+valid), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	require.Len(t, lines, 1)
	var diagnostic map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &diagnostic))
	assert.Equal(t, float64(1), diagnostic["line"])
	assert.Equal(t, "record_error", diagnostic["kind"])

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "filter"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code, stderr.String())
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestFilterDoesNotRequireLoggingOrConfigInitialization(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[logging]\nlevel = 'invalid'\n")))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "filter"}, strings.NewReader(`{"id":"82","type":"illust","url":"https://www.pixiv.net/artworks/82"}`), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), `"id":"82"`)

	_, missingConfigPath := useTempPaths(t)
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "filter"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	_, statErr := os.Stat(missingConfigPath)
	assert.True(t, os.IsNotExist(statErr), "filter must not initialize a configuration file")
}

func TestFilterRejectsInvalidOptionsAndArguments(t *testing.T) {
	useTempPaths(t)
	for _, args := range [][]string{
		{"pixiv", "filter", "unexpected"},
		{"pixiv", "filter", "--on-error", "continue"},
		{"pixiv", "filter", "--min-views", "-1"},
		{"pixiv", "filter", "--min-pages", "-1"},
		{"pixiv", "filter", "--json"},
		{"pixiv", "filter", "--proxy", "http://127.0.0.1:7890"},
	} {
		var stdout, stderr bytes.Buffer
		wantCode := 1
		if args[2] == "--on-error" || args[2] == "--min-views" || args[2] == "--min-pages" {
			wantCode = 2
		}
		assert.Equal(t, wantCode, Run(args, strings.NewReader(""), &stdout, &stderr), args)
		assert.Empty(t, stdout.String(), args)
	}
}

func TestFilterReturnsOutputWriteFailureWithoutMislabelingRecord(t *testing.T) {
	useTempPaths(t)
	var stderr bytes.Buffer
	writeErr := errors.New("downstream write failed")
	code := Run([]string{"pixiv", "filter"}, strings.NewReader(`{"id":"85","type":"illust","url":"https://www.pixiv.net/artworks/85"}`), filterFailingWriter{err: writeErr}, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "downstream write failed")
	assert.Contains(t, stderr.String(), "error:")
	assert.NotContains(t, stderr.String(), `"kind":"record_error"`)
}

func TestFilterTreatsEachRepeatedTagAsOneExactTag(t *testing.T) {
	useTempPaths(t)
	input := `{"id":"88","type":"illust","url":"https://www.pixiv.net/artworks/88","tags":["comma,tag","second"]}`
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "filter", "--tag", "comma,tag", "--tag", "second"}, strings.NewReader(input), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var record map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &record))
	assert.Equal(t, "88", record["id"])
}
