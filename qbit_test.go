package qbit_test

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golift.io/qbit"
)

func newServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return server
}

func TestNewLogin(t *testing.T) {
	t.Parallel()

	server := newServer(t, func(writer http.ResponseWriter, req *http.Request) {
		assert.Equal(t, http.MethodPost, req.Method)
		assert.Equal(t, "/api/v2/auth/login", req.URL.Path)
		require.NoError(t, req.ParseForm())
		assert.Equal(t, "admin", req.Form.Get("username"))
		assert.Equal(t, "secret", req.Form.Get("password"))
		assert.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("httpuser:httppass")), req.Header.Get("Authorization"))

		http.SetCookie(writer, &http.Cookie{Name: "SID", Value: "cookie"})
		_, _ = writer.Write([]byte("Ok."))
	})

	client, err := qbit.New(context.Background(), &qbit.Config{
		URL:      server.URL + "/",
		User:     "admin",
		Pass:     "secret",
		HTTPUser: "httpuser",
		HTTPPass: "httppass",
	})
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestNewLoginFailed(t *testing.T) {
	t.Parallel()

	server := newServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte("Fails."))
	})

	client, err := qbit.New(context.Background(), &qbit.Config{
		URL:  server.URL,
		User: "admin",
		Pass: "wrong",
	})
	require.ErrorIs(t, err, qbit.ErrLoginFailed)
	assert.Nil(t, client)
}

func TestGetXfers(t *testing.T) {
	t.Parallel()

	server := newServer(t, func(writer http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/v2/auth/login":
			_, _ = writer.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			assert.Equal(t, http.MethodGet, req.Method)
			assert.Empty(t, req.URL.RawQuery)
			_, _ = writer.Write([]byte(`[{"name":"ubuntu.iso","progress":1,"hash":"abc","root_path":"/downloads/ubuntu"}]`))
		default:
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
	})

	client, err := qbit.New(context.Background(), &qbit.Config{URL: server.URL, User: "admin", Pass: "secret"})
	require.NoError(t, err)

	xfers, err := client.GetXfers()
	require.NoError(t, err)
	require.Len(t, xfers, 1)
	assert.Equal(t, "ubuntu.iso", xfers[0].Name)
	assert.Equal(t, 1.0, xfers[0].Progress)
	assert.Equal(t, "abc", xfers[0].Hash)
	assert.Equal(t, "/downloads/ubuntu", xfers[0].RootPath)
}

func TestGetXfersRelogin(t *testing.T) {
	t.Parallel()

	loggedIn := false
	server := newServer(t, func(writer http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/v2/auth/login":
			loggedIn = true
			_, _ = writer.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if !loggedIn {
				writer.WriteHeader(http.StatusForbidden)
				_, _ = writer.Write([]byte("Forbidden"))

				return
			}

			_, _ = writer.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
	})

	client, err := qbit.NewNoAuth(&qbit.Config{URL: server.URL, User: "admin", Pass: "secret"})
	require.NoError(t, err)

	xfers, err := client.GetXfersContext(context.Background())
	require.NoError(t, err)
	assert.Empty(t, xfers)
	assert.True(t, loggedIn)
}

func TestGetCategories(t *testing.T) {
	t.Parallel()

	server := newServer(t, func(writer http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/v2/auth/login":
			_, _ = writer.Write([]byte("Ok."))
		case "/api/v2/torrents/categories":
			assert.Equal(t, http.MethodGet, req.Method)
			_, _ = writer.Write([]byte(`{"tv":{"name":"tv","savePath":"/data/tv"}}`))
		default:
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
	})

	client, err := qbit.New(context.Background(), &qbit.Config{URL: server.URL})
	require.NoError(t, err)

	cats, err := client.GetCategories()
	require.NoError(t, err)
	require.Contains(t, cats, "tv")
	assert.Equal(t, "tv", cats["tv"].Name)
	assert.Equal(t, "/data/tv", cats["tv"].SavePath)
}

func TestSetTorrentCategory(t *testing.T) {
	t.Parallel()

	server := newServer(t, func(writer http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/v2/auth/login":
			_, _ = writer.Write([]byte("Ok."))
		case "/api/v2/torrents/setCategory":
			assert.Equal(t, http.MethodPost, req.Method)
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), "category=movies")
			assert.Contains(t, string(body), "hashes=aaa%7Cbbb")
			// qBittorrent returns an empty body on success.
		default:
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
	})

	client, err := qbit.New(context.Background(), &qbit.Config{URL: server.URL})
	require.NoError(t, err)
	require.NoError(t, client.SetTorrentCategory("movies", "aaa", "bbb"))
}
