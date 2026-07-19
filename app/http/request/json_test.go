package request

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBindJSONBodySupportsTopLevelArray(t *testing.T) {
	request, err := http.NewRequest(http.MethodDelete, "/", strings.NewReader(`[1,2]`))
	require.NoError(t, err)
	var ids []uint64

	require.NoError(t, BindJSONBody(request, &ids))
	require.Equal(t, []uint64{1, 2}, ids)
}

func TestBindJSONBodySupportsObjectAndEmptyBody(t *testing.T) {
	request, err := http.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"Goravel"}`))
	require.NoError(t, err)
	var payload struct {
		Name string `json:"name"`
	}

	require.NoError(t, BindJSONBody(request, &payload))
	require.Equal(t, "Goravel", payload.Name)

	empty, err := http.NewRequest(http.MethodPost, "/", nil)
	require.NoError(t, err)
	require.NoError(t, BindJSONBody(empty, &payload))
}

func TestBindJSONBodyPreservesBodyAndReturnsDecodeError(t *testing.T) {
	request, err := http.NewRequest(http.MethodPost, "/", strings.NewReader(`{"invalid"`))
	require.NoError(t, err)

	require.Error(t, BindJSONBody(request, &struct{}{}))
	body, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	require.Equal(t, `{"invalid"`, string(body))
}
