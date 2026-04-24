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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/app"
)

// ── stub SourceServiceClient ──────────────────────────────────────────────

type stubSource struct {
	t        *testing.T
	listFn   func(*feediumapi.V1ListSourcesRequest) (*feediumapi.V1ListSourcesResponse, error)
	getFn    func(*feediumapi.V1GetSourceRequest) (*feediumapi.V1GetSourceResponse, error)
	createFn func(*feediumapi.V1CreateSourceRequest) (*feediumapi.V1CreateSourceResponse, error)
	updateFn func(*feediumapi.V1UpdateSourceRequest) (*feediumapi.V1UpdateSourceResponse, error)
	deleteFn func(*feediumapi.V1DeleteSourceRequest) (*feediumapi.V1DeleteSourceResponse, error)
}

func (s *stubSource) V1ListSources(_ context.Context, in *feediumapi.V1ListSourcesRequest, _ ...grpc.CallOption) (*feediumapi.V1ListSourcesResponse, error) {
	if s.listFn == nil {
		s.t.Fatal("V1ListSources called unexpectedly")
	}
	return s.listFn(in)
}
func (s *stubSource) V1GetSource(_ context.Context, in *feediumapi.V1GetSourceRequest, _ ...grpc.CallOption) (*feediumapi.V1GetSourceResponse, error) {
	if s.getFn == nil {
		s.t.Fatal("V1GetSource called unexpectedly")
	}
	return s.getFn(in)
}
func (s *stubSource) V1CreateSource(_ context.Context, in *feediumapi.V1CreateSourceRequest, _ ...grpc.CallOption) (*feediumapi.V1CreateSourceResponse, error) {
	if s.createFn == nil {
		s.t.Fatal("V1CreateSource called unexpectedly")
	}
	return s.createFn(in)
}
func (s *stubSource) V1UpdateSource(_ context.Context, in *feediumapi.V1UpdateSourceRequest, _ ...grpc.CallOption) (*feediumapi.V1UpdateSourceResponse, error) {
	if s.updateFn == nil {
		s.t.Fatal("V1UpdateSource called unexpectedly")
	}
	return s.updateFn(in)
}
func (s *stubSource) V1DeleteSource(_ context.Context, in *feediumapi.V1DeleteSourceRequest, _ ...grpc.CallOption) (*feediumapi.V1DeleteSourceResponse, error) {
	if s.deleteFn == nil {
		s.t.Fatal("V1DeleteSource called unexpectedly")
	}
	return s.deleteFn(in)
}

// ── helpers ───────────────────────────────────────────────────────────────

func runSource(t *testing.T, stub feediumapi.SourceServiceClient, args ...string) (stdout string, stderr string, err error) {
	t.Helper()
	cmd := app.NewRootCommandWithSource(app.StubSourceFactory(stub))
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.ExecuteContext(context.Background())
	return outBuf.String(), errBuf.String(), err
}

// ── source --help (AC-S6, INV-03) ────────────────────────────────────────

func TestSourceHelp(t *testing.T) {
	stub := &stubSource{t: t} // no RPC should fire
	stdout, _, err := runSource(t, stub, "source", "--help")
	require.NoError(t, err)
	for _, sub := range []string{"list", "get", "create", "update", "delete"} {
		assert.Contains(t, stdout, sub, "help must mention subcommand %q", sub)
	}
	assert.Empty(t, stdout[:0], "") // just ensure no panic; content checked above
}

// ── source list (SR-01, EC-C, EC-G, AC-S3) ───────────────────────────────

func TestSourceList_Request_PageSize(t *testing.T) {
	var captured *feediumapi.V1ListSourcesRequest
	stub := &stubSource{t: t, listFn: func(req *feediumapi.V1ListSourcesRequest) (*feediumapi.V1ListSourcesResponse, error) {
		captured = req
		return &feediumapi.V1ListSourcesResponse{}, nil
	}}
	stdout, _, err := runSource(t, stub, "source", "list", "--page-size=25")
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, int32(25), captured.GetPageSize())
	assert.Equal(t, "", captured.GetPageToken(), "page_token must be empty (SR-01)")
	assert.Nil(t, captured.Type, "--type not set → Type must be nil")
	assert.Contains(t, stdout, "ID | TYPE")
}

