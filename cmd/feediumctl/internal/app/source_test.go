package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/app"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/app/mock"
)

// ── helpers ───────────────────────────────────────────────────────────────

func newSourceMock(t *testing.T) *mock.MockSourceServiceClient {
	t.Helper()
	ctrl := gomock.NewController(t)
	return mock.NewMockSourceServiceClient(ctrl)
}

func runSource(t *testing.T, client feediumapi.SourceServiceClient, args ...string) (stdout string, stderr string, err error) {
	t.Helper()
	cmd := app.NewRootCommandWithSource(app.FactoryFromSource(client))
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.ExecuteContext(context.Background())
	return outBuf.String(), errBuf.String(), err
}

// ── source --help (AC-S6, INV-03) ────────────────────────────────────────

func TestSourceHelp(t *testing.T) {
	client := newSourceMock(t) // no EXPECT: gomock fails on any RPC call
	stdout, _, err := runSource(t, client, "source", "--help")
	require.NoError(t, err)
	for _, sub := range []string{"list", "get", "create", "update", "delete"} {
		assert.Contains(t, stdout, sub, "help must mention subcommand %q", sub)
	}
}

func TestSource_UnknownSubcommand_EC(t *testing.T) {
	client := newSourceMock(t)
	stdout, _, err := runSource(t, client, "source", "bogus")
	require.Error(t, err)
	assert.Empty(t, stdout, "stdout must be empty on error (INV-02)")
	assert.Contains(t, app.FormatError(err), "flag: unknown subcommand")
}

// ── source list (SR-01, EC-C, EC-G, AC-S3) ───────────────────────────────

func TestSourceList_Request_PageSize(t *testing.T) {
	client := newSourceMock(t)
	var captured *feediumapi.V1ListSourcesRequest
	client.EXPECT().
		V1ListSources(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1ListSourcesRequest, _ ...grpc.CallOption) (*feediumapi.V1ListSourcesResponse, error) {
			captured = req
			return &feediumapi.V1ListSourcesResponse{}, nil
		})

	stdout, _, err := runSource(t, client, "source", "list", "--page-size=25")
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, int32(25), captured.GetPageSize())
	assert.Equal(t, "", captured.GetPageToken(), "page_token must be empty (SR-01)")
	assert.Nil(t, captured.Type, "--type not set → Type must be nil")
	assert.Contains(t, stdout, "ID | TYPE")
}

func TestSourceList_TypeFilter(t *testing.T) {
	client := newSourceMock(t)
	var captured *feediumapi.V1ListSourcesRequest
	client.EXPECT().
		V1ListSources(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1ListSourcesRequest, _ ...grpc.CallOption) (*feediumapi.V1ListSourcesResponse, error) {
			captured = req
			return &feediumapi.V1ListSourcesResponse{}, nil
		})

	_, _, err := runSource(t, client, "source", "list", "--type=RSS")
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.NotNil(t, captured.Type)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_RSS, *captured.Type)
}

func TestSourceList_EmptyItems_Table_EC_C(t *testing.T) {
	client := newSourceMock(t)
	client.EXPECT().
		V1ListSources(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1ListSourcesResponse{}, nil)

	stdout, _, err := runSource(t, client, "source", "list")
	require.NoError(t, err)
	assert.Equal(t, "ID | TYPE | MODE | CONFIG | CREATED_AT\n", stdout)
}

func TestSourceList_EmptyItems_JSON_EC_C(t *testing.T) {
	client := newSourceMock(t)
	client.EXPECT().
		V1ListSources(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1ListSourcesResponse{}, nil)

	stdout, _, err := runSource(t, client, "source", "list", "--output=json")
	require.NoError(t, err)
	assert.Equal(t, "[]\n", stdout)
}

func TestSourceList_EmptyItems_YAML_EC_C(t *testing.T) {
	client := newSourceMock(t)
	client.EXPECT().
		V1ListSources(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1ListSourcesResponse{}, nil)

	stdout, _, err := runSource(t, client, "source", "list", "--output=yaml")
	require.NoError(t, err)
	assert.Equal(t, "[]\n", stdout)
}

