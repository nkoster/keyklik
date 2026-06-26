package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultVolume = 0.30
	DefaultPitch  = 3
	SampleRate    = 48000
	byPathDir     = "/dev/input/by-path"
	preferredKbd  = "platform-i8042-serio-0-event-kbd"
)

var ErrHelp = errors.New("help requested")

type Config struct {
	KeyboardDevice string
	Volume         float64
	PitchLevel     int
}

func Usage(program string) string {
	return fmt.Sprintf(`Usage:
  %s [/dev/input/eventX] [volume 0.0-1.0 | pitch 1-5] [pitch 1-5]
  %s [/dev/input/eventX] [--volume 0.0-1.0] [--pitch 1-5]
  %s [/dev/input/eventX] [--volume=0.0-1.0] [--pitch=1-5]

Examples:
  %s /dev/input/event3
  %s
  %s /dev/input/event3 0.20
  %s /dev/input/event3 5
  %s /dev/input/event3 0.20 5
  %s /dev/input/event3 --volume 0.20 --pitch 4
`, program, program, program, program, program, program, program, program, program)
}

func Parse(args []string) (Config, error) {
	cfg := Config{
		Volume:     DefaultVolume,
		PitchLevel: DefaultPitch,
	}

	if len(args) == 0 {
		return cfg, fmt.Errorf("missing arguments")
	}

	if len(args) > 1 && isHelpArg(args[1]) {
		return cfg, ErrHelp
	}

	start := 1
	if len(args) > 1 && isDeviceArg(args[1]) {
		cfg.KeyboardDevice = args[1]
		start = 2
	}

	var flagVolume *float64
	var flagPitch *int
	positional := make([]string, 0, 2)

	for i := start; i < len(args); i++ {
		arg := args[i]

		switch {
		case isHelpArg(arg):
			return cfg, ErrHelp
		case arg == "--volume":
			if i+1 >= len(args) {
				return cfg, fmt.Errorf("missing value for --volume")
			}
			v, err := parseVolume(args[i+1])
			if err != nil {
				return cfg, err
			}
			flagVolume = &v
			i++
		case strings.HasPrefix(arg, "--volume="):
			v, err := parseVolume(strings.TrimPrefix(arg, "--volume="))
			if err != nil {
				return cfg, err
			}
			flagVolume = &v
		case arg == "--pitch":
			if i+1 >= len(args) {
				return cfg, fmt.Errorf("missing value for --pitch")
			}
			p, err := parsePitch(args[i+1])
			if err != nil {
				return cfg, err
			}
			flagPitch = &p
			i++
		case strings.HasPrefix(arg, "--pitch="):
			p, err := parsePitch(strings.TrimPrefix(arg, "--pitch="))
			if err != nil {
				return cfg, err
			}
			flagPitch = &p
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) > 2 {
		return cfg, fmt.Errorf("too many positional arguments")
	}

	if len(positional) == 1 {
		if p, err := parsePitch(positional[0]); err == nil {
			cfg.PitchLevel = p
		} else {
			v, err := parseVolume(positional[0])
			if err != nil {
				return cfg, fmt.Errorf("invalid value %q: expected volume 0.0-1.0 or pitch 1-5", positional[0])
			}
			cfg.Volume = v
		}
	}

	if len(positional) == 2 {
		v, err := parseVolume(positional[0])
		if err != nil {
			return cfg, err
		}
		p, err := parsePitch(positional[1])
		if err != nil {
			return cfg, err
		}
		cfg.Volume = v
		cfg.PitchLevel = p
	}

	if flagVolume != nil {
		cfg.Volume = *flagVolume
	}
	if flagPitch != nil {
		cfg.PitchLevel = *flagPitch
	}

	return cfg, nil
}

func isHelpArg(s string) bool {
	return s == "-h" || s == "--help"
}

func parseVolume(value string) (float64, error) {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid volume %q: %w", value, err)
	}
	if v < 0 || v > 1 {
		return 0, fmt.Errorf("invalid volume %q: must be between 0.0 and 1.0", value)
	}
	return v, nil
}

func parsePitch(value string) (int, error) {
	p, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid pitch level %q: %w", value, err)
	}
	if p < 1 || p > 5 {
		return 0, fmt.Errorf("invalid pitch level %q: must be between 1 and 5", value)
	}
	return p, nil
}

func isDeviceArg(arg string) bool {
	if strings.HasPrefix(arg, "--") {
		return false
	}
	if _, err := parsePitch(arg); err == nil {
		return false
	}
	if _, err := parseVolume(arg); err == nil {
		return false
	}
	return true
}

func defaultKeyboardDevice() (string, error) {
	entries, err := os.ReadDir(byPathDir)
	if err != nil {
		return "", fmt.Errorf("detect default keyboard device: %w", err)
	}

	for _, entry := range entries {
		if entry.Name() == preferredKbd {
			return filepath.Join(byPathDir, entry.Name()), nil
		}
	}

	for _, entry := range entries {
		if strings.Contains(entry.Name(), "kbd") {
			return filepath.Join(byPathDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("detect default keyboard device: no entry containing \"kbd\" found in %s", byPathDir)
}

func DefaultKeyboardDevice() (string, error) {
	return defaultKeyboardDevice()
}
