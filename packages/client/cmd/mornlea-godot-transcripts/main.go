// Command mornlea-godot-transcripts serves one deterministic protocol v44
// transcript scenario to sequentially connecting pilot clients.
//
// The command is a check-fixture server for the Godot terrain gate: it
// listens on 127.0.0.1, accepts exactly one connection per scenario round
// (rounds run sequentially, never concurrently), performs the server side of
// the protocol v44 login handshake through the shared `network` package, and
// then replays the scenario's play-state frames with a fixed inter-stage
// interval. Every packet is synthesized from the scenario's semantic
// description through the protocol structs, so the wire bytes are always
// protocol-legal and deterministic; the helper never improvises state.
//
// The inter-stage interval is the pacing mechanism: the protocol carries no
// consumption acknowledgment a server could observe, so the helper separates
// observable client states (initial snapshot, block delta, forget) in
// wall-clock time while the check scene polls the structural terrain summary
// with its own generous frame budgets. The helper exits zero after the last
// round and non-zero on any handshake, send, or deadline failure, so the
// gate can attribute failures from the helper log alone.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	scenarioPath := flag.String("scenario", "", "path to the terrain scenario JSON file")
	port := flag.Int("port", 0, "loopback TCP port to listen on (1..65535)")
	deadline := flag.Duration("deadline", 3*time.Minute, "overall serving deadline")
	flag.Parse()
	if *scenarioPath == "" {
		log.Fatal("mornlea-godot-transcripts: -scenario is required")
	}
	if *port < 1 || *port > 65535 {
		log.Fatalf("mornlea-godot-transcripts: -port %d is outside 1..65535", *port)
	}
	scenario, err := loadScenario(*scenarioPath)
	if err != nil {
		log.Fatalf("mornlea-godot-transcripts: %v", err)
	}

	// The interrupt context keeps a gate-side signal from wedging the
	// accept loop; the deadline context bounds the whole scenario.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithTimeout(context.Background(), *deadline)
	defer cancel()
	go func() {
		select {
		case <-signals:
			cancel()
		case <-ctx.Done():
		}
	}()

	if err := serveScenario(ctx, scenario, *port); err != nil {
		log.Fatalf("mornlea-godot-transcripts: %v", err)
	}
	log.Print("mornlea-godot-transcripts: scenario complete")
}
