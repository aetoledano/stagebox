//go:build !windows || !cgo

package audio

import "errors"

var errUnsupported = errors.New("sound settings require Windows with CGO enabled")

func GetSettings() (Settings, error)             { return Settings{Devices: []Device{}}, errUnsupported }
func CurrentStatus() (Status, error)             { return Status{}, errUnsupported }
func SetOutput(id string) error                  { return errUnsupported }
func ChangeVolume(direction int) (Status, error) { return Status{}, errUnsupported }
func SetVolume(level int) (Status, error)        { return Status{}, errUnsupported }
func ToggleMute() (Status, error)                { return Status{}, errUnsupported }
