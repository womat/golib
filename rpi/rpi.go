// Package rpi defines a generic interface for GPIO event handling on Raspberry Pi.
//
// This package provides a standard GPIO interface that can be implemented by different backends.
// It defines event types for rising and falling edges, an Event struct with timestamps, and
// a common interface (`GPIO`) for interacting with GPIO lines.
//
// Features:
// - Defines GPIO event types (`RisingEdge` and `LineEventFallingEdge`)
// - Provides an Event struct containing a timestamp and event type
// - Defines a GPIO interface (`GPIO`) that different implementations can use
// - Supports input/output modes, pull-up/pull-down resistors, and debounce settings
// - Can be implemented with various hardware backends (e.g., `gpioemu`, `gpiod`)
//
// Example Usage with different backends:
//
//	// Using a real GPIO backend
//	import "github.com/example/gpiodriver"
//
//	gpioDevice, err := gpiodriver.NewPort(17)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Using an emulated GPIO backend
//	import "github.com/example/gpioemu"
//
//	emulatedDevice, err := gpioemu.NewPort(17, time.Millisecond*100)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// General usage with any backend implementing `rpi.GPIO`
//	var gpioPort rpi.GPIO = gpioDevice
//
//	gpioPort.SetOutputMode()
//	gpioPort.SetValue(1)
//
//	gpioPort.StartWatchingEvents(func(evt rpi.Event) {
//	    fmt.Println("GPIO Event:", evt)
//	})
//
// Note: This package does not interact with hardware directly but defines
// the interface for GPIO event handling, allowing multiple implementations.
package rpi

import (
	"time"
)

const (
	FallingEdge = 0 // FallingEdge indicates an active to inactive event (high to low).
	RisingEdge  = 1 // RisingEdge indicates an inactive event to an active event (low to high).

	High = 1 // High represents a high signal level.
	Low  = 0 // Low represents a low signal level.
)

// Event represents a state change event (e.g., RisingEdge or FallingEdge) on a GPIO port.
type Event struct {
	Time time.Time // Time is the exact time when the edge event was detected.
	Edge int       // Edge indicates the type of state change (RisingEdge or FallingEdge).
}

type GPIO interface {
	Close() error
	GetValue() (int, error)
	Port() int
	Info() string
	SetDebounceTime(time.Duration) error
	SetInputMode() error
	SetOutputMode() error
	SetPullDown() error
	SetPullUp() error
	SetValue(int) error
	WatchingEvents(func(Event)) error
	StopWatching() error
}
