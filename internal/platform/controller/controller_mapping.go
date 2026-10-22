package controller

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Raw input is captured before SDL's existing gamepad mapping is applied.
type mappingInput struct {
	axes    []int16
	buttons []bool
	hats    []uint8
}

type ControllerMappingState struct {
	ID       string            `json:"id"`
	Step     int               `json:"step"`
	Total    int               `json:"total"`
	Control  string            `json:"control"`
	Phase    string            `json:"phase"`
	Binding  string            `json:"binding"`
	Message  string            `json:"message"`
	CanSkip  bool              `json:"canSkip"`
	Bindings map[string]string `json:"bindings"`
	Command  string            `json:"command"`
}

type mappingStep struct {
	control, kind string
	optional      bool
}

var mappingSteps = []mappingStep{
	{"a", "button", false}, {"b", "button", false}, {"x", "button", false}, {"y", "button", false},
	{"dpup", "button", false}, {"dpdown", "button", false}, {"dpleft", "button", false}, {"dpright", "button", false},
	{"leftshoulder", "button", true}, {"rightshoulder", "button", true},
	{"back", "button", false}, {"start", "button", false}, {"guide", "button", true},
	{"leftstick", "button", true}, {"rightstick", "button", true},
	{"leftx", "stick", true}, {"lefty", "stick", true}, {"rightx", "stick", true}, {"righty", "stick", true},
	{"lefttrigger", "trigger", true}, {"righttrigger", "trigger", true},
}

type controllerMappingCapture struct {
	id, guid, name            string
	instance                  uint32
	revision                  uint64
	step                      int
	phase, candidate, message string
	since                     time.Time
	baseline                  mappingInput
	bindings                  map[string]string
	// Negative and positive raw endpoints verify stick polarity and full travel.
	firstAxis  int
	firstValue int16
	calibrated bool
	skipSince  time.Time
	command    string
	reviewHeld bool
}

func newMappingCapture(id, guid, name string, instance uint32, revision uint64) *controllerMappingCapture {
	return &controllerMappingCapture{id: id, guid: guid, name: name, instance: instance, revision: revision, phase: "center", bindings: map[string]string{}, firstAxis: -1}
}

func (c *controllerMappingCapture) state() ControllerMappingState {
	s := ControllerMappingState{ID: c.id, Step: c.step, Total: len(mappingSteps), Phase: c.phase, Binding: c.candidate, Message: c.message, Bindings: map[string]string{}, Command: c.command}
	for k, v := range c.bindings {
		s.Bindings[k] = v
	}
	if c.step < len(mappingSteps) {
		s.Control = mappingSteps[c.step].control
		s.CanSkip = mappingSteps[c.step].optional
	}
	return s
}

func copyMappingInput(in mappingInput) mappingInput {
	return mappingInput{axes: append([]int16(nil), in.axes...), buttons: append([]bool(nil), in.buttons...), hats: append([]uint8(nil), in.hats...)}
}

func mappingAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func inputReleased(in, base mappingInput) bool {
	if len(in.axes) != len(base.axes) || len(in.buttons) != len(base.buttons) || len(in.hats) != len(base.hats) {
		return false
	}
	for _, v := range in.buttons {
		if v {
			return false
		}
	}
	for _, v := range in.hats {
		if v != 0 {
			return false
		}
	}
	for i, v := range in.axes {
		if mappingAbs(int(v)-int(base.axes[i])) > 6000 {
			return false
		}
	}
	return true
}

func (c *controllerMappingCapture) axisUsed(index int) bool {
	token := "a" + strconv.Itoa(index)
	for _, v := range c.bindings {
		if strings.TrimSuffix(v, "~") == token {
			return true
		}
	}
	return false
}