func TestSourceList_UnknownType_EC_G(t *testing.T) {
	client := newSourceMock(t) // no EXPECT: RPC must not fire
	stdout, _, err := runSource(t, client, "source", "list", "--type=unknown_type")
	require.Error(t, err)
	assert.Empty(t, stdout, "stdout must be empty on error (INV-02)")
	assert.Equal(t, `flag: unknown --type "unknown_type"`, app.FormatError(err))
}

// ── source get (SR-02, EC-A, AC-S3) ──────────────────────────────────────

func TestSourceGet_Request(t *testing.T) {
	client := newSourceMock(t)
	var captured *feediumapi.V1GetSourceRequest
	client.EXPECT().
		V1GetSource(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1GetSourceRequest, _ ...grpc.CallOption) (*feediumapi.V1GetSourceResponse, error) {
			captured = req
			return &feediumapi.V1GetSourceResponse{
				Source: &feediumapi.Source{Id: req.GetId(), Type: feediumapi.SourceType_SOURCE_TYPE_HTML},
			}, nil
		})

	stdout, _, err := runSource(t, client, "source", "get", "my-uuid-123")
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "my-uuid-123", captured.GetId())
	assert.Contains(t, stdout, "my-uuid-123")
	assert.Contains(t, stdout, "HTML")
}

func TestSourceGet_NotFound_EC_A(t *testing.T) {
	client := newSourceMock(t)
	client.EXPECT().
		V1GetSource(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.NotFound, "source not found"))

	stdout, _, err := runSource(t, client, "source", "get", "no-such-id")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "code=NotFound message=source not found", app.FormatError(err))
}

// ── source create (SR-03, EC-D, EC-E, EC-I, AC-S3, AC-S5) ───────────────

func TestSourceCreate_TelegramChannel(t *testing.T) {
	client := newSourceMock(t)
	var captured *feediumapi.V1CreateSourceRequest
	client.EXPECT().
		V1CreateSource(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1CreateSourceRequest, _ ...grpc.CallOption) (*feediumapi.V1CreateSourceResponse, error) {
			captured = req
			return &feediumapi.V1CreateSourceResponse{
				Source: &feediumapi.Source{Id: "new-id", Type: req.GetType()},
			}, nil
		})

	_, _, err := runSource(t, client, "source", "create", "telegram-channel", "--tg-id=99", "--username=chan")
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL, captured.GetType())
	tc, ok := captured.GetConfig().GetConfig().(*feediumapi.SourceConfig_TelegramChannel)
	require.True(t, ok)
	assert.Equal(t, int64(99), tc.TelegramChannel.GetTgId())
	assert.Equal(t, "chan", tc.TelegramChannel.GetUsername())
}

func TestSourceCreate_TelegramGroup(t *testing.T) {
	client := newSourceMock(t)
	var captured *feediumapi.V1CreateSourceRequest
	client.EXPECT().
		V1CreateSource(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1CreateSourceRequest, _ ...grpc.CallOption) (*feediumapi.V1CreateSourceResponse, error) {
			captured = req
			return &feediumapi.V1CreateSourceResponse{Source: &feediumapi.Source{Id: "g"}}, nil
		})

	_, _, err := runSource(t, client, "source", "create", "telegram-group", "--tg-id=7", "--username=grp")
	require.NoError(t, err)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_GROUP, captured.GetType())
	tg, ok := captured.GetConfig().GetConfig().(*feediumapi.SourceConfig_TelegramGroup)
	require.True(t, ok)
	assert.Equal(t, int64(7), tg.TelegramGroup.GetTgId())
}

