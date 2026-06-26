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
)

type TimeVal struct {
	Sec  int64
	Usec int64
}

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
