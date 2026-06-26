package app

import (
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"keyklik/internal/audio"
	"keyklik/internal/config"
	"keyklik/internal/input"
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

func Run(args []string, stdout io.Writer, stderr io.Writer) error {
	cfg, err := config.Parse(args)
	if err != nil {
		if errors.Is(err, config.ErrHelp) {
			fmt.Fprint(stdout, config.Usage(args[0]))
			return nil
		}

		fmt.Fprint(stderr, config.Usage(args[0]))
		return err
	}

	if cfg.KeyboardDevice == "" {
		device, err := defaultKeyboardDevice()
		if err != nil {
			return err
		}
		cfg.KeyboardDevice = device
	}

	clickPool, err := newClickPool(config.SampleRate, cfg.Volume, cfg.PitchLevel, playerPoolSize)
	if err != nil {
		return err
	}
	defer clickPool.Close()

	reader, err := openReader(cfg.KeyboardDevice)
	if err != nil {
		return err
	}
	defer reader.Close()

	log.Printf("listening on %s", cfg.KeyboardDevice)
	log.Printf("click config: sample rate %d, volume %.2f, pitch level %d", config.SampleRate, cfg.Volume, cfg.PitchLevel)

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

		if err := clickPool.Play(); err != nil {
			log.Printf("play click failed: %v", err)
		}
	}
}