func TestSourceList_TypeFilter(t *testing.T) {
	var captured *feediumapi.V1ListSourcesRequest
	stub := &stubSource{t: t, listFn: func(req *feediumapi.V1ListSourcesRequest) (*feediumapi.V1ListSourcesResponse, error) {
		captured = req
		return &feediumapi.V1ListSourcesResponse{}, nil
	}}
	_, _, err := runSource(t, stub, "source", "list", "--type=RSS")
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.NotNil(t, captured.Type)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_RSS, *captured.Type)
}

func TestSourceList_EmptyItems_Table_EC_C(t *testing.T) {
	stub := &stubSource{t: t, listFn: func(*feediumapi.V1ListSourcesRequest) (*feediumapi.V1ListSourcesResponse, error) {
		return &feediumapi.V1ListSourcesResponse{}, nil
	}}
	stdout, _, err := runSource(t, stub, "source", "list")
	require.NoError(t, err)
	assert.Equal(t, "ID | TYPE | MODE | CONFIG | CREATED_AT\n", stdout)
}

func TestSourceList_EmptyItems_JSON_EC_C(t *testing.T) {
	stub := &stubSource{t: t, listFn: func(*feediumapi.V1ListSourcesRequest) (*feediumapi.V1ListSourcesResponse, error) {
		return &feediumapi.V1ListSourcesResponse{}, nil
	}}
	stdout, _, err := runSource(t, stub, "source", "list", "--output=json")
	require.NoError(t, err)
	assert.Equal(t, "[]\n", stdout)
}

func TestSourceList_EmptyItems_YAML_EC_C(t *testing.T) {
	stub := &stubSource{t: t, listFn: func(*feediumapi.V1ListSourcesRequest) (*feediumapi.V1ListSourcesResponse, error) {
		return &feediumapi.V1ListSourcesResponse{}, nil
	}}
	stdout, _, err := runSource(t, stub, "source", "list", "--output=yaml")
	require.NoError(t, err)
	assert.Equal(t, "[]\n", stdout)
}

func TestSourceList_UnknownType_EC_G(t *testing.T) {
	stub := &stubSource{t: t} // listFn nil → panics if called
	stdout, _, err := runSource(t, stub, "source", "list", "--type=unknown_type")
	require.Error(t, err)
	assert.Empty(t, stdout, "stdout must be empty on error (INV-02)")
	assert.Equal(t, `flag: unknown --type "unknown_type"`, err.Error())
}

// ── source get (SR-02, EC-A, AC-S3) ──────────────────────────────────────

func TestSourceGet_Request(t *testing.T) {
	var captured *feediumapi.V1GetSourceRequest
	stub := &stubSource{t: t, getFn: func(req *feediumapi.V1GetSourceRequest) (*feediumapi.V1GetSourceResponse, error) {
		captured = req
		return &feediumapi.V1GetSourceResponse{
			Source: &feediumapi.Source{Id: req.GetId(), Type: feediumapi.SourceType_SOURCE_TYPE_HTML},
		}, nil
	}}
	stdout, _, err := runSource(t, stub, "source", "get", "my-uuid-123")
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "my-uuid-123", captured.GetId())
	assert.Contains(t, stdout, "my-uuid-123")
	assert.Contains(t, stdout, "HTML")
}

func TestSourceGet_NotFound_EC_A(t *testing.T) {
	stub := &stubSource{t: t, getFn: func(*feediumapi.V1GetSourceRequest) (*feediumapi.V1GetSourceResponse, error) {
		return nil, status.Error(codes.NotFound, "source not found")
	}}
	stdout, _, err := runSource(t, stub, "source", "get", "no-such-id")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "code=NotFound message=source not found", err.Error())
}

// ── source create (SR-03, EC-D, EC-E, EC-I, AC-S3, AC-S5) ───────────────

