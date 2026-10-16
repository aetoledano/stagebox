package controller

import "strings"

// Keep the actual SDL mapping token, including half-axis/inversion modifiers.
// A digital binding cannot provide pressure; never guess an axis by pad name.
func triggerMappingSources(mapping string) [2]string {
	sources := [2]string{"unmapped", "unmapped"}
	fields := strings.Split(mapping, ",")
	if len(fields) < 3 {
		return sources
	}
	for _, field := range fields[2:] {
		key, value, ok := strings.Cut(field, ":")
		if !ok || value == "" {
			continue
		}
		switch key {
		case "lefttrigger":
			sources[0] = value
		case "righttrigger":
			sources[1] = value
		}
	}
	return sources
}
