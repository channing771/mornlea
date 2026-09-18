package main

import (
	"testing"
	"time"
)

func TestParseConfigRequiresAddressAndPositiveDuration(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "missing address", args: []string{"--duration", "5s"}},
		{name: "missing duration", args: []string{"--address", "127.0.0.1:25565"}},
		{name: "missing helper identity", args: []string{
			"--address", "127.0.0.1:25565", "--duration", "5s",
			"--expected-peer-id", godotPilotPlayerID,
		}},
		{name: "missing expected peer identity", args: []string{
			"--address", "127.0.0.1:25565", "--duration", "5s",
			"--player-id", remotePlayerID,
		}},
		{name: "colliding identities", args: []string{
			"--address", "127.0.0.1:25565", "--duration", "5s",
			"--player-id", remotePlayerID, "--expected-peer-id", remotePlayerID,
		}},
		{name: "unexpected helper identity", args: []string{
			"--address", "127.0.0.1:25565", "--duration", "5s",
			"--player-id", godotPilotPlayerID, "--expected-peer-id", remotePlayerID,
		}},
		{name: "zero duration", args: []string{
			"--address", "127.0.0.1:25565", "--duration", "0s",
			"--player-id", remotePlayerID, "--expected-peer-id", godotPilotPlayerID,
		}},
		{name: "positional argument", args: []string{
			"--address", "127.0.0.1:25565", "--duration", "5s",
			"--player-id", remotePlayerID, "--expected-peer-id", godotPilotPlayerID, "extra",
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseConfig(test.args); err == nil {
				t.Fatalf("parseConfig(%q) accepted invalid arguments", test.args)
			}
		})
	}
}

func TestParseConfigAcceptsBoundedRun(t *testing.T) {
	t.Parallel()

	config, err := parseConfig([]string{
		"--address", "127.0.0.1:25565",
		"--duration", "5s",
		"--player-id", remotePlayerID,
		"--expected-peer-id", godotPilotPlayerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.address != "127.0.0.1:25565" || config.duration != 5*time.Second {
		t.Fatalf("config = %+v", config)
	}
	if !config.identity.PlayerID.Valid() || config.identity.DisplayName != "GodotSmokeRemote" {
		t.Fatalf("helper identity = %+v", config.identity)
	}
	if config.identity.PlayerID.String() == godotPilotPlayerID {
		t.Fatal("helper identity collides with the Godot pilot identity")
	}
	if config.expectedPeerID.String() != godotPilotPlayerID {
		t.Fatalf("expected peer identity = %s, want %s", config.expectedPeerID, godotPilotPlayerID)
	}
}
