//go:build windows

package tools

var procShowCursor = user32.NewProc("ShowCursor")

// Hides/shows cursor globally using the display-count semantics:
// visible when count >= 0, hidden when count < 0.
func SetCursorHidden(hidden bool) {
	if hidden {
		for {
			r, _, _ := procShowCursor.Call(0) // FALSE
			if int32(r) < 0 {
				break // now hidden
			}
		}
	} else {
		for {
			r, _, _ := procShowCursor.Call(1) // TRUE
			if int32(r) >= 0 {
				break // now visible
			}
		}
	}
}
