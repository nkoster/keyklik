package input

import (
	"testing"
	"unsafe"
)

func TestEventSize(t *testing.T) {
	if unsafe.Sizeof(Event{}) != 24 {
		t.Fatalf("unexpected input.Event size: %d", unsafe.Sizeof(Event{}))
	}
}
