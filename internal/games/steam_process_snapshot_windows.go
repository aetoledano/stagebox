//go:build windows

package games

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type steamRecoveryEntry struct {
	processEntry
	sessionID uint32
	created   uint64
}

// The native snapshot supplies session IDs without opening each process.
// ProcessIdToSessionId cannot reliably query protected processes such as
// Registry, so recovery must filter sessions before doing handle inspection.
func steamRecoveryProcessEntries(sessionID uint32) ([]steamRecoveryEntry, error) {
	const maxSize = 32 << 20
	size := uint32(256 << 10)
	for attempt := 0; attempt < 8 && size <= maxSize; attempt++ {
		buffer := make([]byte, size)
		var returned uint32
		err := windows.NtQuerySystemInformation(windows.SystemProcessInformation, unsafe.Pointer(&buffer[0]), size, &returned)
		if err == nil {
			if returned == 0 || returned > size {
				return nil, errors.New("invalid native process snapshot length")
			}
			return parseSteamRecoveryProcessEntries(buffer[:returned], sessionID)
		}
		if !errors.Is(err, windows.STATUS_INFO_LENGTH_MISMATCH) && !errors.Is(err, windows.STATUS_BUFFER_TOO_SMALL) {
			return nil, fmt.Errorf("native process snapshot unavailable: %w", err)
		}
		// Leave room for processes created while the buffer is being resized.
		next := uint64(size) * 2
		if uint64(returned) > next {
			next = uint64(returned) + uint64(size)
		}
		if next > maxSize {
			break
		}
		size = uint32(next)
	}
	return nil, errors.New("native process snapshot exceeded size or retry limit")
}

func parseSteamRecoveryProcessEntries(buffer []byte, sessionID uint32) ([]steamRecoveryEntry, error) {
	headerSize := int(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{}))
	if len(buffer) < headerSize {
		return nil, errors.New("truncated native process snapshot")
	}
	base := uintptr(unsafe.Pointer(&buffer[0]))
	var entries []steamRecoveryEntry
	for offset := 0; ; {
		if offset > len(buffer)-headerSize || offset%int(unsafe.Alignof(windows.SYSTEM_PROCESS_INFORMATION{})) != 0 {
			return nil, errors.New("invalid native process snapshot offset")
		}
		info := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buffer[offset]))
		if info.SessionID == sessionID && info.UniqueProcessID != 0 && info.UniqueProcessID != 4 {
			if info.UniqueProcessID > uintptr(^uint32(0)) || info.CreateTime <= 0 {
				return nil, errors.New("invalid native process snapshot identity")
			}
			name := ""
			if length := uintptr(info.ImageName.Length); length != 0 {
				namePointer := uintptr(unsafe.Pointer(info.ImageName.Buffer))
				if length%2 != 0 || info.ImageName.Length > info.ImageName.MaximumLength || namePointer < base {
					return nil, errors.New("invalid native process snapshot name")
				}
				nameOffset := namePointer - base
				if nameOffset%2 != 0 || nameOffset > uintptr(len(buffer)) || length > uintptr(len(buffer))-nameOffset {
					return nil, errors.New("native process snapshot name outside buffer")
				}
				name = windows.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[int(nameOffset)])), int(length/2)))
			}
			entries = append(entries, steamRecoveryEntry{
				processEntry: processEntry{pid: uint32(info.UniqueProcessID), parentPID: uint32(info.InheritedFromUniqueProcessID), exeName: name},
				sessionID:    info.SessionID,
				created:      uint64(info.CreateTime),
			})
		}
		if info.NextEntryOffset == 0 {
			return entries, nil
		}
		if uint64(info.NextEntryOffset) < uint64(headerSize) || uint64(info.NextEntryOffset) > uint64(len(buffer)-offset) {
			return nil, errors.New("invalid native process snapshot next offset")
		}
		offset += int(info.NextEntryOffset)
	}
}

// Select native identities descended from handle-verified Steam processes.
// Enumeration sees unrelated processes, but recovery never opens them.
func steamRecoveryCandidates(tracker *steamTracker, entries []steamRecoveryEntry) []steamRecoveryEntry {
	byPID := make(map[uint32]steamRecoveryEntry, len(entries))
	children := make(map[uint32][]steamRecoveryEntry, len(entries))
	for _, entry := range entries {
		byPID[entry.pid] = entry
		children[entry.parentPID] = append(children[entry.parentPID], entry)
	}
	type parent struct {
		pid      uint32
		lifetime steamProcessLifetime
	}
	queue := make([]parent, 0, len(tracker.processes))
	for pid, process := range tracker.processes {
		queue = append(queue, parent{pid: pid, lifetime: process.lifetime})
	}
	seen := make(map[uint32]uint64, len(entries))
	var candidates []steamRecoveryEntry
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if current.lifetime.created == 0 || seen[current.pid] == current.lifetime.created {
			continue
		}
		seen[current.pid] = current.lifetime.created
		if entry, ok := byPID[current.pid]; ok && entry.created == current.lifetime.created {
			candidates = append(candidates, entry)
		}
		for _, child := range children[current.pid] {
			if current.lifetime.canParent(child.created) {
				queue = append(queue, parent{pid: child.pid, lifetime: steamProcessLifetime{created: child.created}})
			}
		}
	}
	return candidates
}
