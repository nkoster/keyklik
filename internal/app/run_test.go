package app

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"keyklik/internal/input"
)

type stubClickPool struct{}

func TestMain(m *testing.M) {
	origGetEffectiveUID := getEffectiveUID
	getEffectiveUID = func() int { return 0 }
	code := m.Run()
	getEffectiveUID = origGetEffectiveUID
	os.Exit(code)
}

func TestRun_DefaultMode_StartsBackgroundProcess(t *testing.T) {
	origStartDetachedProcess := startDetachedProcess
	origWritePIDFile := writePIDFile
	origDefaultKeyboardDevices := defaultKeyboardDevices
	defer func() {
		startDetachedProcess = origStartDetachedProcess
		writePIDFile = origWritePIDFile
		defaultKeyboardDevices = origDefaultKeyboardDevices
	}()

	var startedPath string
	var startedArgs []string
	var pidfilePath string
	startDetachedProcess = func(path string, args []string) (int, error) {
		startedPath = path
		startedArgs = append([]string(nil), args...)
		return 4321, nil
	}
	writePIDFile = func(path string, pid int) error {
		pidfilePath = path
		return nil
	}
	defaultKeyboardDevices = func() ([]string, error) {
		return []string{"/dev/input/by-path/platform-i8042-serio-0-event-kbd"}, nil
	}

	var stderr bytes.Buffer
	err := Run([]string{"keyklik"}, &bytes.Buffer{}, &stderr)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if startedPath != "keyklik" {
		t.Fatalf("expected started path %q, got %q", "keyklik", startedPath)
	}
	if len(startedArgs) != 1 || startedArgs[0] != "--foreground" {
		t.Fatalf("expected child args [--foreground], got %v", startedArgs)
	}
	if pidfilePath == "" {
		t.Fatal("expected default pidfile path to be used")
	}
	if !strings.Contains(stderr.String(), "listening on /dev/input/by-path/platform-i8042-serio-0-event-kbd") {
		t.Fatalf("expected startup listening log, got %q", stderr.String())
	}
}

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
	origDefaultKeyboardDevices := defaultKeyboardDevices
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevices = origDefaultKeyboardDevices
	}()

	stopErr := errors.New("stop loop")
	detectedDevice := "/dev/input/by-path/platform-i8042-serio-0-event-kbd"
	openedPath := ""

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		return &stubClickPool{}, nil
	}
	defaultKeyboardDevices = func() ([]string, error) {
		return []string{detectedDevice}, nil
	}
	openReader = func(devicePath string) (eventReader, error) {
		openedPath = devicePath
		return &stubReader{err: stopErr}, nil
	}

	err := Run([]string{"keyklik", "--foreground"}, &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected stop error, got %v", err)
	}
	if openedPath != detectedDevice {
		t.Fatalf("expected open path %q, got %q", detectedDevice, openedPath)
	}
}

func TestRun_NoDeviceArg_OpensAllDefaultKeyboardDevices(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevices := defaultKeyboardDevices
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevices = origDefaultKeyboardDevices
	}()

	stopErr := errors.New("stop loop")
	firstDevice := "/dev/input/by-path/platform-i8042-serio-0-event-kbd"
	secondDevice := "/dev/input/by-path/pci-0000:00:14.0-usb-0:13.4:1.0-event-kbd"
	openedPaths := make([]string, 0, 2)

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		return &stubClickPool{}, nil
	}
	defaultKeyboardDevices = func() ([]string, error) {
		return []string{firstDevice, secondDevice}, nil
	}
	openReader = func(devicePath string) (eventReader, error) {
		openedPaths = append(openedPaths, devicePath)
		return &stubReader{err: stopErr}, nil
	}

	err := Run([]string{"keyklik", "--foreground"}, &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected stop error, got %v", err)
	}
	if len(openedPaths) != 2 {
		t.Fatalf("expected 2 opened paths, got %d (%v)", len(openedPaths), openedPaths)
	}
	if openedPaths[0] != firstDevice {
		t.Fatalf("expected first open path %q, got %q", firstDevice, openedPaths[0])
	}
	if openedPaths[1] != secondDevice {
		t.Fatalf("expected second open path %q, got %q", secondDevice, openedPaths[1])
	}
}

