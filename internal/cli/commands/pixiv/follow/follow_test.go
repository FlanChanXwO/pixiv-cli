package follow

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	pixivuser "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/user"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistersFollowMutations(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	if len(cmd.Commands()) != 2 || cmd.Commands()[0].Name() != "add" || cmd.Commands()[1].Name() != "remove" {
		t.Fatalf("unexpected follow leaves: %v", cmd.Commands())
	}
}

func TestFollowRecordTypesRemainUserOnly(t *testing.T) {
	assert.Equal(t, map[string]struct{}{"user": {}}, userRecordTypes)

	called := false
	var diagnostics bytes.Buffer
	err := pipeline.ConsumeActionRecords(
		context.Background(),
		strings.NewReader(`{"id":"42","type":"artwork","url":"https://www.pixiv.net/artworks/42"}`+"\n"),
		&diagnostics, "follow_add", "fail-fast", userRecordTypes,
		func(context.Context, int64) error { called = true; return nil },
		func(err error) error { return err },
	)
	require.Error(t, err)
	assert.Contains(t, diagnostics.String(), `"code":"unsupported_type"`)
	assert.False(t, called)
}

func TestFollowAddRejectsUnsupportedRestrictBeforeOpeningClient(t *testing.T) {
	pooledCalls := 0
	data := deps.Data{
		Input:       strings.NewReader(""),
		Output:      &bytes.Buffer{},
		ErrorOutput: &bytes.Buffer{},
		UsageError: func(err error) error {
			return err
		},
		Pooled: func(
			context.Context,
			deps.Request,
			func(context.Context, *pixiv.Client) (bool, error),
		) error {
			pooledCalls++
			return nil
		},
	}

	cmd := New(data)
	cmd.SetArgs([]string{"add", "123", "--restrict", "all"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	assert.Equal(t, 0, pooledCalls)
}

func TestFollowRejectsPixivURLBeforeOpeningClient(t *testing.T) {
	for _, action := range []string{"add", "remove"} {
		t.Run(action, func(t *testing.T) {
			pooledCalls := 0
			data := deps.Data{
				Input:       strings.NewReader(""),
				Output:      &bytes.Buffer{},
				ErrorOutput: &bytes.Buffer{},
				UsageError: func(err error) error {
					return err
				},
				Pooled: func(
					context.Context,
					deps.Request,
					func(context.Context, *pixiv.Client) (bool, error),
				) error {
					pooledCalls++
					return nil
				},
			}

			cmd := New(data)
			cmd.SetArgs([]string{action, "https://www.pixiv.net/users/123"})

			err := cmd.Execute()

			require.Error(t, err)
			assert.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
			assert.Equal(t, 0, pooledCalls)
		})
	}
}

func TestFollowMutationRoutesShareOwnerAndWireContract(t *testing.T) {
	tests := []struct {
		name     string
		command  func(deps.Data) *cobra.Command
		args     []string
		path     string
		restrict string
	}{
		{
			name:     "root add",
			command:  New,
			args:     []string{"add", "401", "--restrict", "private"},
			path:     "/v1/user/follow/add",
			restrict: "private",
		},
		{
			name: "user add",
			command: func(data deps.Data) *cobra.Command {
				return newUserAliasCommand(data)
			},
			args:     []string{"follow", "add", "401", "--restrict", "private"},
			path:     "/v1/user/follow/add",
			restrict: "private",
		},
		{
			name:    "root remove",
			command: New,
			args:    []string{"remove", "401"},
			path:    "/v1/user/follow/delete",
		},
		{
			name: "user remove",
			command: func(data deps.Data) *cobra.Command {
				return newUserAliasCommand(data)
			},
			args: []string{"follow", "remove", "401"},
			path: "/v1/user/follow/delete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, requests := newFollowHTTPTestData(t)
			cmd := tt.command(data)
			cmd.SetArgs(tt.args)

			require.NoError(t, cmd.Execute())
			require.Len(t, *requests, 1)
			assert.Equal(t, tt.path, (*requests)[0].path)
			assert.Equal(t, "401", (*requests)[0].values["user_id"])
			if tt.restrict == "" {
				assert.NotContains(t, (*requests)[0].values, "restrict")
			} else {
				assert.Equal(t, tt.restrict, (*requests)[0].values["restrict"])
			}
			assert.Empty(t, data.Output.(*bytes.Buffer).String())
		})
	}
}

type capturedFollowRequest struct {
	path   string
	values map[string]string
}

type followRoundTripFunc func(*http.Request) (*http.Response, error)

func (f followRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newFollowHTTPTestData(t *testing.T) (deps.Data, *[]capturedFollowRequest) {
	t.Helper()
	requests := make([]capturedFollowRequest, 0, 1)
	transport := followRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			return nil, err
		}
		values := make(map[string]string, len(req.PostForm))
		for key := range req.PostForm {
			values[key] = req.PostForm.Get(key)
		}
		requests = append(requests, capturedFollowRequest{path: req.URL.Path, values: values})
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader("{}")),
		}, nil
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	require.NoError(t, err)

	output := &bytes.Buffer{}
	return deps.Data{
		Input:       strings.NewReader(""),
		Output:      output,
		ErrorOutput: &bytes.Buffer{},
		UsageError: func(err error) error {
			return err
		},
		Pooled: func(ctx context.Context, _ deps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, client)
			return err
		},
	}, &requests
}

func newUserAliasCommand(data deps.Data) *cobra.Command {
	return pixivuser.New(pixivuser.Dependencies{
		Input:      data.Input,
		Output:     data.Output,
		UsageError: data.UsageError,
		JSONOut:    data.JSONOut,
		Pooled: func(ctx context.Context, request pixivuser.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			return data.Pooled(ctx, deps.Request{HTTPSProxyOverride: request.HTTPSProxyOverride}, attempt)
		},
		Follow: func() *cobra.Command {
			return New(data)
		},
	})
}
