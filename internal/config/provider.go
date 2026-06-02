package config

import "sync/atomic"

// Provider holds the live *Config behind an atomic.Pointer so the settings
// handler can swap it on PUT without racing readers in the analytics engine
// or alerts checker.
//
// Callers must treat the returned *Config as immutable. A reload allocates
// a fresh *Config and swaps the pointer; in-place mutation is a bug.
type Provider struct {
	p atomic.Pointer[Config]
}

func NewProvider(c *Config) *Provider {
	pr := &Provider{}
	pr.p.Store(c)
	return pr
}

// Get returns the current config snapshot. Cheap; safe under concurrent reads.
func (p *Provider) Get() *Config { return p.p.Load() }

// Set atomically swaps the active config. Used by the settings handler after
// a successful PUT writes the new values to disk and re-runs Load.
func (p *Provider) Set(c *Config) { p.p.Store(c) }
