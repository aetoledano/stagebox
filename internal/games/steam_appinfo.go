package games

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

type steamAppInfo struct {
	appType           string
	controllerSupport string
}

func steamControlMode(support string) int {
	switch strings.ToLower(strings.TrimSpace(support)) {
	case "full":
		return 0 // Fully playable from the couch without keyboard/mouse.
	case "partial":
		return 2 // Some menus/launchers still need keyboard/mouse.
	default:
		return 1
	}
}

// Read only installed apps from Steam's offline metadata. Versions 39/40 use
// inline keys; version 41 uses a shared key table. Unknown formats fall back to
// manifest discovery so a Steam cache update cannot hide the whole library.
// Format reference: https://github.com/xPaw/SteamAppInfo
func readSteamAppInfo(path string, wanted map[uint32]bool) (map[uint32]steamAppInfo, error) {
	result := map[uint32]steamAppInfo{}
	f, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer f.Close()
	var header [8]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return result, err
	}
	magic := binary.LittleEndian.Uint32(header[:4])
	if magic != 0x07564427 && magic != 0x07564428 && magic != 0x07564429 {
		return result, fmt.Errorf("unsupported Steam appinfo format: %#x", magic)
	}
	var keys []string
	if magic == 0x07564429 {
		var offset uint64
		if err := binary.Read(f, binary.LittleEndian, &offset); err != nil {
			return result, err
		}
		stat, err := f.Stat()
		if err != nil {
			return result, err
		}
		if offset < 16 || offset > uint64(stat.Size()) {
			return result, fmt.Errorf("invalid Steam appinfo string table offset")
		}
		if _, err := f.Seek(int64(offset), io.SeekStart); err != nil {
			return result, err
		}
		r := bufio.NewReader(io.LimitReader(f, 32<<20))
		var count uint32
		if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
			return result, err
		}
		if count > 500000 {
			return result, fmt.Errorf("Steam appinfo string table is too large")
		}
		for i := uint32(0); i < count; i++ {
			key, err := r.ReadString(0)
			if err != nil {
				return result, err
			}
			keys = append(keys, strings.TrimSuffix(key, "\x00"))
		}
		if _, err := f.Seek(16, io.SeekStart); err != nil {
			return result, err
		}
	}
	metadataSize := uint32(40)
	if magic >= 0x07564428 {
		metadataSize = 60
	}
	for {
		var appID uint32
		if err := binary.Read(f, binary.LittleEndian, &appID); err != nil {
			return result, err
		}
		if appID == 0 {
			return result, nil
		}
		var size uint32
		if err := binary.Read(f, binary.LittleEndian, &size); err != nil {
			return result, err
		}
		if size < metadataSize || size > 8<<20 {
			return result, fmt.Errorf("invalid Steam appinfo entry size")
		}
		if !wanted[appID] {
			if _, err := f.Seek(int64(size), io.SeekCurrent); err != nil {
				return result, err
			}
			continue
		}
		if _, err := f.Seek(int64(metadataSize), io.SeekCurrent); err != nil {
			return result, err
		}
		data := make([]byte, size-metadataSize)
		if _, err := io.ReadFull(f, data); err != nil {
			return result, err
		}
		p := steamBinaryVDF{data: data, keys: keys}
		vdf, err := p.object(0)
		if err != nil {
			return result, fmt.Errorf("Steam appinfo appID=%d: %w", appID, err)
		}
		if app := vdf.objects["appinfo"]; app != nil {
			if common := app.objects["common"]; common != nil {
				result[appID] = steamAppInfo{appType: strings.ToLower(common.values["type"]), controllerSupport: strings.ToLower(common.values["controller_support"])}
			}
		}
	}
}

type steamBinaryVDF struct {
	data []byte
	keys []string
	pos  int
}

func (p *steamBinaryVDF) take(size int) ([]byte, error) {
	if size < 0 || size > len(p.data)-p.pos {
		return nil, io.ErrUnexpectedEOF
	}
	data := p.data[p.pos : p.pos+size]
	p.pos += size
	return data, nil
}

func (p *steamBinaryVDF) text() (string, error) {
	end := bytes.IndexByte(p.data[p.pos:], 0)
	if end < 0 {
		return "", io.ErrUnexpectedEOF
	}
	data, err := p.take(end + 1)
	return string(data[:end]), err
}

func (p *steamBinaryVDF) object(depth int) (*steamVDF, error) {
	if depth > 32 {
		return nil, fmt.Errorf("Steam binary metadata nesting is too deep")
	}
	result := &steamVDF{values: map[string]string{}, objects: map[string]*steamVDF{}}
	for {
		typeByte, err := p.take(1)
		if err != nil {
			return nil, err
		}
		kind := typeByte[0]
		if kind == 8 {
			return result, nil
		}
		var key string
		if p.keys == nil {
			key, err = p.text()
		} else {
			var index []byte
			index, err = p.take(4)
			if err == nil {
				n := binary.LittleEndian.Uint32(index)
				if uint64(n) >= uint64(len(p.keys)) {
					return nil, fmt.Errorf("invalid Steam metadata key index")
				}
				key = p.keys[n]
			}
		}
		if err != nil {
			return nil, err
		}
		key = strings.ToLower(key)
		switch kind {
		case 0:
			result.objects[key], err = p.object(depth + 1)
		case 1:
			result.values[key], err = p.text()
		case 2, 3, 4, 6:
			_, err = p.take(4)
		case 7, 10:
			_, err = p.take(8)
		case 5:
			for {
				var pair []byte
				pair, err = p.take(2)
				if err != nil || (pair[0] == 0 && pair[1] == 0) {
					break
				}
			}
		default:
			return nil, fmt.Errorf("unsupported Steam metadata value type %d", kind)
		}
		if err != nil {
			return nil, err
		}
	}
}
