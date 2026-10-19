// Package webos speaks SSAP, the WebSocket protocol LG smart TVs expose on
// the local network for remote control.
package webos

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/coder/websocket"
)

// Ports SSAP listens on. Pre-2016 firmware negotiates TLS with ciphers that
// modern Go refuses, so the plaintext port is tried first; both are LAN-only.
const (
	PlainPort = "3000"
	TLSPort   = "3001"
)

// Client is a connection to one TV. It is not safe for concurrent use: each
// Request writes a frame and then reads until the matching reply arrives.
type Client struct {
	conn *websocket.Conn
	host string
	url  string
	key  string
	seq  atomic.Uint64
}

// Options configures Dial.
type Options struct {
	// Key is the client key from a previous pairing. When empty, or when the
	// TV no longer honours it, the TV shows a confirmation prompt on screen.
	Key string
	// OnPrompt is called when the TV asks the user to confirm with the remote.
	OnPrompt func()
	// Insecure permits the TLS port with certificate verification disabled.
	// TVs ship a self-signed certificate, so this is the only way to use it.
	Insecure bool
}

// APIError is a non-success reply from the TV.
type APIError struct {
	URI     string
	Message string
	Payload json.RawMessage
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return "ssap://" + e.URI + ": request failed"
	}
	return "ssap://" + e.URI + ": " + e.Message
}

type envelope struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	URI     string          `json:"uri,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type registerPayload struct {
	ForcePairing bool     `json:"forcePairing"`
	PairingType  string   `json:"pairingType"`
	ClientKey    string   `json:"client-key,omitempty"`
	Manifest     manifest `json:"manifest"`
}

// Dial connects to host and completes the SSAP handshake. host may carry an
// explicit port; otherwise both SSAP ports are tried in turn.
func Dial(ctx context.Context, host string, opt Options) (*Client, error) {
	c := &Client{host: host, key: opt.Key}

	var errs []error
	for _, cand := range endpoints(host, opt.Insecure) {
		conn, _, err := websocket.Dial(ctx, cand.url, &websocket.DialOptions{
			HTTPClient: cand.client,
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", cand.url, err))
			continue
		}
		// Channel lists on a full DVB scan run well past the default limit.
		conn.SetReadLimit(8 << 20)
		c.conn, c.url = conn, cand.url
		if err := c.register(ctx, opt.OnPrompt); err != nil {
			conn.Close(websocket.StatusInternalError, "handshake failed")
			c.conn = nil
			return nil, err
		}
		return c, nil
	}
	return nil, fmt.Errorf("connect to %s: %w", host, errors.Join(errs...))
}

type endpoint struct {
	url    string
	client *http.Client
}

func endpoints(host string, insecure bool) []endpoint {
	if h, p, err := net.SplitHostPort(host); err == nil {
		if p == TLSPort {
			return []endpoint{tlsEndpoint(h, p)}
		}
		return []endpoint{{url: "ws://" + net.JoinHostPort(h, p) + "/"}}
	}
	eps := []endpoint{{url: "ws://" + net.JoinHostPort(host, PlainPort) + "/"}}
	if insecure {
		eps = append(eps, tlsEndpoint(host, TLSPort))
	}
	return eps
}

func tlsEndpoint(host, port string) endpoint {
	return endpoint{
		url: "wss://" + net.JoinHostPort(host, port) + "/",
		client: &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // TVs present a self-signed certificate.
				MinVersion:         tls.VersionTLS10,
			},
		}},
	}
}

// Key returns the client key for this session. After a fresh pairing it
// differs from Options.Key and should be persisted.
func (c *Client) Key() string { return c.key }

// Host returns the host the client was dialled with.
func (c *Client) Host() string { return c.host }

// URL returns the endpoint that answered.
func (c *Client) URL() string { return c.url }

// Close ends the session.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close(websocket.StatusNormalClosure, "")
}

func (c *Client) register(ctx context.Context, onPrompt func()) error {
	req := envelope{ID: "register_0", Type: "register"}
	payload, err := json.Marshal(registerPayload{
		PairingType: "PROMPT",
		ClientKey:   c.key,
		Manifest:    remoteManifest,
	})
	if err != nil {
		return fmt.Errorf("encode handshake: %w", err)
	}
	req.Payload = payload
	if err := c.write(ctx, req); err != nil {
		return err
	}

	prompted := false
	for {
		msg, err := c.read(ctx)
		if err != nil {
			return fmt.Errorf("pairing: %w", err)
		}
		switch msg.Type {
		case "registered":
			var reply struct {
				ClientKey string `json:"client-key"`
			}
			if err := json.Unmarshal(msg.Payload, &reply); err != nil {
				return fmt.Errorf("decode pairing reply: %w", err)
			}
			if reply.ClientKey != "" {
				c.key = reply.ClientKey
			}
			return nil
		case "error":
			return fmt.Errorf("pairing rejected: %s", msg.Error)
		case "response":
			// The TV acknowledges first and only then raises the on-screen
			// prompt; the "registered" frame follows once the user accepts.
			var reply struct {
				PairingType string `json:"pairingType"`
			}
			_ = json.Unmarshal(msg.Payload, &reply)
			if reply.PairingType == "PROMPT" && !prompted {
				prompted = true
				if onPrompt != nil {
					onPrompt()
				}
			}
		}
	}
}

// Request calls an SSAP endpoint. uri is given without the "ssap://" prefix.
// payload may be nil. The raw reply payload is returned so callers can decode
// only the fields they need; firmware differs widely in what it includes.
func (c *Client) Request(ctx context.Context, uri string, payload any) (json.RawMessage, error) {
	id := "req_" + strconv.FormatUint(c.seq.Add(1), 10)
	req := envelope{ID: id, Type: "request", URI: "ssap://" + uri}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode payload for %s: %w", uri, err)
		}
		req.Payload = raw
	}
	if err := c.write(ctx, req); err != nil {
		return nil, err
	}

	for {
		msg, err := c.read(ctx)
		if err != nil {
			return nil, fmt.Errorf("ssap://%s: %w", uri, err)
		}
		if msg.ID != id {
			continue // A reply to an earlier call, or an unsolicited event.
		}
		if msg.Type == "error" {
			return msg.Payload, &APIError{URI: uri, Message: msg.Error, Payload: msg.Payload}
		}
		// Some methods report failure in the payload with type "response".
		var status struct {
			ReturnValue *bool  `json:"returnValue"`
			ErrorText   string `json:"errorText"`
		}
		_ = json.Unmarshal(msg.Payload, &status)
		if status.ReturnValue != nil && !*status.ReturnValue {
			return msg.Payload, &APIError{URI: uri, Message: status.ErrorText, Payload: msg.Payload}
		}
		return msg.Payload, nil
	}
}

// Call is Request with the reply decoded into out.
func (c *Client) Call(ctx context.Context, uri string, payload, out any) error {
	raw, err := c.Request(ctx, uri, payload)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode ssap://%s reply: %w", uri, err)
	}
	return nil
}

func (c *Client) write(ctx context.Context, msg envelope) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	if err := c.conn.Write(ctx, websocket.MessageText, raw); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return nil
}

func (c *Client) read(ctx context.Context) (envelope, error) {
	_, raw, err := c.conn.Read(ctx)
	if err != nil {
		return envelope{}, err
	}
	var msg envelope
	if err := json.Unmarshal(raw, &msg); err != nil {
		return envelope{}, fmt.Errorf("decode reply: %w", err)
	}
	return msg, nil
}
