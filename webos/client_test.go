package webos

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeTV is a minimal SSAP server: it completes the handshake and then
// answers requests from a table of canned replies.
type fakeTV struct {
	t         *testing.T
	key       string // key the TV hands out; "" keeps the client's
	prompt    bool   // emit the on-screen-prompt acknowledgement first
	replies   map[string]string
	seenURIs  []string
	clientKey string // key the client presented
}

func (f *fakeTV) start() *httptest.Server {
	f.t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			f.t.Errorf("accept: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		f.serve(r.Context(), conn)
	}))
	f.t.Cleanup(srv.Close)
	return srv
}

func (f *fakeTV) serve(ctx context.Context, conn *websocket.Conn) {
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var msg envelope
		if err := json.Unmarshal(raw, &msg); err != nil {
			f.t.Errorf("client sent invalid JSON: %v", err)
			return
		}

		switch msg.Type {
		case "register":
			var payload struct {
				ClientKey   string `json:"client-key"`
				PairingType string `json:"pairingType"`
			}
			_ = json.Unmarshal(msg.Payload, &payload)
			f.clientKey = payload.ClientKey
			if f.prompt {
				f.send(ctx, conn, envelope{
					ID: msg.ID, Type: "response",
					Payload: json.RawMessage(`{"pairingType":"PROMPT","returnValue":true}`),
				})
			}
			key := f.key
			if key == "" {
				key = payload.ClientKey
			}
			body, _ := json.Marshal(map[string]string{"client-key": key})
			f.send(ctx, conn, envelope{ID: msg.ID, Type: "registered", Payload: body})

		case "request":
			uri := strings.TrimPrefix(msg.URI, "ssap://")
			f.seenURIs = append(f.seenURIs, uri)
			reply, ok := f.replies[uri]
			if !ok {
				f.send(ctx, conn, envelope{ID: msg.ID, Type: "error", Error: "404 no such service or method"})
				continue
			}
			f.send(ctx, conn, envelope{ID: msg.ID, Type: "response", Payload: json.RawMessage(reply)})
		}
	}
}

func (f *fakeTV) send(ctx context.Context, conn *websocket.Conn, msg envelope) {
	f.t.Helper()
	raw, err := json.Marshal(msg)
	if err != nil {
		f.t.Errorf("encode reply: %v", err)
		return
	}
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		f.t.Errorf("write reply: %v", err)
	}
}

// dialFake points the client at a fake TV by handing Dial an explicit port.
func dialFake(t *testing.T, tv *fakeTV, opt Options) *Client {
	t.Helper()
	srv := tv.start()
	host := strings.TrimPrefix(srv.URL, "http://")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	c, err := Dial(ctx, host, opt)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestDialPairsAndReturnsNewKey(t *testing.T) {
	tv := &fakeTV{t: t, key: "fresh-key", prompt: true}
	prompted := false

	c := dialFake(t, tv, Options{OnPrompt: func() { prompted = true }})

	if !prompted {
		t.Error("OnPrompt was not called for a TV that raised the on-screen prompt")
	}
	if got := c.Key(); got != "fresh-key" {
		t.Errorf("Key() = %q, want %q", got, "fresh-key")
	}
}

func TestDialReusesStoredKeyWithoutPrompting(t *testing.T) {
	tv := &fakeTV{t: t}
	prompted := false

	c := dialFake(t, tv, Options{Key: "stored-key", OnPrompt: func() { prompted = true }})

	if prompted {
		t.Error("OnPrompt was called even though the stored key was accepted")
	}
	if tv.clientKey != "stored-key" {
		t.Errorf("TV saw client-key %q, want %q", tv.clientKey, "stored-key")
	}
	if got := c.Key(); got != "stored-key" {
		t.Errorf("Key() = %q, want it unchanged", got)
	}
}

func TestRequestDecodesReply(t *testing.T) {
	tv := &fakeTV{t: t, replies: map[string]string{
		"audio/getStatus": `{"returnValue":true,"volume":7,"mute":false,"scenario":"tv_speaker"}`,
	}}
	c := dialFake(t, tv, Options{Key: "k"})

	got, err := c.AudioStatus(context.Background())
	if err != nil {
		t.Fatalf("AudioStatus: %v", err)
	}
	want := AudioStatus{Volume: 7, Mute: false, Scenario: "tv_speaker"}
	if got != want {
		t.Errorf("AudioStatus() = %+v, want %+v", got, want)
	}
}

func TestRequestReportsErrorFrame(t *testing.T) {
	tv := &fakeTV{t: t} // every URI is unknown
	c := dialFake(t, tv, Options{Key: "k"})

	_, err := c.SystemInfo(context.Background())

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("SystemInfo error = %v, want *APIError", err)
	}
	if apiErr.URI != "system/getSystemInfo" {
		t.Errorf("APIError.URI = %q, want %q", apiErr.URI, "system/getSystemInfo")
	}
	if !strings.Contains(apiErr.Message, "404") {
		t.Errorf("APIError.Message = %q, want it to mention 404", apiErr.Message)
	}
}

// A payload-level failure carries returnValue:false inside a "response"
// frame, which is how the tuner reports that there is no broadcast.
func TestRequestReportsPayloadFailure(t *testing.T) {
	tv := &fakeTV{t: t, replies: map[string]string{
		"tv/getCurrentChannel": `{"returnValue":false,"errorText":"Invalid broadcastID error"}`,
	}}
	c := dialFake(t, tv, Options{Key: "k"})

	_, err := c.CurrentChannel(context.Background())

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("CurrentChannel error = %v, want *APIError", err)
	}
	if apiErr.Message != "Invalid broadcastID error" {
		t.Errorf("APIError.Message = %q, want the errorText from the payload", apiErr.Message)
	}
}

func TestSetVolumeClampsToDeviceRange(t *testing.T) {
	tv := &fakeTV{t: t, replies: map[string]string{"audio/setVolume": `{"returnValue":true}`}}
	c := dialFake(t, tv, Options{Key: "k"})

	for _, level := range []int{-5, 400} {
		if err := c.SetVolume(context.Background(), level); err != nil {
			t.Fatalf("SetVolume(%d): %v", level, err)
		}
	}
	if clamp(-5, 0, 100) != 0 || clamp(400, 0, 100) != 100 {
		t.Error("clamp did not hold volume within 0..100")
	}
}

func TestRequestIgnoresUnrelatedFrames(t *testing.T) {
	// Subscriptions and late replies share the socket; a request must skip
	// frames that do not carry its own id.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		ctx := r.Context()
		for {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var msg envelope
			_ = json.Unmarshal(raw, &msg)
			if msg.Type == "register" {
				_ = conn.Write(ctx, websocket.MessageText,
					[]byte(`{"id":"`+msg.ID+`","type":"registered","payload":{"client-key":"k"}}`))
				continue
			}
			// Noise first, then the real answer.
			_ = conn.Write(ctx, websocket.MessageText,
				[]byte(`{"id":"someone_else","type":"response","payload":{"volume":99}}`))
			_ = conn.Write(ctx, websocket.MessageText,
				[]byte(`{"id":"`+msg.ID+`","type":"response","payload":{"returnValue":true,"volume":7}}`))
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Dial(ctx, strings.TrimPrefix(srv.URL, "http://"), Options{Key: "k"})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	got, err := c.AudioStatus(ctx)
	if err != nil {
		t.Fatalf("AudioStatus: %v", err)
	}
	if got.Volume != 7 {
		t.Errorf("AudioStatus().Volume = %d, want 7 (the frame matching our id)", got.Volume)
	}
}
