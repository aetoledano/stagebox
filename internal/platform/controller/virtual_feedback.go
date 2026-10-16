package controller

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

const virtualFeedbackVersion = 1
const virtualRumbleFreshness = 300 * time.Millisecond

type virtualRumble struct{ low, high uint16 }
type virtualRumbleSnapshot struct {
	levels []virtualRumble
	at     time.Time
}

// Fixed ordered batches match the source roster established by the handshake.
// Feedback never supplies device paths or transient SDL/XInput slot identities.
func decodeVirtualRumble(data []byte, controllers int) ([]virtualRumble, error) {
	if controllers < 1 || controllers > 4 || len(data) != controllers*4 {
		return nil, fmt.Errorf("invalid virtual rumble batch")
	}
	levels := make([]virtualRumble, controllers)
	for i := range levels {
		levels[i] = virtualRumble{binary.LittleEndian.Uint16(data[i*4:]), binary.LittleEndian.Uint16(data[i*4+2:])}
	}
	return levels, nil
}

func (s *virtualInputSession) readFeedback() {
	defer close(s.feedbackDone)
	defer s.cancel()
	data := make([]byte, len(s.sourcePaths)*4)
	for {
		err := s.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		if err == nil {
			_, err = io.ReadFull(s.conn, data)
		}
		if err != nil {
			if s.ctx.Err() == nil {
				s.setError(fmt.Errorf("virtual rumble connection lost: %w", err))
			}
			return
		}
		levels, err := decodeVirtualRumble(data, len(s.sourcePaths))
		if err != nil {
			s.setError(err)
			return
		}
		s.feedback.Store(&virtualRumbleSnapshot{levels: levels, at: time.Now()})
	}
}

func (s *virtualInputSession) rumbleLevels(now time.Time) []virtualRumble {
	if s == nil || !s.active.Load() || s.ctx.Err() != nil {
		return nil
	}
	frame := s.feedback.Load()
	if frame == nil || now.Sub(frame.at) > virtualRumbleFreshness {
		return nil
	}
	return frame.levels
}

// ReadSlice may have prefetched the first binary feedback frame after ready.
// Preserve it, without retaining a handshake-wide LimitReader at runtime.
type virtualFeedbackConnection struct {
	net.Conn
	reader *bufio.Reader
}

func (c *virtualFeedbackConnection) Read(data []byte) (int, error) { return c.reader.Read(data) }
