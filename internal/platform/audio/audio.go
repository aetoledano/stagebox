package audio

const VolumeStep = 2

type Device struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Current bool   `json:"current"`
}

type Status struct {
	Available  bool   `json:"available"`
	DeviceID   string `json:"deviceID"`
	DeviceName string `json:"deviceName"`
	Volume     int    `json:"volume"`
	Muted      bool   `json:"muted"`
}

type Settings struct {
	Devices []Device `json:"devices"`
	Status  Status   `json:"status"`
}
