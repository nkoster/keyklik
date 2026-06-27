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
	if cfg.ModifierVolume != DefaultModifierVolume {
		t.Fatalf("expected default modifier volume %.2f, got %.2f", DefaultModifierVolume, cfg.ModifierVolume)
	}
	if cfg.ModifierPitch != DefaultModifierPitch {
		t.Fatalf("expected default modifier pitch %d, got %d", DefaultModifierPitch, cfg.ModifierPitch)
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

func TestSelectKeyboardDeviceNames_PrefersBuiltInThenAddsOtherKeyboards(t *testing.T) {
	names, err := selectKeyboardDeviceNames([]string{
		"pci-0000:00:14.0-usb-0:13.4:1.0-event-kbd",
		"mouse0",
		preferredKbd,
		"platform-PNP0C14:02-event",
	})
	if err != nil {
		t.Fatalf("selectKeyboardDeviceNames returned error: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("expected 2 keyboard names, got %d (%v)", len(names), names)
	}
	if names[0] != preferredKbd {
		t.Fatalf("expected preferred keyboard first, got %q", names[0])
	}
	if names[1] != "pci-0000:00:14.0-usb-0:13.4:1.0-event-kbd" {
		t.Fatalf("expected second keyboard device to be usb keyboard, got %q", names[1])
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

func TestParse_ModifierFlagsOverrideDefaults(t *testing.T) {
	cfg, err := Parse([]string{"keyklik", "--volume", "0.20", "--pitch", "4", "--modifier-volume", "0.35", "--modifier-pitch", "2"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.Volume != 0.20 {
		t.Fatalf("expected regular volume 0.20, got %.2f", cfg.Volume)
	}
	if cfg.PitchLevel != 4 {
		t.Fatalf("expected regular pitch 4, got %d", cfg.PitchLevel)
	}
	if cfg.ModifierVolume != 0.35 {
		t.Fatalf("expected modifier volume 0.35, got %.2f", cfg.ModifierVolume)
	}
	if cfg.ModifierPitch != 2 {
		t.Fatalf("expected modifier pitch 2, got %d", cfg.ModifierPitch)
	}
}

func TestParse_ModifierSettingsUseDedicatedDefaults(t *testing.T) {
	cfg, err := Parse([]string{"keyklik", "--volume", "0.25", "--pitch", "5"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.ModifierVolume != DefaultModifierVolume {
		t.Fatalf("expected modifier volume %.2f, got %.2f", DefaultModifierVolume, cfg.ModifierVolume)
	}
	if cfg.ModifierPitch != DefaultModifierPitch {
		t.Fatalf("expected modifier pitch %d, got %d", DefaultModifierPitch, cfg.ModifierPitch)
	}
}

func TestParse_BackgroundFlag_ReturnsDeprecationError(t *testing.T) {
	_, err := Parse([]string{"keyklik", "--background"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no longer needed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParse_ForegroundFlag(t *testing.T) {
	cfg, err := Parse([]string{"keyklik", "--foreground"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if !cfg.Foreground {
		t.Fatal("expected foreground mode to be enabled")
	}
}

func TestParse_MisspelledForegroundFlag_ReturnsError(t *testing.T) {
	_, err := Parse([]string{"keyklik", "--forground"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParse_StopAndPIDFileFlags(t *testing.T) {
	cfg, err := Parse([]string{"keyklik", "--stop", "--pidfile", "/tmp/keyklik.pid"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if !cfg.Stop {
		t.Fatal("expected stop mode to be enabled")
	}
	if cfg.PIDFile != "/tmp/keyklik.pid" {
		t.Fatalf("expected pidfile %q, got %q", "/tmp/keyklik.pid", cfg.PIDFile)
	}
}
