package app

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"keyklik/internal/input"
)

type stubClickPool struct{}

func (s *stubClickPool) Play() error { return nil }
func (s *stubClickPool) Close()      {}

type countingClickPool struct {
	plays int
}

func (s *countingClickPool) Play() error {
	s.plays++
	return nil
}

func (s *countingClickPool) Close() {}

type stubReader struct {
	err error
}

func (s *stubReader) ReadEvent() (input.Event, error) {
	return input.Event{}, s.err
}

func (s *stubReader) Close() error { return nil }

type sequenceReader struct {
	events []input.Event
	next   int
	err    error
}

func (s *sequenceReader) ReadEvent() (input.Event, error) {
	if s.next < len(s.events) {
		ev := s.events[s.next]
		s.next++
		return ev, nil
	}
	return input.Event{}, s.err
}

func (s *sequenceReader) Close() error { return nil }

type timedEvent struct {
	ev    input.Event
	delay time.Duration
}

type timedSequenceReader struct {
	events []timedEvent
	next   int
	err    error
}

func (s *timedSequenceReader) ReadEvent() (input.Event, error) {
	if s.next < len(s.events) {
		item := s.events[s.next]
		s.next++
		if item.delay > 0 {
			time.Sleep(item.delay)
		}
		return item.ev, nil
	}
	return input.Event{}, s.err
}

func (s *timedSequenceReader) Close() error { return nil }

func TestRun_NoDeviceArg_UsesDefaultKeyboardDevice(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevice := defaultKeyboardDevice
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevice = origDefaultKeyboardDevice
	}()

	stopErr := errors.New("stop loop")
	detectedDevice := "/dev/input/by-path/platform-i8042-serio-0-event-kbd"
	openedPath := ""

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		return &stubClickPool{}, nil
	}
	defaultKeyboardDevice = func() (string, error) {
		return detectedDevice, nil
	}
	openReader = func(devicePath string) (eventReader, error) {
		openedPath = devicePath
		return &stubReader{err: stopErr}, nil
	}

	err := Run([]string{"keyklik"}, &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected stop error, got %v", err)
	}
	if openedPath != detectedDevice {
		t.Fatalf("expected open path %q, got %q", detectedDevice, openedPath)
	}
}

func TestRun_ExplicitDeviceArg_SkipsDefaultDetection(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevice := defaultKeyboardDevice
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevice = origDefaultKeyboardDevice
	}()

	stopErr := errors.New("stop loop")
	explicitDevice := "/dev/input/event9"
	openedPath := ""
	defaultCalled := false

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		return &stubClickPool{}, nil
	}
	defaultKeyboardDevice = func() (string, error) {
		defaultCalled = true
		return "", errors.New("should not be called")
	}
	openReader = func(devicePath string) (eventReader, error) {
		openedPath = devicePath
		return &stubReader{err: stopErr}, nil
	}

	err := Run([]string{"keyklik", explicitDevice}, &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected stop error, got %v", err)
	}
	if defaultCalled {
		t.Fatal("expected default keyboard detection not to be called")
	}
	if openedPath != explicitDevice {
		t.Fatalf("expected open path %q, got %q", explicitDevice, openedPath)
	}
}

func TestRun_DefaultKeyboardDeviceError_IsReturned(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevice := defaultKeyboardDevice
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevice = origDefaultKeyboardDevice
	}()

	detectErr := errors.New("detect failed")

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		return &stubClickPool{}, nil
	}
	defaultKeyboardDevice = func() (string, error) {
		return "", detectErr
	}
	openReader = func(devicePath string) (eventReader, error) {
		t.Fatalf("openReader should not be called, got %q", devicePath)
		return nil, nil
	}

	err := Run([]string{"keyklik"}, &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, detectErr) {
		t.Fatalf("expected detect error, got %v", err)
	}
}

func TestRun_ModifierKey_UsesModifierClickPool(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevice := defaultKeyboardDevice
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevice = origDefaultKeyboardDevice
	}()

	stopErr := errors.New("stop loop")
	regularPool := &countingClickPool{}
	modifierPool := &countingClickPool{}
	poolCall := 0

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		poolCall++
		if poolCall == 1 {
			return regularPool, nil
		}
		return modifierPool, nil
	}
	defaultKeyboardDevice = func() (string, error) {
		return "/dev/input/by-path/platform-i8042-serio-0-event-kbd", nil
	}
	openReader = func(devicePath string) (eventReader, error) {
		return &sequenceReader{
			events: []input.Event{{Type: 0x01, Code: 42, Value: input.KeyDown}},
			err:    stopErr,
		}, nil
	}

	err := Run([]string{"keyklik", "--modifier-volume", "0.50", "--modifier-pitch", "2"}, &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected stop error, got %v", err)
	}
	if regularPool.plays != 0 {
		t.Fatalf("expected regular pool plays 0, got %d", regularPool.plays)
	}
	if modifierPool.plays != 1 {
		t.Fatalf("expected modifier pool plays 1, got %d", modifierPool.plays)
	}
}

func TestRun_KeyRepeatDoesNotPlayAgain(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevice := defaultKeyboardDevice
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevice = origDefaultKeyboardDevice
	}()

	stopErr := errors.New("stop loop")
	regularPool := &countingClickPool{}
	modifierPool := &countingClickPool{}
	poolCall := 0

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		poolCall++
		if poolCall == 1 {
			return regularPool, nil
		}
		return modifierPool, nil
	}
	defaultKeyboardDevice = func() (string, error) {
		return "/dev/input/by-path/platform-i8042-serio-0-event-kbd", nil
	}
	openReader = func(devicePath string) (eventReader, error) {
		return &sequenceReader{
			events: []input.Event{
				{Type: 0x01, Code: 30, Value: input.KeyDown},
				{Type: 0x01, Code: 30, Value: input.KeyDown},
			},
			err: stopErr,
		}, nil
	}

	err := Run([]string{"keyklik"}, &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected stop error, got %v", err)
	}
	if regularPool.plays != 1 {
		t.Fatalf("expected regular pool plays 1, got %d", regularPool.plays)
	}
	if modifierPool.plays != 0 {
		t.Fatalf("expected modifier pool plays 0, got %d", modifierPool.plays)
	}
}

func TestRun_KeyUpAllowsNewClick(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevice := defaultKeyboardDevice
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevice = origDefaultKeyboardDevice
	}()

	stopErr := errors.New("stop loop")
	regularPool := &countingClickPool{}
	modifierPool := &countingClickPool{}
	poolCall := 0

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		poolCall++
		if poolCall == 1 {
			return regularPool, nil
		}
		return modifierPool, nil
	}
	defaultKeyboardDevice = func() (string, error) {
		return "/dev/input/by-path/platform-i8042-serio-0-event-kbd", nil
	}
	openReader = func(devicePath string) (eventReader, error) {
		return &timedSequenceReader{
			events: []timedEvent{
				{ev: input.Event{Type: 0x01, Code: 30, Value: input.KeyDown}},
				{ev: input.Event{Type: 0x01, Code: 30, Value: input.KeyUp}},
				{ev: input.Event{Type: 0x01, Code: 30, Value: input.KeyDown}, delay: minClickGap + (2 * time.Millisecond)},
			},
			err: stopErr,
		}, nil
	}

	err := Run([]string{"keyklik"}, &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected stop error, got %v", err)
	}
	if regularPool.plays != 2 {
		t.Fatalf("expected regular pool plays 2, got %d", regularPool.plays)
	}
	if modifierPool.plays != 0 {
		t.Fatalf("expected modifier pool plays 0, got %d", modifierPool.plays)
	}
}
