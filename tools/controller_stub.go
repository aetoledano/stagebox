//go:build !windows

package tools

import "context"

type XInputBridge struct{}

func StartXInputBridge(ctx context.Context) *XInputBridge { return nil }
func (b *XInputBridge) Pause()                            {}
func (b *XInputBridge) Resume()                           {}
func (b *XInputBridge) Stop()                             {}
