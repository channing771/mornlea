// Command mornlea-godot-smoke keeps a second real client in the playable-smoke world.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clientruntime "github.com/channing771/mornlea/packages/client/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

const (
	godotPilotPlayerID = "6a3f2c1e-9b4d-4e8a-a1c2-5d7e8f9a0b1c"
	remotePlayerID     = "72e706d8-578c-4b8c-9b71-3d5ac52f73c6"
	remoteDisplayName  = "GodotSmokeRemote"
	receiverCapacity   = 8192
	stepMessageBudget  = 64
	stepMeshBudget     = 0
	fixedStep          = 16_666_667 * time.Nanosecond
	wallClockFrame     = time.Second / 60
	remoteLookYaw      = float32(0.35)
	remoteLookPitch    = float32(-1.55)
)

type config struct {
	address        string
	duration       time.Duration
	identity       network.Identity
	expectedPeerID core.PlayerID
}

func main() {
	configuration, err := parseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, configuration); err != nil {
		fmt.Fprintf(os.Stderr, "Mornlea Godot smoke helper failed: %v\n", err)
		os.Exit(1)
	}
}

func parseConfig(arguments []string) (config, error) {
	flags := flag.NewFlagSet("mornlea-godot-smoke", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	address := flags.String("address", "", "dedicated server address")
	duration := flags.Duration("duration", 0, "maximum helper lifetime")
	playerIDText := flags.String("player-id", "", "fixed helper player identity")
	expectedPeerIDText := flags.String("expected-peer-id", "", "fixed Godot peer identity")
	if err := flags.Parse(arguments); err != nil {
		return config{}, fmt.Errorf("usage: mornlea-godot-smoke --address host:port --duration DURATION --player-id UUID --expected-peer-id UUID: %w", err)
	}
	if flags.NArg() != 0 {
		return config{}, errors.New("usage: mornlea-godot-smoke --address host:port --duration DURATION --player-id UUID --expected-peer-id UUID")
	}
	if strings.TrimSpace(*address) == "" {
		return config{}, errors.New("smoke helper address is required")
	}
	if *duration <= 0 {
		return config{}, errors.New("smoke helper duration must be positive")
	}
	if *playerIDText != remotePlayerID {
		return config{}, fmt.Errorf("smoke helper player identity must be %s", remotePlayerID)
	}
	if *expectedPeerIDText != godotPilotPlayerID {
		return config{}, fmt.Errorf("smoke helper expected peer identity must be %s", godotPilotPlayerID)
	}
	playerID, err := core.ParsePlayerID(*playerIDText)
	if err != nil {
		return config{}, fmt.Errorf("parse fixed smoke helper identity: %w", err)
	}
	expectedPeerID, err := core.ParsePlayerID(*expectedPeerIDText)
	if err != nil {
		return config{}, fmt.Errorf("parse fixed Godot peer identity: %w", err)
	}
	if playerID == expectedPeerID {
		return config{}, errors.New("smoke helper and Godot peer identities must differ")
	}
	return config{
		address:        *address,
		duration:       *duration,
		identity:       network.Identity{PlayerID: playerID, DisplayName: remoteDisplayName},
		expectedPeerID: expectedPeerID,
	}, nil
}

func run(parent context.Context, configuration config) (runErr error) {
	ctx, cancel := context.WithTimeout(parent, configuration.duration)
	defer cancel()

	// The fixture reuses the production v44 login and runtime path so remote-player
	// evidence cannot be manufactured by a shell or Python protocol surrogate.
	runtime, err := clientruntime.NewRemote(ctx, clientruntime.Options{
		RemoteAddress:    configuration.address,
		Identity:         configuration.identity,
		ViewDistance:     protocol.LoginViewDistanceMin,
		ReceiverCapacity: receiverCapacity,
	})
	if err != nil {
		return err
	}
	defer func() {
		runErr = errors.Join(runErr, runtime.Close())
	}()

	fmt.Printf("Mornlea Godot smoke helper connected: player=%s expected_peer=%s address=%s\n",
		configuration.identity.PlayerID, configuration.expectedPeerID, configuration.address)
	ticker := time.NewTicker(wallClockFrame)
	defer ticker.Stop()
	playObserved := false
	remoteObserved := false
	targetObserved := false
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				if !playObserved {
					return errors.New("helper lifetime ended before reaching Play")
				}
				fmt.Println("Mornlea Godot smoke helper completed its bounded lifetime.")
			}
			return nil
		case <-ticker.C:
			if err := runtime.SubmitInput(clientruntime.SemanticInput{
				Yaw: remoteLookYaw, Pitch: remoteLookPitch,
			}); err != nil {
				return fmt.Errorf("submit semantic input: %w", err)
			}
			result, err := runtime.Step(fixedStep, stepMessageBudget, stepMeshBudget)
			if err != nil {
				return fmt.Errorf("step remote runtime: %w", err)
			}
			if !playObserved && result.Frame.Phase == clientruntime.ConnectionPhasePlay {
				playObserved = true
				fmt.Println("Mornlea Godot smoke helper reached Play.")
			}
			if !remoteObserved {
				for _, entity := range result.Frame.Entities.Records() {
					if entity.PlayerID == configuration.expectedPeerID {
						remoteObserved = true
						fmt.Printf("Mornlea Godot smoke helper observed Godot peer: player=%s.\n",
							configuration.expectedPeerID)
						break
					}
				}
			}
			if !targetObserved && result.Frame.Target.Visible {
				targetObserved = true
				fmt.Printf("Mornlea Godot smoke helper observed target: %s.\n", result.Frame.Target.Name)
			}
		}
	}
}
