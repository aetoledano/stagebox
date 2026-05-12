//go:build !windows

package wifi

import "errors"

type Network struct {
	SSID           string `json:"ssid"`
	Authentication string `json:"authentication"`
	Encryption     string `json:"encryption"`
	Signal         int    `json:"signal"`
	Secure         bool   `json:"secure"`
}

type Status struct {
	Available bool   `json:"available"`
	Connected bool   `json:"connected"`
	SSID      string `json:"ssid"`
	State     string `json:"state"`
}

func Scan() ([]Network, error) {
	return nil, errors.New("wifi scanning is only available on Windows")
}

func Connect(ssid string, password string) error {
	return errors.New("wifi connection is only available on Windows")
}

func CurrentStatus() Status {
	return Status{State: "unsupported"}
}
