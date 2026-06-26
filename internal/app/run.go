package app

import (
	"errors"
	"fmt"
	"io"
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

	reader, err := openReader(cfg.KeyboardDevice)
	if err != nil {
		return err
	}
	defer util.IgnoreErr(reader.Close)

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
