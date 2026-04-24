package render_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/render"
)

func TestWriteHealth_Table(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, render.Write(&buf, render.FormatTable, &feediumapi.V1CheckResponse{Status: "SERVING"}))
	got := buf.String()
	assert.Equal(t, "FIELD | VALUE\nstatus | SERVING\n", got)
	assert.Equal(t, 1, strings.Count(got, "FIELD"))
	assert.Equal(t, 1, strings.Count(got, "VALUE"))
}

func TestWriteHealth_JSON_DeterministicAndSnakeCase(t *testing.T) {
	resp := &feediumapi.V1CheckResponse{Status: "SERVING"}
	var a, b bytes.Buffer
	require.NoError(t, render.Write(&a, render.FormatJSON, resp))
	require.NoError(t, render.Write(&b, render.FormatJSON, resp))
	assert.Equal(t, a.Bytes(), b.Bytes(), "same input must produce byte-identical output (INV-06)")

	got := a.String()
	assert.True(t, strings.HasSuffix(got, "\n"), "must end with exactly one newline")
	assert.False(t, strings.HasSuffix(got, "\n\n"), "must not end with a double newline")
	assert.Contains(t, got, `"status"`)
	assert.Contains(t, got, `"SERVING"`)
}

func TestWriteHealth_JSON_OmitsUnpopulated(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, render.Write(&buf, render.FormatJSON, &feediumapi.V1CheckResponse{}))
	// EmitUnpopulated=false ⇒ empty "status" must be absent.
	assert.NotContains(t, buf.String(), "status")
}

func TestWriteHealth_YAML_Deterministic(t *testing.T) {
	resp := &feediumapi.V1CheckResponse{Status: "SERVING"}
	var a, b bytes.Buffer
	require.NoError(t, render.Write(&a, render.FormatYAML, resp))
	require.NoError(t, render.Write(&b, render.FormatYAML, resp))
	assert.Equal(t, a.Bytes(), b.Bytes())

	got := a.String()
	assert.True(t, strings.HasSuffix(got, "\n"))
	assert.False(t, strings.HasSuffix(got, "\n\n"))
	assert.Contains(t, got, "status: SERVING")
}

func TestWriteHealth_TableDeterministic(t *testing.T) {
	resp := &feediumapi.V1CheckResponse{Status: "SERVING"}
	var a, b bytes.Buffer
	require.NoError(t, render.Write(&a, render.FormatTable, resp))
	require.NoError(t, render.Write(&b, render.FormatTable, resp))
	assert.Equal(t, a.Bytes(), b.Bytes())
}