func TestRun_DefaultBackground_StartsDetachedProcessAndReturns(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevices := defaultKeyboardDevices
	origStartDetachedProcess := startDetachedProcess
	origWritePIDFile := writePIDFile
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevices = origDefaultKeyboardDevices
		startDetachedProcess = origStartDetachedProcess
		writePIDFile = origWritePIDFile
	}()

	var startedPath string
	var startedArgs []string
	var wrotePIDFile string
	var wrotePID int

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		t.Fatal("newClickPool should not be called in background mode")
		return nil, nil
	}
	defaultKeyboardDevices = func() ([]string, error) {
		return []string{"/dev/input/by-path/platform-i8042-serio-0-event-kbd"}, nil
	}
	openReader = func(devicePath string) (eventReader, error) {
		t.Fatal("openReader should not be called in background mode")
		return nil, nil
	}
	startDetachedProcess = func(path string, args []string) (int, error) {
		startedPath = path
		startedArgs = append([]string(nil), args...)
		return 1234, nil
	}
	writePIDFile = func(path string, pid int) error {
		wrotePIDFile = path
		wrotePID = pid
		return nil
	}

	var stderr bytes.Buffer
	err := Run([]string{"keyklik", "/dev/input/event9", "--volume", "0.20"}, &bytes.Buffer{}, &stderr)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if startedPath != "keyklik" {
		t.Fatalf("expected started path %q, got %q", "keyklik", startedPath)
	}

	expectedArgs := []string{"/dev/input/event9", "--volume", "0.20", "--foreground"}
	if len(startedArgs) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d (%v)", len(expectedArgs), len(startedArgs), startedArgs)
	}
	for i := range expectedArgs {
		if startedArgs[i] != expectedArgs[i] {
			t.Fatalf("expected arg[%d] = %q, got %q", i, expectedArgs[i], startedArgs[i])
		}
	}

	if wrotePIDFile == "" {
		t.Fatal("expected default pidfile path to be used")
	}
	if wrotePID != 1234 {
		t.Fatalf("expected pid %d, got %d", 1234, wrotePID)
	}

	if !strings.Contains(stderr.String(), "started in background with pid=1234") {
		t.Fatalf("expected background start log, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "pid written to ") {
		t.Fatalf("expected pidfile log, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "listening on /dev/input/event9") {
		t.Fatalf("expected startup listening log, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "click config: regular volume 0.20") {
		t.Fatalf("expected startup click config log, got %q", stderr.String())
	}
}

func TestRun_DefaultBackground_WithPIDFile_WritesPID(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevices := defaultKeyboardDevices
	origStartDetachedProcess := startDetachedProcess
	origWritePIDFile := writePIDFile
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevices = origDefaultKeyboardDevices
		startDetachedProcess = origStartDetachedProcess
		writePIDFile = origWritePIDFile
	}()

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		t.Fatal("newClickPool should not be called in background mode")
		return nil, nil
	}
	defaultKeyboardDevices = func() ([]string, error) {
		return []string{"/dev/input/by-path/platform-i8042-serio-0-event-kbd"}, nil
	}
	openReader = func(devicePath string) (eventReader, error) {
		t.Fatal("openReader should not be called in background mode")
		return nil, nil
	}

	var gotPIDFile string
	var gotPID int
	startDetachedProcess = func(path string, args []string) (int, error) {
		return 2222, nil
	}
	writePIDFile = func(path string, pid int) error {
		gotPIDFile = path
		gotPID = pid
		return nil
	}

	var stderr bytes.Buffer
	err := Run([]string{"keyklik", "--pidfile", "/tmp/keyklik.pid"}, &bytes.Buffer{}, &stderr)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if gotPIDFile != "/tmp/keyklik.pid" {
		t.Fatalf("expected pidfile %q, got %q", "/tmp/keyklik.pid", gotPIDFile)
	}
	if gotPID != 2222 {
		t.Fatalf("expected pid %d, got %d", 2222, gotPID)
	}
	if !strings.Contains(stderr.String(), "pid written to /tmp/keyklik.pid") {
		t.Fatalf("expected pidfile log, got %q", stderr.String())
	}
}

