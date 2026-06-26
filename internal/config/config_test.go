package config

import (
	"errors"
	"strings"
	"testing"
)

func TestParse_NoExtraArgs_UsesDefaultsWithoutDeviceError(t *testing.T) {
	cfg, err := Parse([]string{"keyklik"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.KeyboardDevice != "" {
		t.Fatalf("expected empty keyboard device, got %q", cfg.KeyboardDevice)
	}
	if cfg.Volume != DefaultVolume {
		t.Fatalf("expected default volume %.2f, got %.2f", DefaultVolume, cfg.Volume)
	}
	if cfg.PitchLevel != DefaultPitch {
		t.Fatalf("expected default pitch %d, got %d", DefaultPitch, cfg.PitchLevel)
	}
}

func TestParse_SingleNumericArg_IsPositionalValue(t *testing.T) {
	t.Run("volume", func(t *testing.T) {
		cfg, err := Parse([]string{"keyklik", "0.20"})
		if err != nil {
			t.Fatalf("Parse returned error: %v", err)
		}
		if cfg.KeyboardDevice != "" {
			t.Fatalf("expected empty keyboard device, got %q", cfg.KeyboardDevice)
		}
		if cfg.Volume != 0.20 {
			t.Fatalf("expected volume 0.20, got %.2f", cfg.Volume)
		}
	})

	t.Run("pitch", func(t *testing.T) {
		cfg, err := Parse([]string{"keyklik", "5"})
		if err != nil {
			t.Fatalf("Parse returned error: %v", err)
		}
		if cfg.KeyboardDevice != "" {
			t.Fatalf("expected empty keyboard device, got %q", cfg.KeyboardDevice)
		}
		if cfg.PitchLevel != 5 {
			t.Fatalf("expected pitch level 5, got %d", cfg.PitchLevel)
		}
	})
}

func TestSelectKeyboardDeviceName_PrefersConfiguredDevice(t *testing.T) {
	name, err := selectKeyboardDeviceName([]string{
		"pci-0000:00:14.0-usb-0:1:1.0-event-kbd",
		preferredKbd,
	})
	if err != nil {
		t.Fatalf("selectKeyboardDeviceName returned error: %v", err)
	}
	if name != preferredKbd {
		t.Fatalf("expected preferred device %q, got %q", preferredKbd, name)
	}
}

func TestSelectKeyboardDeviceName_FallsBackToFirstKbd(t *testing.T) {
	expected := "pci-0000:00:14.0-usb-0:1:1.0-event-kbd"
	name, err := selectKeyboardDeviceName([]string{
		"mouse0",
		expected,
		"random",
	})
	if err != nil {
		t.Fatalf("selectKeyboardDeviceName returned error: %v", err)
	}
	if name != expected {
		t.Fatalf("expected fallback device %q, got %q", expected, name)
	}
}

func TestSelectKeyboardDeviceName_NoKeyboardEntry(t *testing.T) {
	_, err := selectKeyboardDeviceName([]string{"mouse0", "touchpad0"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no entry containing \"kbd\"") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParse_HelpArg(t *testing.T) {
	_, err := Parse([]string{"keyklik", "--help"})
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("expected ErrHelp, got %v", err)
	}
}
