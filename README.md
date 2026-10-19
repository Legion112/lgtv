# lgtv

Go CLI and library for controlling LG smart TVs over **SSAP**, the WebSocket
protocol webOS exposes on the local network. No cloud, no vendor app: the tool
talks straight to the TV.

Requires **Go 1.27**.

## Install

```bash
go install github.com/Legion112/lgtv/cmd/lgtv@latest
```

Or from a checkout:

```bash
make build && sudo make install
```

## Quick start

Find the TV, then talk to it:

```bash
lgtv discover
```

```
ADDRESS        MODEL        NAME
192.168.8.187  LG Smart TV  [LG] webOS TV LF630V
```

```bash
lgtv -host 192.168.8.187 info
```

```
address  192.168.8.187
model    55LF630V-ZA
tuner    dvb
app      youtube.leanback.v4
volume   7
output   mastervolume_tv_speaker
```

The **first** connection to a TV raises a confirmation prompt on screen that
has to be accepted with the remote. The key the TV hands back is saved to
`$XDG_CONFIG_HOME/lgtv/<host>.key` (mode `0600`) and reused, so later commands
connect without asking. The host is remembered too — after the first run,
`-host` can be dropped.

## Commands

| Command | Effect |
| --- | --- |
| `discover` | find TVs on the LAN via SSDP |
| `info` | model, tuner, current app, volume |
| `services` | SSAP services this firmware implements |
| `volume` / `volume <n\|+n\|-n>` | read or set volume, absolute or relative |
| `mute on\|off` | mute or unmute |
| `toast <message>` | notification on screen |
| `apps` / `launch <appID>` / `close <appID>` | installed apps |
| `inputs` / `input <id>` | list and switch external inputs (`HDMI_1`, …) |
| `channels` / `channel up\|down\|<n>` | tuner |
| `play` `pause` `stop` `rewind` `forward` | media playback |
| `off` | standby — waking the TV again needs Wake-on-LAN, not SSAP |
| `raw <uri> [json]` | call any SSAP endpoint directly |

`raw` is the escape hatch for anything not wrapped above:

```bash
lgtv raw audio/getStatus
```

```bash
lgtv raw system.notifications/createToast '{"message":"hello"}'
```

Add `-json` to any command to get the reply verbatim instead of a table.

## Host selection

In order of precedence: `-host`, then `$LGTV_HOST`, then the last host used.

## Library

```go
tv, err := webos.Dial(ctx, "192.168.8.187", webos.Options{
    Key:      storedKey, // empty on first run; the TV then prompts on screen
    OnPrompt: func() { log.Println("accept the prompt on the TV") },
})
if err != nil {
    return err
}
defer tv.Close()

status, err := tv.AudioStatus(ctx)
```

Typed helpers cover the common endpoints; `Request` and `Call` reach anything
else. Replies come back as raw JSON so callers decode only the fields they
need — firmware differs widely in what it includes. A `*webos.APIError`
carries the TV's own message, including payload-level failures that arrive as
a nominally successful `response` frame with `returnValue: false`.

A `Client` is a single connection and is **not** safe for concurrent use: each
request writes a frame and reads until the matching reply arrives.

## Firmware differences

Capabilities vary a lot by model year — run `lgtv services` to see what a given
TV actually implements. Observed on a 55LF630V (2015, webOS 2.0):

- `apps` fails with `401 insufficient permissions`: webOS 2.x withholds the
  app list unless pairing granted `READ_INSTALLED_APPS`, which it does not.
  Launching a known app ID still works.
- `getCurrentSWInformation` is likewise `401`.
- `getPowerState` and the connection-manager status return `404`; those
  methods arrived in later firmware.
- Port **3001** (TLS) negotiates ciphers modern Go refuses, so the plaintext
  port **3000** is used by default. `-insecure` additionally tries 3001 with
  certificate verification disabled, for TVs that need it.

## Development

```bash
make test
```

```bash
make vet lint
```

Tests run against an in-process fake TV, so no hardware is needed.
