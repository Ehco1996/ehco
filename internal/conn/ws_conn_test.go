package conn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gobwas/ws"
	"github.com/stretchr/testify/assert"
)

func TestClientConn_ReadWrite(t *testing.T) {
	data := []byte("hello")

	// Create a WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go func() {
			defer conn.Close()
			wsc := NewWSConn(conn, true)

			buf := make([]byte, 1024)
			for {
				n, err := wsc.Read(buf)
				if err != nil {
					return
				}
				assert.Equal(t, len(data), n)
				assert.Equal(t, "hello", string(buf[:n]))
				_, err = wsc.Write(buf[:n])
				if err != nil {
					return
				}
			}
		}()
	}))
	defer server.Close()

	// Create a WebSocket client
	addr, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	conn, _, _, err := ws.DefaultDialer.Dial(context.TODO(), "ws://"+addr.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	wsClientConn := NewWSConn(conn, false)
	for i := 0; i < 3; i++ {
		// test write
		n, err := wsClientConn.Write(data)
		assert.NoError(t, err, "test cnt %d", i)
		assert.Equal(t, len(data), n, "test cnt %d", i)

		// test read
		buf := make([]byte, 100)
		n, err = wsClientConn.Read(buf)
		assert.NoError(t, err, "test cnt %d", i)
		assert.Equal(t, len(data), n, "test cnt %d", i)
		assert.Equal(t, "hello", string(buf[:n]), "test cnt %d", i)
	}
}
