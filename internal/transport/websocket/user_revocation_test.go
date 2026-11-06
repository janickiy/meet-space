package websocket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
)

func TestDisconnectUserRevokesAllTargetStreamsOnly(t *testing.T) {
	target1, cancel1 := context.WithCancelCause(context.Background())
	target2, cancel2 := context.WithCancelCause(context.Background())
	other, cancelOther := context.WithCancelCause(context.Background())
	defer cancel1(nil)
	defer cancel2(nil)
	defer cancelOther(nil)
	c1, c2, c3 := &ws.Conn{}, &ws.Conn{}, &ws.Conn{}
	h := &UserHandler{clients: map[*ws.Conn]context.CancelCauseFunc{c1: cancel1, c2: cancel2, c3: cancelOther}, clientUsers: map[*ws.Conn]string{c1: "blocked", c2: "blocked", c3: "other"}}
	h.DisconnectUser("blocked")
	for _, ctx := range []context.Context{target1, target2} {
		if !errors.Is(context.Cause(ctx), errUserAuthenticationRevoked) {
			t.Fatal("blocked stream did not receive authentication revocation")
		}
	}
	if other.Err() != nil {
		t.Fatal("another user's stream was cancelled")
	}
	h.DisconnectUser("missing")
	if other.Err() != nil {
		t.Fatal("unknown user affected another stream")
	}
}

func TestAuthenticationRevocationHasExplicitWebSocketClose(t *testing.T) {
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		conn, err := (&ws.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		closeUserAuthentication(conn, time.Second)
	}))
	defer server.Close()
	conn, _, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	var closed *ws.CloseError
	if !errors.As(err, &closed) || closed.Code != 4401 || closed.Text != "authentication_revoked" {
		t.Fatalf("expected explicit revocation close, got %v", err)
	}
	<-finished
}
