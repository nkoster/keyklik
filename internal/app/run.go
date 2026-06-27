package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"keyklik/internal/audio"
	"keyklik/internal/config"
	"keyklik/internal/input"
	"keyklik/internal/util"
)

const (
	playerPoolSize = 4
	minClickGap    = 8 * time.Millisecond
)

type clickPool interface {
	Play() error
	Close()
}

type eventReader interface {
	ReadEvent() (input.Event, error)
	Close() error
}

var (
	newClickPool = func(sampleRate int, volume float64, pitchLevel int, poolSize int) (clickPool, error) {
		return audio.NewClickPool(sampleRate, volume, pitchLevel, poolSize)
	}
	getEffectiveUID = os.Geteuid
	openReader      = func(devicePath string) (eventReader, error) {
		return input.Open(devicePath)
	}
	defaultKeyboardDevices = config.DefaultKeyboardDevices
	writePIDFile           = func(path string, pid int) error {
		dir := filepath.Dir(path)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644)
	}
	readPIDFile = func(path string) (int, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || pid <= 0 {
			return 0, fmt.Errorf("invalid pid in %s", path)
		}
		return pid, nil
	}
	sendSignal = func(pid int, signal syscall.Signal) error {
		return syscall.Kill(pid, signal)
	}
	removeFile = func(path string) error {
		return os.Remove(path)
	}
	startDetachedProcess = func(path string, args []string) (int, error) {
		devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			return 0, err
		}
		defer util.IgnoreErr(devNull.Close)

		cmd := exec.Command(path, args...)
		cmd.Stdin = devNull
		cmd.Stdout = devNull
		cmd.Stderr = devNull
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

		if err := cmd.Start(); err != nil {
			return 0, err
		}

		return cmd.Process.Pid, nil
	}
)

func argsForBackgroundChild(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--foreground" {
			continue
		}
		filtered = append(filtered, arg)
	}
	filtered = append(filtered, "--foreground")
	return filtered
}

func defaultPIDFilePath() string {
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "keyklik.pid")
	}

	return filepath.Join(os.TempDir(), "keyklik.pid")
}

func programName(args []string) string {
	if len(args) == 0 || args[0] == "" {
		return "keyklik"
	}
	return args[0]
}

func logf(w io.Writer, format string, args ...any) {
	util.Ignore(fmt.Fprintf(w, format+"\n", args...))
}

func logStartupInfo(w io.Writer, cfg config.Config, keyboardDevices []string) {
	for _, devicePath := range keyboardDevices {
		logf(w, "listening on %s", devicePath)
	}
	logf(w, "click config: regular volume %.2f, regular pitch level %d, modifier volume %.2f, modifier pitch level %d", cfg.Volume, cfg.PitchLevel, cfg.ModifierVolume, cfg.ModifierPitch)
}

type sourcedEvent struct {
	devicePath string
	event      input.Event
	err        error
}

func openReaders(devicePaths []string) ([]eventReader, error) {
	readers := make([]eventReader, 0, len(devicePaths))
	for _, devicePath := range devicePaths {
		reader, err := openReader(devicePath)
		if err != nil {
			for _, openedReader := range readers {
				util.IgnoreErr(openedReader.Close)
			}
			return nil, err
		}
		readers = append(readers, reader)
	}

	return readers, nil
}

func readEventsFromAll(devicePaths []string, readers []eventReader) <-chan sourcedEvent {
	events := make(chan sourcedEvent)

	for idx, reader := range readers {
		devicePath := devicePaths[idx]
		go func(path string, r eventReader) {
			for {
				ev, err := r.ReadEvent()
				if err != nil {
					events <- sourcedEvent{devicePath: path, err: err}
					return
				}
				events <- sourcedEvent{devicePath: path, event: ev}
			}
		}(devicePath, reader)
	}

	return events
}

func sudoInvokingUserIDs(getenv func(string) string, euid int) (int, int, bool) {
	if euid != 0 {
		return 0, 0, false
	}

	uidRaw := getenv("SUDO_UID")
	gidRaw := getenv("SUDO_GID")
	if uidRaw == "" || gidRaw == "" {
		return 0, 0, false
	}

	uid, err := strconv.Atoi(uidRaw)
	if err != nil || uid <= 0 {
		return 0, 0, false
	}

	gid, err := strconv.Atoi(gidRaw)
	if err != nil || gid <= 0 {
		return 0, 0, false
	}

	return uid, gid, true
}

func maybeDropSudoPrivileges() (int, int, bool, error) {
	uid, gid, ok := sudoInvokingUserIDs(os.Getenv, os.Geteuid())
	if !ok {
		return 0, 0, false, nil
	}

	if err := syscall.Setgroups([]int{gid}); err != nil {
		return 0, 0, false, fmt.Errorf("drop privileges setgroups failed: %w", err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return 0, 0, false, fmt.Errorf("drop privileges setgid failed: %w", err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return 0, 0, false, fmt.Errorf("drop privileges setuid failed: %w", err)
	}

	runtimeDir := fmt.Sprintf("/run/user/%d", uid)
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		_ = os.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	}
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		_ = os.Setenv("USER", sudoUser)
		_ = os.Setenv("LOGNAME", sudoUser)
	}
	if u, err := user.LookupId(strconv.Itoa(uid)); err == nil && u.HomeDir != "" {
		if os.Getenv("HOME") == "" || os.Getenv("HOME") == "/root" {
			_ = os.Setenv("HOME", u.HomeDir)
		}
	}

	return uid, gid, true, nil
}

