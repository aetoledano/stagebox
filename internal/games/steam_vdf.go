package games

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Steam's text KeyValues files mix scalar values and nested objects. Keep them
// separate so a nested apps/depot entry cannot masquerade as a manifest field.
type steamVDF struct {
	values  map[string]string
	objects map[string]*steamVDF
}

func readSteamVDF(path string) (*steamVDF, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 8<<20 {
		return nil, fmt.Errorf("Steam text manifest is too large: %s", path)
	}
	return parseSteamVDF(string(data))
}

func parseSteamVDF(data string) (*steamVDF, error) {
	p := steamVDFParser{data: strings.TrimPrefix(data, "\ufeff")}
	return p.object(false, 0)
}

type steamVDFParser struct {
	data string
	pos  int
}

func (p *steamVDFParser) token() (string, bool, error) {
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			p.pos++
			continue
		}
		if strings.HasPrefix(p.data[p.pos:], "//") {
			for p.pos < len(p.data) && p.data[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
	if p.pos == len(p.data) {
		return "", false, io.EOF
	}
	c := p.data[p.pos]
	p.pos++
	if c == '{' || c == '}' {
		return string(c), true, nil
	}
	if c == '"' {
		var value strings.Builder
		for p.pos < len(p.data) {
			c = p.data[p.pos]
			p.pos++
			if c == '"' {
				return value.String(), false, nil
			}
			if c == '\\' && p.pos < len(p.data) {
				next := p.data[p.pos]
				// Preserve single Windows separators; decode the escaped separators
				// and quotes written by Steam's KeyValues serializer.
				if next == '\\' || next == '"' {
					c = next
					p.pos++
				}
			}
			value.WriteByte(c)
		}
		return "", false, fmt.Errorf("unterminated Steam KeyValues string")
	}
	start := p.pos - 1
	for p.pos < len(p.data) && !strings.ContainsRune(" \t\r\n{}", rune(p.data[p.pos])) {
		p.pos++
	}
	return p.data[start:p.pos], false, nil
}

func (p *steamVDFParser) object(nested bool, depth int) (*steamVDF, error) {
	if depth > 32 {
		return nil, fmt.Errorf("Steam KeyValues nesting is too deep")
	}
	result := &steamVDF{values: map[string]string{}, objects: map[string]*steamVDF{}}
	for {
		key, brace, err := p.token()
		if err == io.EOF {
			if nested {
				return nil, fmt.Errorf("missing Steam KeyValues closing brace")
			}
			return result, nil
		}
		if err != nil {
			return nil, err
		}
		if brace && key == "}" {
			if !nested {
				return nil, fmt.Errorf("unexpected Steam KeyValues closing brace")
			}
			return result, nil
		}
		if brace || key == "" {
			return nil, fmt.Errorf("invalid Steam KeyValues key")
		}
		value, brace, err := p.token()
		if err != nil {
			return nil, err
		}
		key = strings.ToLower(key)
		if brace && value == "{" {
			child, err := p.object(true, depth+1)
			if err != nil {
				return nil, err
			}
			result.objects[key] = child
		} else if brace {
			return nil, fmt.Errorf("missing Steam KeyValues value for %q", key)
		} else {
			result.values[key] = value
		}
	}
}
