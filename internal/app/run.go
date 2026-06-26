package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strconv"
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
	openReader = func(devicePath string) (eventReader, error) {
		return input.Open(devicePath)
	}
	defaultKeyboardDevice = config.DefaultKeyboardDevice
)

func programName(args []string) string {
	if len(args) == 0 || args[0] == "" {
		return "keyklik"
	}
	return args[0]
}

func logf(w io.Writer, format string, args ...any) {
	util.Ignore(fmt.Fprintf(w, format+"\n", args...))
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

	if cfg.KeyboardDevice == "" {
		device, err := defaultKeyboardDevice()
		if err != nil {
			return err
		}
		cfg.KeyboardDevice = device
	}

	reader, err := openReader(cfg.KeyboardDevice)
	if err != nil {
		return err
	}
	defer util.IgnoreErr(reader.Close)

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

	logf(stderr, "listening on %s", cfg.KeyboardDevice)
	logf(stderr, "click config: regular volume %.2f, regular pitch level %d, modifier volume %.2f, modifier pitch level %d", cfg.Volume, cfg.PitchLevel, cfg.ModifierVolume, cfg.ModifierPitch)

	pressedKeys := make(map[uint16]bool)
	lastClick := time.Time{}

	for {
		ev, err := reader.ReadEvent()
		if err != nil {
			return err
		}

		if !input.IsKeyboardEvent(ev) {
			continue
		}

		if ev.Value == input.KeyUp {
			delete(pressedKeys, ev.Code)
			continue
		}

		if ev.Value != input.KeyDown {
			continue
		}

		if pressedKeys[ev.Code] {
			continue
		}
		pressedKeys[ev.Code] = true

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