func requirePrivileges(stopMode bool) error {
	if getEffectiveUID() == 0 {
		return nil
	}

	action := "start"
	if stopMode {
		action = "stop"
	}

	return fmt.Errorf("insufficient privileges to %s keyklik: run as root or with sudo", action)
}

func Run(args []string, stdout io.Writer, stderr io.Writer) error {
	prog := programName(args)

	cfg, err := config.Parse(args)
	if err != nil {
		if errors.Is(err, config.ErrHelp) {
			util.Ignore(fmt.Fprint(stdout, config.Usage(prog)))
			return nil
		}

		util.Ignore(fmt.Fprint(stderr, config.Usage(prog)))
		return err
	}

	if err := requirePrivileges(cfg.Stop); err != nil {
		return err
	}

	shouldBackground := !cfg.Stop && !cfg.Foreground
	keyboardDevices := make([]string, 0, 1)

	if !cfg.Stop {
		if cfg.KeyboardDevice != "" {
			keyboardDevices = append(keyboardDevices, cfg.KeyboardDevice)
		} else {
			devices, err := defaultKeyboardDevices()
			if err != nil {
				return err
			}
			keyboardDevices = append(keyboardDevices, devices...)
		}
		cfg.KeyboardDevice = keyboardDevices[0]
	}

	if shouldBackground {
		pidFile := cfg.PIDFile
		if pidFile == "" {
			pidFile = defaultPIDFilePath()
		}

		if uid, gid, ok := sudoInvokingUserIDs(os.Getenv, os.Geteuid()); ok {
			logf(stderr, "sudo detected: dropped privileges to uid=%d gid=%d for audio compatibility", uid, gid)
		}

		logStartupInfo(stderr, cfg, keyboardDevices)

		childArgs := argsForBackgroundChild(args)
		if len(childArgs) == 0 {
			childArgs = []string{prog, "--foreground"}
		}

		pid, err := startDetachedProcess(childArgs[0], childArgs[1:])
		if err != nil {
			return fmt.Errorf("start background process: %w", err)
		}
		if err := writePIDFile(pidFile, pid); err != nil {
			return fmt.Errorf("write pidfile: %w", err)
		}

		logf(stderr, "started in background with pid=%d", pid)
		logf(stderr, "pid written to %s", pidFile)
		return nil
	}

	if cfg.Stop {
		pidFile := cfg.PIDFile
		if pidFile == "" {
			pidFile = defaultPIDFilePath()
		}

		pid, err := readPIDFile(pidFile)
		if err != nil {
			return fmt.Errorf("read pidfile: %w", err)
		}

		if err := sendSignal(pid, syscall.SIGTERM); err != nil {
			return fmt.Errorf("stop process %d: %w", pid, err)
		}

		if err := removeFile(pidFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove pidfile: %w", err)
		}

		logf(stderr, "stopped process pid=%d", pid)
		return nil
	}

	readers, err := openReaders(keyboardDevices)
	if err != nil {
		return err
	}
	for _, reader := range readers {
		defer util.IgnoreErr(reader.Close)
	}

	uid, gid, dropped, err := maybeDropSudoPrivileges()
	if err != nil {
		return err
	}
	if dropped {
		logf(stderr, "sudo detected: dropped privileges to uid=%d gid=%d for audio compatibility", uid, gid)
	}

	regularClickPool, err := newClickPool(config.SampleRate, cfg.Volume, cfg.PitchLevel, playerPoolSize)
	if err != nil {
		return err
	}
	defer regularClickPool.Close()

	modifierClickPool, err := newClickPool(config.SampleRate, cfg.ModifierVolume, cfg.ModifierPitch, playerPoolSize)
	if err != nil {
		return err
	}
	defer modifierClickPool.Close()

	logStartupInfo(stderr, cfg, keyboardDevices)

	pressedKeys := make(map[string]map[uint16]bool)
	lastClick := time.Time{}
	events := readEventsFromAll(keyboardDevices, readers)

	for {
		sourceEvent := <-events
		if sourceEvent.err != nil {
			return sourceEvent.err
		}
		ev := sourceEvent.event

		if !input.IsKeyboardEvent(ev) {
			continue
		}

		devicePressed, ok := pressedKeys[sourceEvent.devicePath]
		if !ok {
			devicePressed = make(map[uint16]bool)
			pressedKeys[sourceEvent.devicePath] = devicePressed
		}

		if ev.Value == input.KeyUp {
			delete(devicePressed, ev.Code)
			continue
		}

		if ev.Value != input.KeyDown {
			continue
		}

		if devicePressed[ev.Code] {
			continue
		}
		devicePressed[ev.Code] = true

		now := time.Now()
		if now.Sub(lastClick) < minClickGap {
			continue
		}
		lastClick = now

		pool := regularClickPool
		if input.IsModifierKey(ev.Code) {
			pool = modifierClickPool
		}

		if err := pool.Play(); err != nil {
			logf(stderr, "play click failed: %v", err)
		}
	}
}
