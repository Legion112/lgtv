// Command lgtv is a command-line remote for LG smart TVs on the local network.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Legion112/lgtv/webos"
)

const usage = `lgtv — remote control for LG webOS TVs over SSAP.

Usage:
  lgtv [flags] <command> [arguments]

Commands:
  discover              find TVs on the local network via SSDP
  info                  model, current app, volume
  services              SSAP services this firmware implements
  volume                print current volume
  volume <n|+n|-n>      set volume, absolutely or relative to current
  mute on|off           mute or unmute
  toast <message>       show a notification on screen
  apps                  list installed apps
  launch <appID>        launch an app
  close <appID>         close an app
  inputs                list external inputs
  input <id>            switch to an input, e.g. HDMI_1
  channels              list tuner channels
  channel up|down|<n>   change channel
  play|pause|stop|rewind|forward
  off                   put the TV into standby
  raw <uri> [json]      call any SSAP endpoint, e.g. raw audio/getStatus

The TV is chosen from -host, then $LGTV_HOST, then the last host used.
The first connection to a TV raises an on-screen prompt that must be accepted
with the remote; the resulting key is saved and reused.

Flags:
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lgtv: "+err.Error())
		os.Exit(1)
	}
}

type options struct {
	host     string
	timeout  time.Duration
	insecure bool
	jsonOut  bool
}

func run() error {
	var opt options
	fs := flag.NewFlagSet("lgtv", flag.ContinueOnError)
	fs.StringVar(&opt.host, "host", "", "TV address (default: $LGTV_HOST, else last used)")
	fs.DurationVar(&opt.timeout, "timeout", 20*time.Second, "per-command timeout; pairing waits longer")
	fs.BoolVar(&opt.insecure, "insecure", false, "also try the TLS port, skipping certificate verification")
	fs.BoolVar(&opt.jsonOut, "json", false, "print raw JSON replies")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usage)
		fs.PrintDefaults()
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	args := fs.Args()
	if len(args) == 0 {
		fs.Usage()
		return errors.New("no command given")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	store, err := webos.NewKeyStore()
	if err != nil {
		return err
	}

	if args[0] == "discover" {
		return discover(ctx, os.Stdout, opt)
	}
	return dispatch(ctx, os.Stdout, opt, store, args)
}

func discover(ctx context.Context, w io.Writer, opt options) error {
	window := opt.timeout
	if window > 10*time.Second {
		window = 5 * time.Second // SSDP replies arrive within MX seconds.
	}
	devices, err := webos.Discover(ctx, window)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		return errors.New("no TVs answered; check that the TV is on and on this network")
	}
	if opt.jsonOut {
		return writeJSON(w, devices)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ADDRESS\tMODEL\tNAME")
	for _, d := range devices {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", d.Host, dash(d.Model), dash(d.Name))
	}
	return tw.Flush()
}

// resolveHost applies the host precedence: flag, environment, last used.
func resolveHost(opt options, store *webos.KeyStore) (string, error) {
	if opt.host != "" {
		return opt.host, nil
	}
	if h := os.Getenv("LGTV_HOST"); h != "" {
		return h, nil
	}
	if h := store.Host(); h != "" {
		return h, nil
	}
	return "", errors.New("no TV address: pass -host, set $LGTV_HOST, or run `lgtv discover`")
}

func dispatch(ctx context.Context, w io.Writer, opt options, store *webos.KeyStore, args []string) error {
	host, err := resolveHost(opt, store)
	if err != nil {
		return err
	}

	// Pairing blocks on a human reaching for the remote, so the dial gets a
	// generous deadline of its own regardless of -timeout.
	known := store.Key(host)
	dialTimeout := opt.timeout
	if known == "" && dialTimeout < 90*time.Second {
		dialTimeout = 90 * time.Second
	}
	dialCtx, cancelDial := context.WithTimeout(ctx, dialTimeout)
	defer cancelDial()

	tv, err := webos.Dial(dialCtx, host, webos.Options{
		Key:      known,
		Insecure: opt.insecure,
		OnPrompt: func() {
			fmt.Fprintln(os.Stderr, "lgtv: confirm the connection request on the TV screen...")
		},
	})
	if err != nil {
		return err
	}
	defer tv.Close()

	if key := tv.Key(); key != "" && key != known {
		if err := store.SaveKey(host, key); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "lgtv: paired, key saved")
	}
	if host != store.Host() {
		if err := store.SaveHost(host); err != nil {
			return err
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, opt.timeout)
	defer cancel()
	return execute(cmdCtx, w, opt, tv, args)
}

func execute(ctx context.Context, w io.Writer, opt options, tv *webos.Client, args []string) error {
	cmd, rest := args[0], args[1:]

	arg := func(what string) (string, error) {
		if len(rest) == 0 {
			return "", fmt.Errorf("%s: %s required", cmd, what)
		}
		return rest[0], nil
	}

	switch cmd {
	case "info":
		return info(ctx, w, opt, tv)

	case "services":
		names, err := tv.Services(ctx)
		if err != nil {
			return err
		}
		if opt.jsonOut {
			return writeJSON(w, names)
		}
		fmt.Fprintln(w, strings.Join(names, "\n"))
		return nil

	case "volume":
		return volume(ctx, w, opt, tv, rest)

	case "mute":
		state, err := arg("on or off")
		if err != nil {
			return err
		}
		switch state {
		case "on":
			return tv.SetMute(ctx, true)
		case "off":
			return tv.SetMute(ctx, false)
		default:
			return fmt.Errorf("mute: want on or off, got %q", state)
		}

	case "toast":
		if len(rest) == 0 {
			return errors.New("toast: message required")
		}
		return tv.Toast(ctx, strings.Join(rest, " "))

	case "apps":
		apps, err := tv.Apps(ctx)
		if err != nil {
			return err
		}
		if opt.jsonOut {
			return writeJSON(w, apps)
		}
		if len(apps) == 0 {
			return errors.New("the TV returned an empty app list; " +
				"webOS 2.x withholds it unless pairing granted READ_INSTALLED_APPS")
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tTITLE")
		for _, a := range apps {
			fmt.Fprintf(tw, "%s\t%s\n", a.ID, a.Title)
		}
		return tw.Flush()

	case "launch":
		id, err := arg("app ID")
		if err != nil {
			return err
		}
		return tv.Launch(ctx, id)

	case "close":
		id, err := arg("app ID")
		if err != nil {
			return err
		}
		return tv.CloseApp(ctx, id)

	case "inputs":
		inputs, err := tv.Inputs(ctx)
		if err != nil {
			return err
		}
		if opt.jsonOut {
			return writeJSON(w, inputs)
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tLABEL\tCONNECTED")
		for _, in := range inputs {
			fmt.Fprintf(tw, "%s\t%s\t%t\n", in.ID, in.Label, in.Connected)
		}
		return tw.Flush()

	case "input":
		id, err := arg("input ID")
		if err != nil {
			return err
		}
		return tv.SwitchInput(ctx, id)

	case "channels":
		channels, err := tv.Channels(ctx)
		if err != nil {
			return err
		}
		if opt.jsonOut {
			return writeJSON(w, channels)
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NUMBER\tNAME\tTYPE\tFLAGS")
		for _, ch := range channels {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				ch.Number, dash(ch.Name), dash(ch.Type), channelFlags(ch))
		}
		return tw.Flush()

	case "channel":
		which, err := arg("up, down or a channel number")
		if err != nil {
			return err
		}
		switch which {
		case "up":
			return tv.ChannelUp(ctx)
		case "down":
			return tv.ChannelDown(ctx)
		default:
			return tv.OpenChannel(ctx, which)
		}

	case "play":
		return tv.Play(ctx)
	case "pause":
		return tv.Pause(ctx)
	case "stop":
		return tv.Stop(ctx)
	case "rewind":
		return tv.Rewind(ctx)
	case "forward":
		return tv.Forward(ctx)

	case "off":
		return tv.TurnOff(ctx)

	case "raw":
		uri, err := arg("an ssap URI")
		if err != nil {
			return err
		}
		var payload any
		if len(rest) > 1 {
			if err := json.Unmarshal([]byte(rest[1]), &payload); err != nil {
				return fmt.Errorf("raw: payload is not valid JSON: %w", err)
			}
		}
		raw, err := tv.Request(ctx, uri, payload)
		if err != nil && raw == nil {
			return err
		}
		if printErr := writeJSON(w, json.RawMessage(raw)); printErr != nil {
			return printErr
		}
		return err

	default:
		return fmt.Errorf("unknown command %q; run `lgtv -h` for the list", cmd)
	}
}

func info(ctx context.Context, w io.Writer, opt options, tv *webos.Client) error {
	sys, sysErr := tv.SystemInfo(ctx)
	app, appErr := tv.ForegroundApp(ctx)
	audio, audioErr := tv.AudioStatus(ctx)

	if opt.jsonOut {
		return writeJSON(w, map[string]any{
			"host":          tv.Host(),
			"url":           tv.URL(),
			"system":        sys,
			"foregroundApp": app,
			"audio":         audio,
		})
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "address\t%s\n", tv.Host())
	fmt.Fprintf(tw, "model\t%s\n", valueOr(sys.ModelName, sysErr))
	if sys.ReceiverType != "" {
		fmt.Fprintf(tw, "tuner\t%s\n", sys.ReceiverType)
	}
	fmt.Fprintf(tw, "app\t%s\n", valueOr(app.AppID, appErr))
	if audioErr != nil {
		fmt.Fprintf(tw, "volume\t%s\n", errText(audioErr))
	} else {
		muted := ""
		if audio.Mute {
			muted = " (muted)"
		}
		fmt.Fprintf(tw, "volume\t%d%s\n", audio.Volume, muted)
		if audio.Scenario != "" {
			fmt.Fprintf(tw, "output\t%s\n", audio.Scenario)
		}
	}
	return tw.Flush()
}

func volume(ctx context.Context, w io.Writer, opt options, tv *webos.Client, rest []string) error {
	if len(rest) == 0 {
		status, err := tv.AudioStatus(ctx)
		if err != nil {
			return err
		}
		if opt.jsonOut {
			return writeJSON(w, status)
		}
		fmt.Fprintln(w, status.Volume)
		return nil
	}

	spec := rest[0]
	if relative := strings.HasPrefix(spec, "+") || strings.HasPrefix(spec, "-"); relative {
		delta, err := strconv.Atoi(spec)
		if err != nil {
			return fmt.Errorf("volume: %q is not a number", spec)
		}
		status, err := tv.AudioStatus(ctx)
		if err != nil {
			return err
		}
		return tv.SetVolume(ctx, status.Volume+delta)
	}
	level, err := strconv.Atoi(spec)
	if err != nil {
		return fmt.Errorf("volume: %q is not a number", spec)
	}
	return tv.SetVolume(ctx, level)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// valueOr renders a field that some firmware simply does not report.
func valueOr(value string, err error) string {
	if err != nil {
		return errText(err)
	}
	return dash(value)
}

func errText(err error) string {
	var apiErr *webos.APIError
	if errors.As(err, &apiErr) && apiErr.Message != "" {
		return "unavailable (" + apiErr.Message + ")"
	}
	return "unavailable"
}

// channelFlags renders the handful of per-channel booleans worth seeing in a
// list, so the table stays narrow when none of them are set.
func channelFlags(ch webos.Channel) string {
	var flags []string
	if ch.HD {
		flags = append(flags, "HD")
	}
	if ch.Locked {
		flags = append(flags, "locked")
	}
	if ch.Skipped {
		flags = append(flags, "skipped")
	}
	return strings.Join(flags, ",")
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
