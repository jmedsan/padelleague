package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeScoreReq(vals url.Values) (*core.RequestEvent, *httptest.ResponseRecorder) {
	r, _ := http.NewRequest("POST", "/", strings.NewReader(vals.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	return &core.RequestEvent{Request: r, Response: w}, w
}

func TestReadScoreForm(t *testing.T) {
	t.Parallel()

	t.Run("complete score without carried", func(t *testing.T) {
		e, w := makeScoreReq(url.Values{"scores": {"6-3 6-4"}})
		full, err := readScoreForm(e, "", "scores")
		require.NoError(t, err)
		assert.Equal(t, "6-3 6-4", full)
		assert.Equal(t, 0, w.Body.Len())
	})

	t.Run("carried prepend produces full score", func(t *testing.T) {
		e, _ := makeScoreReq(url.Values{"scores": {"6-4"}, "unfinished": {"on"}})
		full, err := readScoreForm(e, "6-3", "scores")
		require.NoError(t, err)
		assert.Equal(t, "6-3 6-4", full)
	})

	t.Run("unfinished checkbox with partial score", func(t *testing.T) {
		e, _ := makeScoreReq(url.Values{"scores": {"2-1"}, "unfinished": {"on"}})
		full, err := readScoreForm(e, "6-3", "scores")
		require.NoError(t, err)
		assert.Equal(t, "6-3 2-1", full)
	})

	t.Run("partial score rejected without checkbox", func(t *testing.T) {
		e, w := makeScoreReq(url.Values{"scores": {"2-1"}})
		readScoreForm(e, "6-3", "scores")
		assert.Contains(t, w.Body.String(), "Marcador no válido")
	})

	t.Run("empty score rejected", func(t *testing.T) {
		e, w := makeScoreReq(url.Values{"scores": {""}})
		readScoreForm(e, "", "scores")
		assert.Contains(t, w.Body.String(), "Debes indicar el marcador")
	})

	t.Run("WO rejected", func(t *testing.T) {
		e, w := makeScoreReq(url.Values{"scores": {"WO"}})
		readScoreForm(e, "", "scores")
		assert.Contains(t, w.Body.String(), "partido no jugado")
	})

	t.Run("admin partial no-winner rejected without checkbox", func(t *testing.T) {
		e, w := makeScoreReq(url.Values{"scores": {"6-3 2-1"}})
		readScoreForm(e, "", "scores")
		assert.Contains(t, w.Body.String(), "Marcador no válido")
	})

	t.Run("checkbox with complete score is normal win", func(t *testing.T) {
		e, _ := makeScoreReq(url.Values{"scores": {"6-4"}, "unfinished": {"on"}})
		full, err := readScoreForm(e, "6-3", "scores")
		require.NoError(t, err)
		assert.Equal(t, "6-3 6-4", full)
	})

	t.Run("carried accumulation", func(t *testing.T) {
		e, _ := makeScoreReq(url.Values{"scores": {"3-6 2-1"}, "unfinished": {"on"}})
		full, err := readScoreForm(e, "6-3", "scores")
		require.NoError(t, err)
		assert.Equal(t, "6-3 3-6 2-1", full)
	})

	t.Run("counter_scores field name", func(t *testing.T) {
		e, _ := makeScoreReq(url.Values{"counter_scores": {"6-4"}, "unfinished": {"on"}})
		full, err := readScoreForm(e, "6-3", "counter_scores")
		require.NoError(t, err)
		assert.Equal(t, "6-3 6-4", full)
	})

	t.Run("rule win via carried", func(t *testing.T) {
		e, _ := makeScoreReq(url.Values{"scores": {"4-1"}, "unfinished": {"on"}})
		full, err := readScoreForm(e, "6-3", "scores")
		require.NoError(t, err)
		assert.Equal(t, "6-3 4-1", full)
	})
}