func TestRun_Stop_WithPIDFile_SendsSignalAndRemovesPIDFile(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevices := defaultKeyboardDevices
	origReadPIDFile := readPIDFile
	origSendSignal := sendSignal
	origRemoveFile := removeFile
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevices = origDefaultKeyboardDevices
		readPIDFile = origReadPIDFile
		sendSignal = origSendSignal
		removeFile = origRemoveFile
	}()

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		t.Fatal("newClickPool should not be called in stop mode")
		return nil, nil
	}
	defaultKeyboardDevices = func() ([]string, error) {
		t.Fatal("defaultKeyboardDevices should not be called in stop mode")
		return nil, nil
	}
	openReader = func(devicePath string) (eventReader, error) {
		t.Fatal("openReader should not be called in stop mode")
		return nil, nil
	}

	var gotReadPath string
	var gotSignalPID int
	var gotSignal syscall.Signal
	var gotRemovePath string
	readPIDFile = func(path string) (int, error) {
		gotReadPath = path
		return 3456, nil
	}
	sendSignal = func(pid int, signal syscall.Signal) error {
		gotSignalPID = pid
		gotSignal = signal
		return nil
	}
	removeFile = func(path string) error {
		gotRemovePath = path
		return nil
	}

	var stderr bytes.Buffer
	err := Run([]string{"keyklik", "--stop", "--pidfile", "/tmp/keyklik.pid"}, &bytes.Buffer{}, &stderr)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if gotReadPath != "/tmp/keyklik.pid" {
		t.Fatalf("expected read path %q, got %q", "/tmp/keyklik.pid", gotReadPath)
	}
	if gotSignalPID != 3456 {
		t.Fatalf("expected signal pid %d, got %d", 3456, gotSignalPID)
	}
	if gotSignal != syscall.SIGTERM {
		t.Fatalf("expected signal %v, got %v", syscall.SIGTERM, gotSignal)
	}
	if gotRemovePath != "/tmp/keyklik.pid" {
		t.Fatalf("expected remove path %q, got %q", "/tmp/keyklik.pid", gotRemovePath)
	}
	if !strings.Contains(stderr.String(), "stopped process pid=3456") {
		t.Fatalf("expected stop log, got %q", stderr.String())
	}
}