func (c *controllerMappingCapture) candidateFor(in mappingInput) string {
	kind := mappingSteps[c.step].kind
	// Prefer a moving analog trigger over its simultaneously exposed button.
	if kind == "stick" || kind == "trigger" {
		for i, v := range in.axes {
			if i >= len(c.baseline.axes) || c.axisUsed(i) {
				continue
			}
			base := int(c.baseline.axes[i])
			delta := int(v) - base
			if mappingAbs(delta) < 20000 {
				continue
			}
			if kind == "stick" {
				if mappingAbs(base) > 8000 {
					continue
				}
				used := false
				for _, binding := range c.bindings {
					if strings.TrimSuffix(strings.TrimLeft(binding, "+-"), "~") == fmt.Sprintf("a%d", i) {
						used = true
					}
				}
				if used {
					continue
				}
				return fmt.Sprintf("a%d", i)
			}
			if base < -20000 && v > 12000 {
				return fmt.Sprintf("a%d", i)
			}
			if base > 20000 && v < -12000 {
				return fmt.Sprintf("a%d~", i)
			}
			if mappingAbs(base) < 8000 {
				token := fmt.Sprintf("+a%d", i)
				if delta < 0 {
					token = fmt.Sprintf("-a%d", i)
				}
				used := false
				for _, binding := range c.bindings {
					if binding == token {
						used = true
					}
				}
				if !used {
					return token
				}
			}
		}
	}
	if kind == "stick" {
		return ""
	}
	for i, down := range in.buttons {
		if !down {
			continue
		}
		token := fmt.Sprintf("b%d", i)
		used := false
		for _, v := range c.bindings {
			if v == token {
				used = true
			}
		}
		if !used {
			return token
		}
	}
	if kind == "button" {
		for i, v := range in.hats {
			if v != 1 && v != 2 && v != 4 && v != 8 {
				continue
			}
			token := fmt.Sprintf("h%d.%d", i, v)
			used := false
			for _, binding := range c.bindings {
				if binding == token {
					used = true
				}
			}
			if !used {
				return token
			}
		}
		// Some D-pads are axes rather than hats/buttons.
		if strings.HasPrefix(mappingSteps[c.step].control, "dp") {
			for i, v := range in.axes {
				if i >= len(c.baseline.axes) || c.axisUsed(i) {
					continue
				}
				delta := int(v) - int(c.baseline.axes[i])
				if mappingAbs(delta) < 20000 {
					continue
				}
				sign := "+"
				if delta < 0 {
					sign = "-"
				}
				token := fmt.Sprintf("%sa%d", sign, i)
				used := false
				for _, binding := range c.bindings {
					if binding == token {
						used = true
					}
				}
				if !used {
					return token
				}
			}
		}
	}
	return ""
}

func mappingRowIdentity(row string) (string, string, bool) {
	fields := strings.Split(strings.TrimSpace(strings.TrimPrefix(row, "\ufeff")), ",")
	if len(fields) < 3 || !validMappingGUID(strings.ToLower(fields[0])) {
		return "", "", false
	}
	guid := strings.ToLower(fields[0])
	crc := guid[6:8] + guid[4:6]
	windows := false
	for _, field := range fields[2:] {
		if field == "platform:Windows" {
			windows = true
		}
		if strings.HasPrefix(field, "crc:") {
			if v, err := strconv.ParseUint(strings.TrimPrefix(field, "crc:"), 16, 16); err == nil {
				crc = fmt.Sprintf("%04x", v)
			}
		}
	}
	return guid[:4] + "0000" + guid[8:], crc, windows
}

func mappingOverrideMatches(row, guid string) bool {
	if !validMappingGUID(guid) {
		return false
	}
	key, crc, windows := mappingRowIdentity(row)
	return windows && key == guid[:4]+"0000"+guid[8:] && (crc == "0000" || crc == guid[6:8]+guid[4:6])
}

func upsertMappingOverride(data []byte, guid, row string) []byte {
	key, crc, _ := mappingRowIdentity(row)
	lines := strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n")
	out := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		k, c, w := mappingRowIdentity(line)
		if w && k == key && c == crc {
			continue
		}
		out = append(out, line)
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	out = append(out, row, "")
	return []byte(strings.Join(out, "\n"))
}

func sameMappingBindings(a, b string) bool {
	read := func(row string) map[string]string {
		result := map[string]string{}
		fields := strings.Split(row, ",")
		if len(fields) > 2 {
			for _, field := range fields[2:] {
				k, v, ok := strings.Cut(field, ":")
				if ok {
					result[k] = v
				}
			}
		}
		return result
	}
	x, y := read(a), read(b)
	for _, step := range mappingSteps {
		if x[step.control] != y[step.control] {
			return false
		}
	}
	return true
}