func TestSourceCreate_TelegramChannel(t *testing.T) {
	var captured *feediumapi.V1CreateSourceRequest
	stub := &stubSource{t: t, createFn: func(req *feediumapi.V1CreateSourceRequest) (*feediumapi.V1CreateSourceResponse, error) {
		captured = req
		return &feediumapi.V1CreateSourceResponse{
			Source: &feediumapi.Source{Id: "new-id", Type: req.GetType()},
		}, nil
	}}
	_, _, err := runSource(t, stub, "source", "create", "telegram-channel", "--tg-id=99", "--username=chan")
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL, captured.GetType())
	tc, ok := captured.GetConfig().GetConfig().(*feediumapi.SourceConfig_TelegramChannel)
	require.True(t, ok)
	assert.Equal(t, int64(99), tc.TelegramChannel.GetTgId())
	assert.Equal(t, "chan", tc.TelegramChannel.GetUsername())
}

func TestSourceCreate_TelegramGroup(t *testing.T) {
	var captured *feediumapi.V1CreateSourceRequest
	stub := &stubSource{t: t, createFn: func(req *feediumapi.V1CreateSourceRequest) (*feediumapi.V1CreateSourceResponse, error) {
		captured = req
		return &feediumapi.V1CreateSourceResponse{Source: &feediumapi.Source{Id: "g"}}, nil
	}}
	_, _, err := runSource(t, stub, "source", "create", "telegram-group", "--tg-id=7", "--username=grp")
	require.NoError(t, err)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_GROUP, captured.GetType())
	tg, ok := captured.GetConfig().GetConfig().(*feediumapi.SourceConfig_TelegramGroup)
	require.True(t, ok)
	assert.Equal(t, int64(7), tg.TelegramGroup.GetTgId())
}

func TestSourceCreate_RSS(t *testing.T) {
	var captured *feediumapi.V1CreateSourceRequest
	stub := &stubSource{t: t, createFn: func(req *feediumapi.V1CreateSourceRequest) (*feediumapi.V1CreateSourceResponse, error) {
		captured = req
		return &feediumapi.V1CreateSourceResponse{Source: &feediumapi.Source{Id: "r"}}, nil
	}}
	_, _, err := runSource(t, stub, "source", "create", "rss", "--feed-url=https://x.com/feed")
	require.NoError(t, err)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_RSS, captured.GetType())
	r, ok := captured.GetConfig().GetConfig().(*feediumapi.SourceConfig_Rss)
	require.True(t, ok)
	assert.Equal(t, "https://x.com/feed", r.Rss.GetFeedUrl())
}

func TestSourceCreate_HTML(t *testing.T) {
	var captured *feediumapi.V1CreateSourceRequest
	stub := &stubSource{t: t, createFn: func(req *feediumapi.V1CreateSourceRequest) (*feediumapi.V1CreateSourceResponse, error) {
		captured = req
		return &feediumapi.V1CreateSourceResponse{Source: &feediumapi.Source{Id: "h"}}, nil
	}}
	_, _, err := runSource(t, stub, "source", "create", "html", "--url=https://x.com")
	require.NoError(t, err)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_HTML, captured.GetType())
	h, ok := captured.GetConfig().GetConfig().(*feediumapi.SourceConfig_Html)
	require.True(t, ok)
	assert.Equal(t, "https://x.com", h.Html.GetUrl())
}

func TestSourceCreate_UnknownType_EC_E(t *testing.T) {
	stub := &stubSource{t: t} // createFn nil → fatal if called
	stdout, _, err := runSource(t, stub, "source", "create", "ftp", "--feed-url=x")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, `flag: unknown source type "ftp" (allowed: html,rss,telegram-channel,telegram-group)`, err.Error())
}

func TestSourceCreate_MissingRequired_EC_D(t *testing.T) {
	stub := &stubSource{t: t}
	stdout, _, err := runSource(t, stub, "source", "create", "rss")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, `flag: --feed-url is required for type "rss"`, err.Error())
}

func TestSourceCreate_DisallowedFlag_EC_I(t *testing.T) {
	stub := &stubSource{t: t}
	stdout, _, err := runSource(t, stub, "source", "create", "rss", "--tg-id=42", "--feed-url=https://x")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, `flag: --tg-id is not allowed for type "rss"`, err.Error())
}

// ── source update (SR-04, EC-F, EC-G, EC-I, AC-S3, AC-S5) ───────────────