func TestRun_StopWithoutPIDFile_UsesDefaultPIDFile(t *testing.T) {
	origReadPIDFile := readPIDFile
	origSendSignal := sendSignal
	origRemoveFile := removeFile
	defer func() {
		readPIDFile = origReadPIDFile
		sendSignal = origSendSignal
		removeFile = origRemoveFile
	}()

	var gotReadPath string
	readPIDFile = func(path string) (int, error) {
		gotReadPath = path
		return 9999, nil
	}
	sendSignal = func(pid int, signal syscall.Signal) error { return nil }
	removeFile = func(path string) error { return nil }

	err := Run([]string{"keyklik", "--stop"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if gotReadPath == "" {
		t.Fatal("expected default pidfile path to be used")
	}
}

func TestRun_StartWithoutRoot_ReturnsPrivilegeError(t *testing.T) {
	origGetEffectiveUID := getEffectiveUID
	origDefaultKeyboardDevices := defaultKeyboardDevices
	defer func() {
		getEffectiveUID = origGetEffectiveUID
		defaultKeyboardDevices = origDefaultKeyboardDevices
	}()

	getEffectiveUID = func() int { return 1000 }
	defaultKeyboardDevices = func() ([]string, error) {
		t.Fatal("defaultKeyboardDevices should not be called when privileges are insufficient")
		return nil, nil
	}

	err := Run([]string{"keyklik", "--foreground"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected privilege error, got nil")
	}
	if !strings.Contains(err.Error(), "insufficient privileges to start keyklik") {
		t.Fatalf("expected start privilege error, got %v", err)
	}
}

func TestRun_StopWithoutRoot_ReturnsPrivilegeError(t *testing.T) {
	origGetEffectiveUID := getEffectiveUID
	origReadPIDFile := readPIDFile
	defer func() {
		getEffectiveUID = origGetEffectiveUID
		readPIDFile = origReadPIDFile
	}()

	getEffectiveUID = func() int { return 1000 }
	readPIDFile = func(path string) (int, error) {
		t.Fatal("readPIDFile should not be called when privileges are insufficient")
		return 0, nil
	}

	err := Run([]string{"keyklik", "--stop"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected privilege error, got nil")
	}
	if !strings.Contains(err.Error(), "insufficient privileges to stop keyklik") {
		t.Fatalf("expected stop privilege error, got %v", err)
	}
}

func TestRun_ExplicitDeviceArg_SkipsDefaultDetection(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevices := defaultKeyboardDevices
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevices = origDefaultKeyboardDevices
	}()

	stopErr := errors.New("stop loop")
	explicitDevice := "/dev/input/event9"
	openedPath := ""
	defaultCalled := false

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		return &stubClickPool{}, nil
	}
	defaultKeyboardDevices = func() ([]string, error) {
		defaultCalled = true
		return nil, errors.New("should not be called")
	}
	openReader = func(devicePath string) (eventReader, error) {
		openedPath = devicePath
		return &stubReader{err: stopErr}, nil
	}

	err := Run([]string{"keyklik", explicitDevice, "--foreground"}, &bytes.Buffer{}, &bytes.Buffer{})
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

func TestRun_DefaultKeyboardDevicesError_IsReturned(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevices := defaultKeyboardDevices
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevices = origDefaultKeyboardDevices
	}()

	detectErr := errors.New("detect failed")

	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		return &stubClickPool{}, nil
	}
	defaultKeyboardDevices = func() ([]string, error) {
		return nil, detectErr
	}
	openReader = func(devicePath string) (eventReader, error) {
		t.Fatalf("openReader should not be called, got %q", devicePath)
		return nil, nil
	}

	err := Run([]string{"keyklik", "--foreground"}, &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, detectErr) {
		t.Fatalf("expected detect error, got %v", err)
	}
}

func TestRun_ModifierKey_UsesModifierClickPool(t *testing.T) {
	origNewClickPool := newClickPool
	origOpenReader := openReader
	origDefaultKeyboardDevices := defaultKeyboardDevices
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevices = origDefaultKeyboardDevices
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
	defaultKeyboardDevices = func() ([]string, error) {
		return []string{"/dev/input/by-path/platform-i8042-serio-0-event-kbd"}, nil
	}
	openReader = func(devicePath string) (eventReader, error) {
		return &sequenceReader{
			events: []input.Event{{Type: 0x01, Code: 42, Value: input.KeyDown}},
			err:    stopErr,
		}, nil
	}

	err := Run([]string{"keyklik", "--foreground", "--modifier-volume", "0.50", "--modifier-pitch", "2"}, &bytes.Buffer{}, &bytes.Buffer{})
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
	origDefaultKeyboardDevices := defaultKeyboardDevices
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevices = origDefaultKeyboardDevices
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
	defaultKeyboardDevices = func() ([]string, error) {
		return []string{"/dev/input/by-path/platform-i8042-serio-0-event-kbd"}, nil
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

	err := Run([]string{"keyklik", "--foreground"}, &bytes.Buffer{}, &bytes.Buffer{})
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
	origDefaultKeyboardDevices := defaultKeyboardDevices
	defer func() {
		newClickPool = origNewClickPool
		openReader = origOpenReader
		defaultKeyboardDevices = origDefaultKeyboardDevices
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
	defaultKeyboardDevices = func() ([]string, error) {
		return []string{"/dev/input/by-path/platform-i8042-serio-0-event-kbd"}, nil
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

	err := Run([]string{"keyklik", "--foreground"}, &bytes.Buffer{}, &bytes.Buffer{})
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
