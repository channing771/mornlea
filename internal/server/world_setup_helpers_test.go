package server

// world_setup_helpers_test.go：server 包共享的内存世界装配助手。

import (
	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/storage"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

func newAttachedWorldForTest(config Config, endpoint network.ServerEndpoint, generator Generator, store storage.Store) *Server {
	running := NewWorld(config, generator, store)
	if config.TrustedObserver {
		if err := running.AttachTrustedObserver(endpoint); err != nil {
			panic(err)
		}
		return running
	}
	spec := registrySessionSpec(testSessionID, 1, endpoint)
	spec.Restore = contract.PlayerRestore{
		SpawnDimension: config.SpawnDimension,
		SpawnAnchor:    config.SpawnAnchor,
	}
	if _, err := running.AttachSession(spec); err != nil {
		panic(err)
	}
	return running
}

func newMemoryAttachedWorldForTest(config Config, endpoint network.ServerEndpoint, generator Generator) *Server {
	return newAttachedWorldForTest(config, endpoint, generator, storage.NewMemory(storage.Metadata{FormatVersion: 3, Seed: config.Seed, SpawnDimension: config.SpawnDimension, SpawnAnchor: config.SpawnAnchor}))
}

func newEmbeddedAttachedWorldForTest(config Config, endpoint network.ServerEndpoint, store storage.Store) *Server {
	metadata := store.Metadata()
	config.Seed, config.SpawnDimension, config.SpawnAnchor = metadata.Seed, metadata.SpawnDimension, metadata.SpawnAnchor
	return newAttachedWorldForTest(config, endpoint, worldgen.New(metadata.Seed, false), store)
}
