package server

import (
	"testing"
	"time"
)

func TestPersistenceConfigDefaultsAndValidation(t *testing.T) {
	config := DefaultConfig(42)
	if config.SaveWorkers != 2 || config.SaveChunks != 8 ||
		config.SaveBytes != 4<<20 || config.AutosaveTicks != 6000 ||
		config.RetryBaseTicks != 20 || config.RetryMaxTicks != 1200 ||
		config.UnsavedBytes != 512<<20 || config.ShutdownTimeout != 30*time.Second ||
		config.SaveObserver != nil {
		t.Fatalf("persistence defaults=%+v", config)
	}

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "save workers", mutate: func(c *Config) { c.SaveWorkers = 0 }},
		{name: "save chunks", mutate: func(c *Config) { c.SaveChunks = 0 }},
		{name: "save bytes", mutate: func(c *Config) { c.SaveBytes = 0 }},
		{name: "autosave ticks", mutate: func(c *Config) { c.AutosaveTicks = 0 }},
		{name: "retry base ticks", mutate: func(c *Config) { c.RetryBaseTicks = 0 }},
		{name: "retry max ticks", mutate: func(c *Config) { c.RetryMaxTicks = 0 }},
		{name: "retry max below base", mutate: func(c *Config) { c.RetryMaxTicks = c.RetryBaseTicks - 1 }},
		{name: "unsaved bytes", mutate: func(c *Config) { c.UnsavedBytes = 0 }},
		{name: "shutdown timeout", mutate: func(c *Config) { c.ShutdownTimeout = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := DefaultConfig(42)
			test.mutate(&invalid)
			assertPanicsPersistence(t, invalid.validate)
		})
	}
}

func assertPanicsPersistence(t *testing.T, action func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("operation did not panic")
		}
	}()
	action()
}
