package webos

// The registration handshake below is the one LG's own remote app presents.
// webOS validates the signed block against a built-in key, so these values
// are fixed rather than configurable: changing them makes the TV reject the
// pairing outright.

type manifest struct {
	ManifestVersion      int               `json:"manifestVersion"`
	AppVersion           string            `json:"appVersion"`
	Signed               signedManifest    `json:"signed"`
	Permissions          []string          `json:"permissions"`
	LocalizedAppNames    map[string]string `json:"localizedAppNames"`
	LocalizedVendorNames map[string]string `json:"localizedVendorNames"`
}

type signedManifest struct {
	Created              string            `json:"created"`
	AppID                string            `json:"appId"`
	VendorID             string            `json:"vendorId"`
	LocalizedAppNames    map[string]string `json:"localizedAppNames"`
	LocalizedVendorNames map[string]string `json:"localizedVendorNames"`
	Permissions          []string          `json:"permissions"`
	Serial               string            `json:"serial"`
}

var remoteManifest = manifest{
	ManifestVersion: 1,
	AppVersion:      "1.1",
	Signed: signedManifest{
		Created:              "20140509",
		AppID:                "com.lge.test",
		VendorID:             "com.lge",
		LocalizedAppNames:    map[string]string{"": "LG Remote App", "ko-KR": "리모컨 앱"},
		LocalizedVendorNames: map[string]string{"": "LG Electronics"},
		Permissions: []string{
			"TEST_SECURE", "CONTROL_INPUT_TEXT", "CONTROL_MOUSE_AND_KEYBOARD",
			"READ_INSTALLED_APPS", "READ_LGE_SDX", "READ_NOTIFICATIONS", "SEARCH",
			"WRITE_SETTINGS", "WRITE_NOTIFICATION_ALERT", "CONTROL_POWER",
			"READ_CURRENT_CHANNEL", "READ_RUNNING_APPS", "READ_UPDATE_INFO",
			"UPDATE_FROM_REMOTE_APP", "READ_LGE_TV_INPUT_EVENTS", "READ_TV_CURRENT_TIME",
		},
		Serial: "2f930e2d2cfe083771f68e4fe7bb07",
	},
	Permissions: []string{
		"LAUNCH", "LAUNCH_WEBAPP", "APP_TO_APP", "CLOSE", "TEST_OPEN", "TEST_PROTECTED",
		"CONTROL_AUDIO", "CONTROL_DISPLAY", "CONTROL_INPUT_JOYSTICK",
		"CONTROL_INPUT_MEDIA_RECORDING", "CONTROL_INPUT_MEDIA_PLAYBACK",
		"CONTROL_INPUT_TV", "CONTROL_POWER", "READ_APP_STATUS", "READ_CURRENT_CHANNEL",
		"READ_INPUT_DEVICE_LIST", "READ_NETWORK_STATE", "READ_RUNNING_APPS",
		"READ_TV_CHANNEL_LIST", "WRITE_NOTIFICATION_TOAST", "READ_POWER_STATE",
		"READ_COUNTRY_INFO", "READ_SETTINGS", "CONTROL_TV_SCREEN", "CONTROL_TV_STANBY",
		"CONTROL_FAVORITE_GROUP", "CONTROL_USER_INFO", "CHECK_BLUETOOTH_DEVICE",
		"CONTROL_BLUETOOTH", "CONTROL_TIMER_INFO", "STB_INTERNAL_CONNECTION",
		"CONTROL_RECORDING", "READ_RECORDING_STATE", "WRITE_RECORDING_LIST",
		"READ_RECORDING_LIST", "READ_RECORDING_SCHEDULE", "WRITE_RECORDING_SCHEDULE",
		"READ_STORAGE_DEVICE_LIST", "READ_TV_PROGRAM_INFO", "CONTROL_BOX_CHANNEL",
		"READ_TV_ACR_AUTH_TOKEN", "READ_TV_CONTENT_STATE", "READ_TV_CURRENT_TIME",
		"ADD_LAUNCHER_CHANNEL", "SET_CHANNEL_SKIP", "RELEASE_CHANNEL_SKIP",
		"CONTROL_CHANNEL_BLOCK", "DELETE_SELECT_CHANNEL", "CONTROL_CHANNEL_GROUP",
		"SCAN_TV_CHANNELS", "CONTROL_TV_POWER", "CONTROL_WOL",
	},
	LocalizedAppNames:    map[string]string{"": "LG Remote App"},
	LocalizedVendorNames: map[string]string{"": "LG Electronics"},
}
