package webos

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// searchTarget is the SSDP service LG TVs advertise for second-screen control.
const searchTarget = "urn:lge-com:service:webos-second-screen:1"

// Device is a TV found on the local network.
type Device struct {
	Host     string // IP address
	Name     string // friendlyName from the device description, if reachable
	Model    string // modelName from the device description, if reachable
	Location string // URL of the UPnP device description
}

// Discover sends an SSDP M-SEARCH on every multicast-capable interface and
// collects the TVs that answer within timeout. Devices whose description
// cannot be fetched are still returned, with only Host and Location set.
func Discover(ctx context.Context, timeout time.Duration) ([]Device, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, fmt.Errorf("open SSDP socket: %w", err)
	}
	defer conn.Close()

	group := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	probe := []byte("M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 2\r\n" +
		"ST: " + searchTarget + "\r\n\r\n")

	// Repeat the probe: SSDP is UDP and the first datagram is often dropped
	// while the interface joins the multicast group.
	for range 3 {
		if _, err := conn.WriteTo(probe, group); err != nil {
			return nil, fmt.Errorf("send M-SEARCH: %w", err)
		}
	}

	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}

	found := map[string]Device{}
	buf := make([]byte, 4096)
	for {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			break // Deadline reached; whatever answered is in found.
		}
		host, _, _ := net.SplitHostPort(from.String())
		if _, seen := found[host]; seen {
			continue
		}
		dev := Device{Host: host, Location: ssdpHeader(buf[:n], "LOCATION")}
		found[host] = dev
	}

	out := make([]Device, 0, len(found))
	for _, dev := range found {
		if dev.Location != "" {
			if name, model, err := describe(ctx, dev.Location); err == nil {
				dev.Name, dev.Model = name, model
			}
		}
		out = append(out, dev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out, nil
}

func ssdpHeader(resp []byte, key string) string {
	sc := bufio.NewScanner(bytes.NewReader(resp))
	for sc.Scan() {
		name, value, ok := strings.Cut(sc.Text(), ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// describe fetches the UPnP device description for a friendly name and model.
func describe(ctx context.Context, location string) (name, model string, err error) {
	if _, err := url.Parse(location); err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("describe %s: %s", location, resp.Status)
	}

	var doc struct {
		Device struct {
			FriendlyName string `xml:"friendlyName"`
			ModelName    string `xml:"modelName"`
		} `xml:"device"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return "", "", err
	}
	return doc.Device.FriendlyName, doc.Device.ModelName, nil
}