func TestSourceCreate_RSS(t *testing.T) {
	client := newSourceMock(t)
	var captured *feediumapi.V1CreateSourceRequest
	client.EXPECT().
		V1CreateSource(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1CreateSourceRequest, _ ...grpc.CallOption) (*feediumapi.V1CreateSourceResponse, error) {
			captured = req
			return &feediumapi.V1CreateSourceResponse{Source: &feediumapi.Source{Id: "r"}}, nil
		})

	_, _, err := runSource(t, client, "source", "create", "rss", "--feed-url=https://x.com/feed")
	require.NoError(t, err)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_RSS, captured.GetType())
	r, ok := captured.GetConfig().GetConfig().(*feediumapi.SourceConfig_Rss)
	require.True(t, ok)
	assert.Equal(t, "https://x.com/feed", r.Rss.GetFeedUrl())
}

func TestSourceCreate_HTML(t *testing.T) {
	client := newSourceMock(t)
	var captured *feediumapi.V1CreateSourceRequest
	client.EXPECT().
		V1CreateSource(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1CreateSourceRequest, _ ...grpc.CallOption) (*feediumapi.V1CreateSourceResponse, error) {
			captured = req
			return &feediumapi.V1CreateSourceResponse{Source: &feediumapi.Source{Id: "h"}}, nil
		})

	_, _, err := runSource(t, client, "source", "create", "html", "--url=https://x.com")
	require.NoError(t, err)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_HTML, captured.GetType())
	h, ok := captured.GetConfig().GetConfig().(*feediumapi.SourceConfig_Html)
	require.True(t, ok)
	assert.Equal(t, "https://x.com", h.Html.GetUrl())
}

func TestSourceCreate_UnknownType_EC_E(t *testing.T) {
	client := newSourceMock(t) // no EXPECT: RPC must not fire
	stdout, _, err := runSource(t, client, "source", "create", "ftp", "--feed-url=x")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, `flag: unknown source type "ftp" (allowed: html,rss,telegram-channel,telegram-group)`, app.FormatError(err))
}

func TestSourceCreate_MissingRequired_EC_D(t *testing.T) {
	client := newSourceMock(t)
	stdout, _, err := runSource(t, client, "source", "create", "rss")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, `flag: --feed-url is required for type "rss"`, app.FormatError(err))
}

func TestSourceCreate_DisallowedFlag_EC_I(t *testing.T) {
	client := newSourceMock(t)
	stdout, _, err := runSource(t, client, "source", "create", "rss", "--tg-id=42", "--feed-url=https://x")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, `flag: --tg-id is not allowed for type "rss"`, app.FormatError(err))
}

// ── source update (SR-04, EC-F, EC-G, EC-I, AC-S3, AC-S5) ───────────────

func TestSourceUpdate_RSS(t *testing.T) {
	client := newSourceMock(t)
	var captured *feediumapi.V1UpdateSourceRequest
	client.EXPECT().
		V1UpdateSource(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1UpdateSourceRequest, _ ...grpc.CallOption) (*feediumapi.V1UpdateSourceResponse, error) {
			captured = req
			return &feediumapi.V1UpdateSourceResponse{Source: &feediumapi.Source{Id: req.GetId(), Type: req.GetType()}}, nil
		})

	_, _, err := runSource(t, client, "source", "update", "uuid-upd", "--type=rss", "--feed-url=https://upd.com")
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "uuid-upd", captured.GetId())
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_RSS, captured.GetType())
	r, ok := captured.GetConfig().GetConfig().(*feediumapi.SourceConfig_Rss)
	require.True(t, ok)
	assert.Equal(t, "https://upd.com", r.Rss.GetFeedUrl())
}

func TestSourceUpdate_TelegramChannel(t *testing.T) {
	client := newSourceMock(t)
	var captured *feediumapi.V1UpdateSourceRequest
	client.EXPECT().
		V1UpdateSource(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1UpdateSourceRequest, _ ...grpc.CallOption) (*feediumapi.V1UpdateSourceResponse, error) {
			captured = req
			return &feediumapi.V1UpdateSourceResponse{Source: &feediumapi.Source{Id: req.GetId()}}, nil
		})

	_, _, err := runSource(t, client, "source", "update", "uid", "--type=telegram-channel", "--tg-id=5", "--username=u")
	require.NoError(t, err)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL, captured.GetType())
}

