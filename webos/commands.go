package webos

import "context"

// SystemInfo is what system/getSystemInfo reports. Older firmware omits most
// fields beyond the model name.
type SystemInfo struct {
	ModelName    string `json:"modelName"`
	ReceiverType string `json:"receiverType"`
	Features     struct {
		DVR    bool `json:"dvr"`
		ThreeD bool `json:"3d"`
	} `json:"features"`
}

// AudioStatus is the current volume state.
type AudioStatus struct {
	Volume   int    `json:"volume"`
	Mute     bool   `json:"mute"`
	Scenario string `json:"scenario"`
}

// ForegroundApp identifies the app currently on screen.
type ForegroundApp struct {
	AppID     string `json:"appId"`
	WindowID  string `json:"windowId"`
	ProcessID string `json:"processId"`
}

// Input is one entry of the external input list.
type Input struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	AppID     string `json:"appId"`
	Port      int    `json:"port"`
	Connected bool   `json:"connected"`
	Favorite  bool   `json:"favorite"`
}

// Channel is one entry of the tuner's channel list. The reply carries far
// more per channel (frequency, CA system IDs, favourite groups); these are
// the fields a remote actually needs.
type Channel struct {
	Number  string `json:"channelNumber"`
	Name    string `json:"channelName"`
	ID      string `json:"channelId"`
	Type    string `json:"channelType"`
	Locked  bool   `json:"locked"`
	Skipped bool   `json:"skipped"`
	HD      bool   `json:"HDTV"`
}

// LaunchPoint is one installed app. webOS 2.x returns an empty list here
// unless the TV granted READ_INSTALLED_APPS during pairing.
type LaunchPoint struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Icon  string `json:"icon"`
}

func (c *Client) SystemInfo(ctx context.Context) (SystemInfo, error) {
	var out SystemInfo
	err := c.Call(ctx, "system/getSystemInfo", nil, &out)
	return out, err
}

func (c *Client) AudioStatus(ctx context.Context) (AudioStatus, error) {
	var out AudioStatus
	err := c.Call(ctx, "audio/getStatus", nil, &out)
	return out, err
}

func (c *Client) SetVolume(ctx context.Context, level int) error {
	return c.Call(ctx, "audio/setVolume", map[string]int{"volume": clamp(level, 0, 100)}, nil)
}

func (c *Client) SetMute(ctx context.Context, mute bool) error {
	return c.Call(ctx, "audio/setMute", map[string]bool{"mute": mute}, nil)
}

func (c *Client) ForegroundApp(ctx context.Context) (ForegroundApp, error) {
	var out ForegroundApp
	err := c.Call(ctx, "com.webos.applicationManager/getForegroundAppInfo", nil, &out)
	return out, err
}

func (c *Client) Inputs(ctx context.Context) ([]Input, error) {
	var out struct {
		Devices []Input `json:"devices"`
	}
	err := c.Call(ctx, "tv/getExternalInputList", nil, &out)
	return out.Devices, err
}

func (c *Client) SwitchInput(ctx context.Context, id string) error {
	return c.Call(ctx, "tv/switchInput", map[string]string{"inputId": id}, nil)
}

func (c *Client) Channels(ctx context.Context) ([]Channel, error) {
	var out struct {
		ChannelList []Channel `json:"channelList"`
	}
	err := c.Call(ctx, "tv/getChannelList", nil, &out)
	return out.ChannelList, err
}

func (c *Client) CurrentChannel(ctx context.Context) (Channel, error) {
	var out Channel
	err := c.Call(ctx, "tv/getCurrentChannel", nil, &out)
	return out, err
}

func (c *Client) OpenChannel(ctx context.Context, number string) error {
	return c.Call(ctx, "tv/openChannel", map[string]string{"channelNumber": number}, nil)
}

func (c *Client) ChannelUp(ctx context.Context) error {
	return c.Call(ctx, "tv/channelUp", nil, nil)
}

func (c *Client) ChannelDown(ctx context.Context) error {
	return c.Call(ctx, "tv/channelDown", nil, nil)
}

func (c *Client) Apps(ctx context.Context) ([]LaunchPoint, error) {
	var out struct {
		LaunchPoints []LaunchPoint `json:"launchPoints"`
	}
	err := c.Call(ctx, "com.webos.applicationManager/listLaunchPoints", nil, &out)
	return out.LaunchPoints, err
}

func (c *Client) Launch(ctx context.Context, appID string) error {
	return c.Call(ctx, "system.launcher/launch", map[string]string{"id": appID}, nil)
}

func (c *Client) CloseApp(ctx context.Context, appID string) error {
	return c.Call(ctx, "system.launcher/close", map[string]string{"id": appID}, nil)
}

// Toast shows a transient notification on screen.
func (c *Client) Toast(ctx context.Context, message string) error {
	return c.Call(ctx, "system.notifications/createToast", map[string]string{"message": message}, nil)
}

// TurnOff puts the TV into standby. Waking it again needs Wake-on-LAN, not SSAP.
func (c *Client) TurnOff(ctx context.Context) error {
	return c.Call(ctx, "system/turnOff", nil, nil)
}

// Media playback controls.
func (c *Client) Play(ctx context.Context) error { return c.Call(ctx, "media.controls/play", nil, nil) }
func (c *Client) Pause(ctx context.Context) error {
	return c.Call(ctx, "media.controls/pause", nil, nil)
}
func (c *Client) Stop(ctx context.Context) error { return c.Call(ctx, "media.controls/stop", nil, nil) }
func (c *Client) Rewind(ctx context.Context) error {
	return c.Call(ctx, "media.controls/rewind", nil, nil)
}
func (c *Client) Forward(ctx context.Context) error {
	return c.Call(ctx, "media.controls/fastForward", nil, nil)
}

// Services lists the SSAP services the firmware actually implements, which is
// the quickest way to tell what a given TV supports.
func (c *Client) Services(ctx context.Context) ([]string, error) {
	var out struct {
		Services []struct {
			Name string `json:"name"`
		} `json:"services"`
	}
	if err := c.Call(ctx, "api/getServiceList", nil, &out); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(out.Services))
	for _, s := range out.Services {
		names = append(names, s.Name)
	}
	return names, nil
}

func clamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}