// A stable press and a neutral interval separate every capture. Stick steps
// also require the opposite endpoint before accepting a full axis mapping.
func (c *controllerMappingCapture) update(in mappingInput, now time.Time) {
	if c.phase == "error" {
		return
	}
	if c.phase == "ready" {
		a, b := mappingButtonHeld(in, c.bindings["a"]), mappingButtonHeld(in, c.bindings["b"])
		if !c.reviewHeld {
			if a {
				c.command = "save"
			} else if b {
				c.command = "previous"
			}
		}
		c.reviewHeld = a || b
		return
	}
	if c.calibrated && mappingSteps[c.step].optional && c.phase != "release" && c.phase != "release-opposite" && mappingButtonHeld(in, c.bindings["a"]) {
		if c.skipSince.IsZero() {
			c.skipSince = now
		}
		if now.Sub(c.skipSince) >= 1500*time.Millisecond {
			_ = c.skip()
			c.skipSince = time.Time{}
		}
		return
	}
	c.skipSince = time.Time{}
	if !c.calibrated {
		if c.since.IsZero() {
			c.baseline = copyMappingInput(in)
			c.since = now
		}
		if !inputReleased(in, c.baseline) {
			c.baseline = copyMappingInput(in)
			c.since = now
			return
		}
		if now.Sub(c.since) < 700*time.Millisecond {
			return
		}
		c.calibrated = true
		c.phase = "press"
		c.since = time.Time{}
		return
	}
	if c.phase == "release" || c.phase == "release-opposite" {
		if !inputReleased(in, c.baseline) {
			c.since = time.Time{}
			return
		}
		if c.since.IsZero() {
			c.since = now
		}
		if now.Sub(c.since) < 180*time.Millisecond {
			return
		}
		if c.phase == "release-opposite" {
			c.phase = "opposite"
		} else {
			c.step++
			c.phase = "press"
			c.firstAxis = -1
			if c.step == len(mappingSteps) {
				c.phase = "ready"
			}
		}
		c.candidate = ""
		c.since = time.Time{}
		c.message = ""
		return
	}
	if c.phase == "opposite" {
		if c.firstAxis >= len(in.axes) {
			return
		}
		value := in.axes[c.firstAxis]
		if mappingAbs(int(value)) < 20000 || (int(value)*int(c.firstValue) >= 0) {
			c.since = time.Time{}
			return
		}
		if c.since.IsZero() {
			c.since = now
		}
		if now.Sub(c.since) < 100*time.Millisecond {
			return
		}
		binding := fmt.Sprintf("a%d", c.firstAxis)
		if c.firstValue < 0 {
			binding += "~"
		}
		c.bindings[mappingSteps[c.step].control] = binding
		c.candidate = binding
		c.phase = "release"
		c.since = time.Time{}
		return
	}
	token := c.candidateFor(in)
	if token == "" {
		c.candidate = ""
		c.since = time.Time{}
		return
	}
	if token != c.candidate {
		c.candidate = token
		c.since = now
		return
	}
	if now.Sub(c.since) < 100*time.Millisecond {
		return
	}
	if mappingSteps[c.step].kind == "stick" {
		index, _ := strconv.Atoi(strings.TrimPrefix(token, "a"))
		c.firstAxis = index
		c.firstValue = in.axes[index]
		c.phase = "release-opposite"
	} else {
		c.bindings[mappingSteps[c.step].control] = token
		c.phase = "release"
	}
	c.since = time.Time{}
}

func mappingButtonHeld(in mappingInput, token string) bool {
	if strings.HasPrefix(token, "b") {
		i, err := strconv.Atoi(token[1:])
		return err == nil && i >= 0 && i < len(in.buttons) && in.buttons[i]
	}
	if strings.HasPrefix(token, "h") {
		var i, v int
		if n, _ := fmt.Sscanf(token, "h%d.%d", &i, &v); n == 2 && i >= 0 && i < len(in.hats) {
			return int(in.hats[i]) == v
		}
	}
	return false
}

func (c *controllerMappingCapture) skip() error {
	if c.step >= len(mappingSteps) || !mappingSteps[c.step].optional || !c.calibrated {
		return fmt.Errorf("this control cannot be skipped")
	}
	delete(c.bindings, mappingSteps[c.step].control)
	c.firstAxis = -1
	c.phase = "release"
	c.since = time.Time{}
	return nil
}

func (c *controllerMappingCapture) previous() {
	if c.step > 0 {
		c.step--
	}
	for i := c.step; i < len(mappingSteps); i++ {
		delete(c.bindings, mappingSteps[i].control)
	}
	c.phase = "center"
	c.calibrated = false
	c.candidate = ""
	c.since = time.Time{}
	c.firstAxis = -1
}

func validMappingGUID(guid string) bool {
	decoded, err := hex.DecodeString(guid)
	return err == nil && len(decoded) == 16 && guid == strings.ToLower(guid) && guid != strings.Repeat("0", 32)
}

func (c *controllerMappingCapture) row() (string, error) {
	if c.phase != "ready" || !validMappingGUID(c.guid) {
		return "", fmt.Errorf("finish mapping the controller before saving")
	}
	name := strings.Map(func(r rune) rune {
		if r == ',' || r < ' ' || r == 127 {
			return ' '
		}
		return r
	}, c.name)
	if len(name) > 200 {
		name = "StageBox custom controller"
	}
	if strings.TrimSpace(name) == "" {
		name = "StageBox custom controller"
	}
	fields := []string{c.guid, name}
	for _, step := range mappingSteps {
		if binding := c.bindings[step.control]; binding != "" {
			fields = append(fields, step.control+":"+binding)
		} else if !step.optional {
			return "", fmt.Errorf("required control %s is missing", step.control)
		}
	}
	return strings.Join(append(fields, "platform:Windows", ""), ","), nil
}
