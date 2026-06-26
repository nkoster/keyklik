package input

import (
	"encoding/binary"
	"fmt"
	"os"
)

const (
	evKey = 0x01

	KeyUp   = 0
	KeyDown = 1

	keyLeftCtrl   = 29
	keyLeftShift  = 42
	keyCapsLock   = 58
	keyNumLock    = 69
	keyLeftAlt    = 56
	keyRightShift = 54
	keyRightCtrl  = 97
	keyRightAlt   = 100
)

type TimeVal struct {
	Sec  int64
	Usec int64
}

// Event matches Linux input_event on 64-bit Linux.
type Event struct {
	Time  TimeVal
	Type  uint16
	Code  uint16
	Value int32
}

type Reader struct {
	file *os.File
}

func Open(devicePath string) (*Reader, error) {
	f, err := os.Open(devicePath)
	if err != nil {
		return nil, fmt.Errorf("open keyboard device: %w", err)
	}
	return &Reader{file: f}, nil
}

func (r *Reader) Close() error {
	if r.file == nil {
		return nil
	}
	return r.file.Close()
}

func (r *Reader) ReadEvent() (Event, error) {
	var ev Event
	if err := binary.Read(r.file, binary.LittleEndian, &ev); err != nil {
		return Event{}, fmt.Errorf("read input event: %w", err)
	}
	return ev, nil
}

func IsKeyboardEvent(ev Event) bool {
	return ev.Type == evKey
}

func IsModifierKey(code uint16) bool {
	switch code {
	case keyLeftCtrl, keyRightCtrl,
		keyLeftShift, keyRightShift,
		keyLeftAlt, keyRightAlt,
		keyCapsLock, keyNumLock:
		return true
	default:
		return false
	}
}
