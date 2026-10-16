package controller

import "testing"

func TestTriggerMappingSources(t *testing.T) {
	for _, test := range []struct {
		mapping string
		want    [2]string
	}{
		{"guid,Pad,lefttrigger:b8,righttrigger:b9,platform:Windows,", [2]string{"b8", "b9"}},
		{"guid,Pad,lefttrigger:+a5~,righttrigger:-a4,", [2]string{"+a5~", "-a4"}},
		{"guid,Pad,righttrigger:a2,", [2]string{"unmapped", "a2"}},
		{"guid,Pad,lefttrigger:,", [2]string{"unmapped", "unmapped"}},
		{"", [2]string{"unmapped", "unmapped"}},
	} {
		if got := triggerMappingSources(test.mapping); got != test.want {
			t.Errorf("sources(%q) = %v, want %v", test.mapping, got, test.want)
		}
	}
}