func TestSourceUpdate_RSS(t *testing.T) {
	var captured *feediumapi.V1UpdateSourceRequest
	stub := &stubSource{t: t, updateFn: func(req *feediumapi.V1UpdateSourceRequest) (*feediumapi.V1UpdateSourceResponse, error) {
		captured = req
		return &feediumapi.V1UpdateSourceResponse{Source: &feediumapi.Source{Id: req.GetId(), Type: req.GetType()}}, nil
	}}
	_, _, err := runSource(t, stub, "source", "update", "uuid-upd", "--type=rss", "--feed-url=https://upd.com")
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "uuid-upd", captured.GetId())
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_RSS, captured.GetType())
	r, ok := captured.GetConfig().GetConfig().(*feediumapi.SourceConfig_Rss)
	require.True(t, ok)
	assert.Equal(t, "https://upd.com", r.Rss.GetFeedUrl())
}

func TestSourceUpdate_TelegramChannel(t *testing.T) {
	var captured *feediumapi.V1UpdateSourceRequest
	stub := &stubSource{t: t, updateFn: func(req *feediumapi.V1UpdateSourceRequest) (*feediumapi.V1UpdateSourceResponse, error) {
		captured = req
		return &feediumapi.V1UpdateSourceResponse{Source: &feediumapi.Source{Id: req.GetId()}}, nil
	}}
	_, _, err := runSource(t, stub, "source", "update", "uid", "--type=telegram-channel", "--tg-id=5", "--username=u")
	require.NoError(t, err)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL, captured.GetType())
}

func TestSourceUpdate_UnknownType_EC_G(t *testing.T) {
	stub := &stubSource{t: t}
	stdout, _, err := runSource(t, stub, "source", "update", "uid", "--type=ftp", "--feed-url=x")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, `flag: unknown --type "ftp"`, err.Error())
}

func TestSourceUpdate_DisallowedFlag_EC_I(t *testing.T) {
	stub := &stubSource{t: t}
	// rss does not allow --username
	stdout, _, err := runSource(t, stub, "source", "update", "uid", "--type=rss", "--username=foo", "--feed-url=https://x")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, `flag: --username is not allowed for type "rss"`, err.Error())
}

// ── source delete (SR-05, EC-F, AC-S4, AC-S3) ────────────────────────────

const deleteSnapshotID = "00000000-0000-4000-8000-000000000001"

func deleteStub(t *testing.T) *stubSource {
	t.Helper()
	return &stubSource{t: t, deleteFn: func(req *feediumapi.V1DeleteSourceRequest) (*feediumapi.V1DeleteSourceResponse, error) {
		return &feediumapi.V1DeleteSourceResponse{}, nil
	}}
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
	stdout, _, err := runSource(t, deleteStub(t), "source", "delete", deleteSnapshotID)
	require.NoError(t, err)
	assert.Equal(t, readFixture(t, "table.txt"), stdout)
}

func TestSourceDelete_Snapshot_JSON_AC_S4(t *testing.T) {
	stdout, _, err := runSource(t, deleteStub(t), "source", "delete", deleteSnapshotID, "--output=json")
	require.NoError(t, err)
	assert.Equal(t, readFixture(t, "json.txt"), stdout)
}

func TestSourceDelete_Snapshot_YAML_AC_S4(t *testing.T) {
	stdout, _, err := runSource(t, deleteStub(t), "source", "delete", deleteSnapshotID, "--output=yaml")
	require.NoError(t, err)
	got := stdout
	assert.Equal(t, readFixture(t, "yaml.txt"), got)
	// SR-10: UUID must not be quoted in YAML
	assert.NotContains(t, got, `"`+deleteSnapshotID+`"`)
}

func TestSourceDelete_Request(t *testing.T) {
	var captured *feediumapi.V1DeleteSourceRequest
	stub := &stubSource{t: t, deleteFn: func(req *feediumapi.V1DeleteSourceRequest) (*feediumapi.V1DeleteSourceResponse, error) {
		captured = req
		return &feediumapi.V1DeleteSourceResponse{}, nil
	}}
	_, _, err := runSource(t, stub, "source", "delete", "target-id")
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "target-id", captured.GetId())
}

