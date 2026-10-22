package controller

import (
	"strings"
	"testing"
	"time"
)

func TestMappingCaptureInvertedAxisAndAliasedInputs(t *testing.T) {
	c := newMappingCapture("id", "03000b19c82d00000261000000000000", "SN30 Pro+", 1, 0)
	c.calibrated = true
	c.phase = "press"
	c.step = 17 // right X
	c.baseline = mappingInput{axes: make([]int16, 6)}
	c.bindings["lefty"] = "a1"
	now := time.Unix(100, 0)
	// A duplicated left-Y signal on a2 must not influence an idle stick.
	in := mappingInput{axes: []int16{0, 0, 0, -30000, 0, 0}}
	c.update(in, now)
	c.update(in, now.Add(120*time.Millisecond))
	if c.phase != "release-opposite" || c.firstAxis != 3 {
		t.Fatalf("wrong first capture: %+v", c.state())
	}
	center := mappingInput{axes: make([]int16, 6)}
	c.update(center, now.Add(130*time.Millisecond))
	c.update(center, now.Add(330*time.Millisecond))
	in.axes[3] = 30000
	c.update(in, now.Add(340*time.Millisecond))
	c.update(in, now.Add(460*time.Millisecond))
	if c.bindings["rightx"] != "a3~" {
		t.Fatalf("inversion not retained: %v", c.bindings)
	}
	c.update(center, now.Add(470*time.Millisecond))
	c.update(center, now.Add(670*time.Millisecond))
	// Right-Y aliases on a4 and a5: deterministically choose the first free axis.
	in.axes = []int16{0, 0, 0, 0, 32000, 32000}
	c.update(in, now.Add(680*time.Millisecond))
	c.update(in, now.Add(800*time.Millisecond))
	if c.firstAxis != 4 {
		t.Fatalf("wrong duplicated-axis choice: %d", c.firstAxis)
	}
}

func TestMappingRequiresReleaseAndRejectsDuplicateButtons(t *testing.T) {
	c := newMappingCapture("id", "03000b19c82d00000261000000000000", "Pad", 1, 0)
	c.calibrated = true
	c.phase = "press"
	c.baseline = mappingInput{buttons: make([]bool, 2)}
	now := time.Unix(100, 0)
	press := mappingInput{buttons: []bool{true, false}}
	c.update(press, now)
	c.update(press, now.Add(120*time.Millisecond))
	c.update(press, now.Add(time.Second))
	if c.step != 0 || c.phase != "release" {
		t.Fatal("advanced while held")
	}
	neutral := mappingInput{buttons: make([]bool, 2)}
	c.update(neutral, now.Add(2*time.Second))
	c.update(neutral, now.Add(2200*time.Millisecond))
	if c.step != 1 {
		t.Fatal("did not advance after neutral")
	}
	c.update(press, now.Add(3*time.Second))
	c.update(press, now.Add(3200*time.Millisecond))
	if c.bindings["b"] != "" {
		t.Fatal("duplicate button accepted")
	}
}

func TestMappingOverridePreservesOtherCRCAndComments(t *testing.T) {
	guid := "03000b19c82d00000261000000000000"
	old := guid + ",Old,a:b0,platform:Windows,"
	other := "0300abcdc82d00000261000000000000,Other,a:b0,platform:Windows,"
	fallback := "03000000c82d00000261000000000000,Fallback,a:b0,platform:Windows,"
	row := guid + ",New,a:b1,platform:Windows,"
	out := string(upsertMappingOverride([]byte("# keep\n"+old+"\n"+other+"\n"+fallback+"\n"), guid, row))
	if strings.Contains(out, old) || !strings.Contains(out, other) || !strings.Contains(out, fallback) || !strings.HasPrefix(out, "# keep\n") || !strings.Contains(out, row) {
		t.Fatal(out)
	}
	if !mappingOverrideMatches(fallback, guid) || mappingOverrideMatches(other, guid) {
		t.Fatal("CRC identity mismatch")
	}
	crcRow := "03000000c82d00000261000000000000,CRC,a:b0,crc:190b,platform:Windows,"
	if !mappingOverrideMatches(crcRow, guid) {
		t.Fatal("SDL CRC field not recognized")
	}
}

func TestOverrideForcesVirtualWithoutSavedDefault(t *testing.T) {
	var store *controllerPreferenceStore
	info := store.apply(ControllerInfo{CustomMapping: true})
	if !info.Known || info.Mode != ControllerVirtual || !controllerUsesVirtual(info, false) {
		t.Fatalf("override not routed virtually: %+v", info)
	}
	store = &controllerPreferenceStore{data: controllerPreferences{Controllers: map[string]controllerPreference{"pad": {Mode: ControllerNative}}}}
	info = store.apply(ControllerInfo{ID: "pad", CustomMapping: true})
	if info.Mode != ControllerVirtual || !info.Known {
		t.Fatal("saved Native overrode custom mapping")
	}
}

func TestMappingTriggerPrefersAnalogAndAllowsOppositeSharedHalf(t *testing.T) {
	c := newMappingCapture("id", "03000b19c82d00000261000000000000", "Pad", 1, 0)
	c.step = 19
	c.baseline = mappingInput{axes: []int16{0}, buttons: []bool{false}}
	if got := c.candidateFor(mappingInput{axes: []int16{32000}, buttons: []bool{true}}); got != "+a0" {
		t.Fatalf("lost analog pressure: %s", got)
	}
	c.bindings["lefttrigger"] = "+a0"
	c.step = 20
	if got := c.candidateFor(mappingInput{axes: []int16{-32000}, buttons: []bool{false}}); got != "-a0" {
		t.Fatalf("shared trigger axis: %s", got)
	}
	if got := c.candidateFor(mappingInput{axes: []int16{32000}, buttons: []bool{false}}); got != "" {
		t.Fatalf("accepted duplicate trigger input: %s", got)
	}
}

func TestMappingHoldSkipWaitsForReleaseBeforeAdvancing(t *testing.T) {
	c := newMappingCapture("id", "03000b19c82d00000261000000000000", "Pad", 1, 0)
	c.calibrated = true
	c.phase = "press"
	c.step = 12
	c.bindings["a"] = "b0"
	c.baseline = mappingInput{buttons: []bool{false}}
	now := time.Unix(100, 0)
	held := mappingInput{buttons: []bool{true}}
	c.update(held, now)
	c.update(held, now.Add(1600*time.Millisecond))
	c.update(held, now.Add(2*time.Second))
	if c.step != 12 || c.phase != "release" || c.bindings["guide"] != "" {
		t.Fatal("skip did not wait for release")
	}
	neutral := mappingInput{buttons: []bool{false}}
	c.update(neutral, now.Add(3*time.Second))
	c.update(neutral, now.Add(3200*time.Millisecond))
	if c.step != 13 {
		t.Fatal("skip did not advance exactly once")
	}
}
