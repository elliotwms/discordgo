package discordgo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// listeningSession connects a session to url and starts listening, as Open does once the connection is established
func listeningSession(t *testing.T, url string) *Session {
	t.Helper()

	s, err := New("")
	if err != nil {
		t.Fatal(err)
	}

	s.wsConn, _, err = websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.wsConn.SetCloseHandler(func(code int, text string) error {
		return nil
	})

	s.listening = make(chan interface{})
	s.wsReadDone = make(chan struct{})
	go s.listen(s.wsConn, s.listening, s.wsReadDone)

	return s
}

// wsServer starts a websocket server which runs handle for each connection. The returned function stops the server
// (t.Cleanup is not used as it requires Go 1.14)
func wsServer(t *testing.T, handle func(conn *websocket.Conn)) (url string, stop func()) {
	t.Helper()

	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		handle(conn)
	}))

	return "ws" + strings.TrimPrefix(srv.URL, "http"), srv.Close
}

func TestCloseWaitsForServerToClose(t *testing.T) {
	// the server reads until the client's close frame arrives. gorilla/websocket's default close handler then replies
	// with a close frame, and the connection is closed
	url, stop := wsServer(t, func(conn *websocket.Conn) {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer stop()

	s := listeningSession(t, url)

	start := time.Now()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("Close took %s, expected it to return once the server closed the connection", elapsed)
	}
}

func TestCloseTimesOutIfServerDoesNotClose(t *testing.T) {
	// the server never reads, so never replies to the client's close frame
	release := make(chan struct{})

	url, stop := wsServer(t, func(conn *websocket.Conn) {
		<-release
	})
	defer stop()
	// deferred after stop so that it runs first, as stop waits for the handler to return
	defer close(release)

	s := listeningSession(t, url)

	start := time.Now()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	elapsed := time.Since(start)
	if elapsed < 900*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("Close took %s, expected it to give up after a second", elapsed)
	}
}