func TestSourceDelete_NotFound_EC_F(t *testing.T) {
	stub := &stubSource{t: t, deleteFn: func(*feediumapi.V1DeleteSourceRequest) (*feediumapi.V1DeleteSourceResponse, error) {
		return nil, status.Error(codes.NotFound, "no such source")
	}}
	stdout, _, err := runSource(t, stub, "source", "delete", "ghost-id")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "code=NotFound message=no such source", err.Error())
}

// ── EC-B: DeadlineExceeded (one representative command) ──────────────────

func TestSourceGet_DeadlineExceeded_EC_B(t *testing.T) {
	stub := &stubSource{t: t, getFn: func(*feediumapi.V1GetSourceRequest) (*feediumapi.V1GetSourceResponse, error) {
		return nil, status.Error(codes.DeadlineExceeded, "context deadline exceeded")
	}}
	stdout, _, err := runSource(t, stub, "source", "get", "some-id")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "code=DeadlineExceeded message=context deadline exceeded", err.Error())
}

// ── INV-02: stdout empty on error ────────────────────────────────────────

func TestSource_StdoutEmptyOnError_INV02(t *testing.T) {
	errCases := []struct {
		name string
		args []string
	}{
		{"unknown-type-create", []string{"source", "create", "ftp"}},
		{"missing-flag-rss", []string{"source", "create", "rss"}},
		{"bad-list-type", []string{"source", "list", "--type=BAD"}},
		{"notfound-get", []string{"source", "get", "nope"}},
	}

	notFoundStub := &stubSource{
		t: t,
		getFn: func(*feediumapi.V1GetSourceRequest) (*feediumapi.V1GetSourceResponse, error) {
			return nil, status.Error(codes.NotFound, "nope")
		},
	}

	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, err := runSource(t, notFoundStub, tc.args...)
			require.Error(t, err)
			assert.Empty(t, stdout, "stdout must be empty on error (INV-02)")
		})
	}
}

// ── EC-H: nil config in list (AC-S5) ──────────────────────────────────────

func TestSourceList_NilConfig_Table_EC_H(t *testing.T) {
	stub := &stubSource{t: t, listFn: func(*feediumapi.V1ListSourcesRequest) (*feediumapi.V1ListSourcesResponse, error) {
		return &feediumapi.V1ListSourcesResponse{
			Items: []*feediumapi.Source{
				{Id: "id-h", Config: nil}, // no config set
			},
		}, nil
	}}
	stdout, _, err := runSource(t, stub, "source", "list")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	require.Len(t, lines, 2)
	parts := strings.Split(lines[1], " | ")
	require.Len(t, parts, 5)
	assert.Empty(t, parts[3], "CONFIG column must be empty when config is nil")
}

// ── Render output for create/update in all three formats (AC-S3) ──────────

func TestSourceCreate_JSON_Output(t *testing.T) {
	stub := &stubSource{t: t, createFn: func(*feediumapi.V1CreateSourceRequest) (*feediumapi.V1CreateSourceResponse, error) {
		return &feediumapi.V1CreateSourceResponse{
			Source: &feediumapi.Source{Id: "json-id", Type: feediumapi.SourceType_SOURCE_TYPE_RSS},
		}, nil
	}}
	stdout, _, err := runSource(t, stub, "source", "create", "rss", "--feed-url=https://x", "--output=json")
	require.NoError(t, err)
	assert.Contains(t, stdout, `"id"`)
	assert.Contains(t, stdout, "json-id")
	assert.True(t, strings.HasSuffix(stdout, "\n"))
}

func TestSourceCreate_YAML_Output(t *testing.T) {
	stub := &stubSource{t: t, createFn: func(*feediumapi.V1CreateSourceRequest) (*feediumapi.V1CreateSourceResponse, error) {
		return &feediumapi.V1CreateSourceResponse{
			Source: &feediumapi.Source{Id: "yaml-id", Type: feediumapi.SourceType_SOURCE_TYPE_HTML},
		}, nil
	}}
	stdout, _, err := runSource(t, stub, "source", "create", "html", "--url=https://x", "--output=yaml")
	require.NoError(t, err)
	assert.Contains(t, stdout, "id:")
	assert.Contains(t, stdout, "yaml-id")
}