func TestSourceUpdate_UnknownType_EC_G(t *testing.T) {
	client := newSourceMock(t)
	stdout, _, err := runSource(t, client, "source", "update", "uid", "--type=ftp", "--feed-url=x")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, `flag: unknown --type "ftp"`, app.FormatError(err))
}

func TestSourceUpdate_DisallowedFlag_EC_I(t *testing.T) {
	client := newSourceMock(t)
	// rss does not allow --username
	stdout, _, err := runSource(t, client, "source", "update", "uid", "--type=rss", "--username=foo", "--feed-url=https://x")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, `flag: --username is not allowed for type "rss"`, app.FormatError(err))
}

func TestSourceUpdate_MissingTypeFlag_FormattedAsFlag(t *testing.T) {
	// Cobra-generated "required flag(s) ... not set" must be remapped to
	// the "flag:" prefix by FormatError (NFR-03).
	client := newSourceMock(t)
	stdout, _, err := runSource(t, client, "source", "update", "uid")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.True(t, strings.HasPrefix(app.FormatError(err), "flag: "),
		"cobra error must be remapped to flag: prefix, got %q", app.FormatError(err))
}

// ── source delete (SR-05, EC-F, AC-S4, AC-S3) ────────────────────────────

const deleteSnapshotID = "00000000-0000-4000-8000-000000000001"

func deleteOKClient(t *testing.T) *mock.MockSourceServiceClient {
	t.Helper()
	client := newSourceMock(t)
	client.EXPECT().
		V1DeleteSource(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1DeleteSourceResponse{}, nil)
	return client
}

// readFixture loads a testdata file relative to the test source directory.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", "source_delete", name)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "testdata fixture %s not found", name)
	return string(data)
}

func TestSourceDelete_Snapshot_Table_AC_S4(t *testing.T) {
	stdout, _, err := runSource(t, deleteOKClient(t), "source", "delete", deleteSnapshotID)
	require.NoError(t, err)
	assert.Equal(t, readFixture(t, "table.txt"), stdout)
}

func TestSourceDelete_Snapshot_JSON_AC_S4(t *testing.T) {
	stdout, _, err := runSource(t, deleteOKClient(t), "source", "delete", deleteSnapshotID, "--output=json")
	require.NoError(t, err)
	assert.Equal(t, readFixture(t, "json.txt"), stdout)
}

func TestSourceDelete_Snapshot_YAML_AC_S4(t *testing.T) {
	stdout, _, err := runSource(t, deleteOKClient(t), "source", "delete", deleteSnapshotID, "--output=yaml")
	require.NoError(t, err)
	assert.Equal(t, readFixture(t, "yaml.txt"), stdout)
	// SR-10: UUID must not be quoted in YAML
	assert.NotContains(t, stdout, `"`+deleteSnapshotID+`"`)
}

func TestSourceDelete_Request(t *testing.T) {
	client := newSourceMock(t)
	var captured *feediumapi.V1DeleteSourceRequest
	client.EXPECT().
		V1DeleteSource(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1DeleteSourceRequest, _ ...grpc.CallOption) (*feediumapi.V1DeleteSourceResponse, error) {
			captured = req
			return &feediumapi.V1DeleteSourceResponse{}, nil
		})

	_, _, err := runSource(t, client, "source", "delete", "target-id")
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "target-id", captured.GetId())
}

func TestSourceDelete_NotFound_EC_F(t *testing.T) {
	client := newSourceMock(t)
	client.EXPECT().
		V1DeleteSource(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.NotFound, "no such source"))

	stdout, _, err := runSource(t, client, "source", "delete", "ghost-id")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "code=NotFound message=no such source", app.FormatError(err))
}

// ── EC-B: DeadlineExceeded (one representative command) ──────────────────

