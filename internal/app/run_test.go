package app

import (
	"bytes"
	"errors"
	"testing"

	"keyklik/internal/input"
)

type stubClickPool struct{}

func (s *stubClickPool) Play() error { return nil }
func (s *stubClickPool) Close()      {}

type stubReader struct {
	err error
}

func (s *stubReader) ReadEvent() (input.Event, error) {
	return input.Event{}, s.err
}

func (s *stubReader) Close() error { return nil }

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