func TestSourceGet_DeadlineExceeded_EC_B(t *testing.T) {
	client := newSourceMock(t)
	client.EXPECT().
		V1GetSource(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.DeadlineExceeded, "context deadline exceeded"))

	stdout, _, err := runSource(t, client, "source", "get", "some-id")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "code=DeadlineExceeded message=context deadline exceeded", app.FormatError(err))
}

// ── INV-02: stdout empty on error ────────────────────────────────────────

func TestSource_StdoutEmptyOnError_INV02(t *testing.T) {
	errCases := []struct {
		name  string
		args  []string
		setup func(*mock.MockSourceServiceClient)
	}{
		{"unknown-type-create", []string{"source", "create", "ftp"}, nil},
		{"missing-flag-rss", []string{"source", "create", "rss"}, nil},
		{"bad-list-type", []string{"source", "list", "--type=BAD"}, nil},
		{"notfound-get", []string{"source", "get", "nope"}, func(c *mock.MockSourceServiceClient) {
			c.EXPECT().
				V1GetSource(gomock.Any(), gomock.Any()).
				Return(nil, status.Error(codes.NotFound, "nope"))
		}},
	}

	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			client := newSourceMock(t)
			if tc.setup != nil {
				tc.setup(client)
			}
			stdout, _, err := runSource(t, client, tc.args...)
			require.Error(t, err)
			assert.Empty(t, stdout, "stdout must be empty on error (INV-02)")
		})
	}
}

// ── EC-H: nil config in list (AC-S5) ──────────────────────────────────────

func TestSourceList_NilConfig_Table_EC_H(t *testing.T) {
	client := newSourceMock(t)
	client.EXPECT().
		V1ListSources(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1ListSourcesResponse{
			Items: []*feediumapi.Source{
				{Id: "id-h", Config: nil},
			},
		}, nil)

	stdout, _, err := runSource(t, client, "source", "list")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	require.Len(t, lines, 2)
	parts := strings.Split(lines[1], " | ")
	require.Len(t, parts, 5)
	assert.Empty(t, parts[3], "CONFIG column must be empty when config is nil")
}

// ── Render output for create/update in all three formats (AC-S3) ──────────

func TestSourceCreate_JSON_Output(t *testing.T) {
	client := newSourceMock(t)
	client.EXPECT().
		V1CreateSource(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1CreateSourceResponse{
			Source: &feediumapi.Source{Id: "json-id", Type: feediumapi.SourceType_SOURCE_TYPE_RSS},
		}, nil)

	stdout, _, err := runSource(t, client, "source", "create", "rss", "--feed-url=https://x", "--output=json")
	require.NoError(t, err)
	assert.Contains(t, stdout, `"id"`)
	assert.Contains(t, stdout, "json-id")
	assert.True(t, strings.HasSuffix(stdout, "\n"))
}

func TestSourceCreate_YAML_Output(t *testing.T) {
	client := newSourceMock(t)
	client.EXPECT().
		V1CreateSource(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1CreateSourceResponse{
			Source: &feediumapi.Source{Id: "yaml-id", Type: feediumapi.SourceType_SOURCE_TYPE_HTML},
		}, nil)

	stdout, _, err := runSource(t, client, "source", "create", "html", "--url=https://x", "--output=yaml")
	require.NoError(t, err)
	assert.Contains(t, stdout, "id:")
	assert.Contains(t, stdout, "yaml-id")
}

// ── Cobra-emitted errors must carry the NFR-03 "flag:" prefix ─────────────

func TestSource_CobraErrors_RemappedToFlagPrefix(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"missing-positional-get", []string{"source", "get"}},
		{"missing-positional-delete", []string{"source", "delete"}},
		{"unknown-flag", []string{"source", "--nope"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newSourceMock(t)
			stdout, _, err := runSource(t, client, tc.args...)
			require.Error(t, err)
			assert.Empty(t, stdout, "stdout must be empty on error (INV-02)")
			assert.True(t, strings.HasPrefix(app.FormatError(err), "flag: "),
				"cobra error must be remapped to flag: prefix, got %q", app.FormatError(err))
		})
	}
}
