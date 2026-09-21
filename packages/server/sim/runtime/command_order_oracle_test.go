package runtime_test

// This file is the package-local Go producer for the command ordering evidence.
// It is deliberately self-contained: the corpus JSON shape, the encoding and the
// hashing are the standard library only, and it imports neither the runtime
// oracle package nor any shared test package, because `packages/server` is not
// allowed to depend on `packages/tools`.
//
// The producer drives the real authoritative engine. It reuses the existing
// engine ordering test setup (`readyFlatEngineStocked` plus `stockedHotbar`),
// submits the frozen case's exact commands to the actual `Step`, and records
// what the authority did with them. Every outcome field it publishes comes from
// that execution: this file contains no ordering comparator that decides an
// outcome, so a lexical kind ordering cannot be mistaken for executed evidence.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/server/sim/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// commandOrderFamily is the corpus family this package executes. Its
	// eventual owner is the Rust domain crate, which owns the envelope and the
	// ordering rule this family pins.
	commandOrderFamily = "domain.input"
	// commandOrderVersion labels the case as the current command ordering rule
	// rather than a numbered schema: the ordering has no version history, so a
	// version number would imply a migration path that does not exist.
	commandOrderVersion = "current"
	// commandOrderOperation is the manifest operation name for an ordering case.
	commandOrderOperation = "order"
	// commandOrderConsumer is the manifest consumer this family pins: the Rust
	// crate that owns the envelope and the ordering rule.
	commandOrderConsumer = "mornlea_domain"
	// commandOrderCorpusRelDir is the repository-relative directory holding the
	// frozen ordering corpus case.
	commandOrderCorpusRelDir = "testdata/runtime-migration/cases/domain/command_order"
	// commandOrderCaseLabel is the corpus label of the one case this node
	// registers, and the subject of both corpus files.
	commandOrderCaseLabel = "session-sequence-arrival"
	// commandOrderCaseID is the manifest case identity the label carries.
	commandOrderCaseID = commandOrderFamily + "/" + commandOrderVersion + "/" + commandOrderCaseLabel
	// commandOrderSource is the primary provenance source of the ordering rule:
	// the authoritative engine that sorts and admits the drained commands.
	commandOrderSource = "packages/server/sim/runtime/engine_step.go"
	// commandOrderNumericSemantics records the numeric contract this family
	// pins, in the same shape the other domain families use.
	commandOrderNumericSemantics = "order by tick, session, sequence then arrival index; no key consults the command kind; a same-sequence pair of different kinds resolves to the earliest arrival; a zero sequence is rejected only for the command that takes part in acknowledgement"
	// commandOrderProducerTestRelPath and the two producer test names locate the
	// package-local producer that drives the real authority. The topic name is
	// the entry point the domain plan's filter requires; a corpus whose producer
	// test is missing has no independently executed evidence at all.
	commandOrderProducerTestRelPath = "packages/server/sim/runtime/command_order_oracle_test.go"
	commandOrderProducerExecuteName = "TestCommandOrderOracle"
	commandOrderProducerTopicName   = "TestCommandOrderOracle"
	// commandOrderCorpusReportName is the published report file name for the
	// executed ordering evidence.
	commandOrderCorpusReportName = "runtime-corpus-domain-command-order.json"
	// commandOrderTick is the tick the frozen case submits its commands in.
	commandOrderTick = uint64(7)
	// commandOrderSessionTwo is the second session the frozen case names.
	// Cross-session sequence numbers are not globally comparable, so the case
	// needs two sessions to pin that.
	commandOrderSessionTwo = runtime.SessionID(2)
	// commandOrderYaw is the look direction that makes the frozen case's block
	// placement admissible, carried as a decimal string so the exact float32
	// bits survive the corpus round trip.
	commandOrderYaw = "3.1415927"
	// commandOrderProbeSlot is the hotbar slot the admission boundary probe
	// selects. It is a slot the stocked hotbar leaves empty, so the probe's
	// write is unambiguous.
	commandOrderProbeSlot = 7
)

// commandOrderUpdateCorpus rewrites the frozen ordering corpus from the executed
// authority. It follows the same discipline as the other fixture update flags:
// an ordinary run only compares, so a frozen artifact is never silently
// regenerated to match an implementation.
var commandOrderUpdateCorpus = flag.Bool(
	"update-command-order-corpus",
	false,
	"rewrite testdata/runtime-migration/cases/domain/command_order from the executed authoritative engine",
)

// commandOrderInputCommand is one command a frozen case submits, in arrival
// order.
//
// The arrival index is the producer's admission position, so it is the only
// field that can break a tie the tick, session and sequence leave open. The yaw
// is carried as a decimal string because a placement command needs a finite look
// direction and the exact float32 bits have to survive the corpus round trip.
type commandOrderInputCommand struct {
	Session      uint64 `json:"session"`
	Sequence     uint64 `json:"sequence"`
	ArrivalIndex uint64 `json:"arrival_index"`
	Kind         string `json:"kind"`
	Slot         uint8  `json:"slot"`
	Yaw          string `json:"yaw,omitempty"`
}

// commandOrderInput is the frozen, self-describing corpus input for one case.
//
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the authority.
type commandOrderInput struct {
	Consumer string                     `json:"consumer"`
	Rule     string                     `json:"rule"`
	Tick     uint64                     `json:"tick"`
	Commands []commandOrderInputCommand `json:"commands"`
}

// commandOrderCase is one frozen corpus case: its label and its input, and
// nothing else.
type commandOrderCase struct {
	label string
	input commandOrderInput
}

// commandOrderCases is the ordered case table the producer executes.
//
// The one row is the packet's exact red input: two sessions at tick seven, with
// session one carrying a sequence eight selection, a sequence nine selection at
// arrival zero and a sequence nine block placement at arrival one. The contested
// pair is what makes the case worth freezing, because a kind-name ordering would
// hand the sequence nine slot to the block placement.
func commandOrderCases() []commandOrderCase {
	return []commandOrderCase{
		{
			label: commandOrderCaseLabel,
			input: commandOrderInput{
				Consumer: commandOrderConsumer,
				Rule:     commandOrderCaseLabel,
				Tick:     commandOrderTick,
				Commands: []commandOrderInputCommand{
					{Session: 2, Sequence: 1, ArrivalIndex: 0, Kind: "select_hotbar", Slot: 0},
					{Session: 1, Sequence: 9, ArrivalIndex: 0, Kind: "select_hotbar", Slot: 2},
					{Session: 1, Sequence: 9, ArrivalIndex: 1, Kind: "place_block", Slot: 0, Yaw: commandOrderYaw},
					{Session: 1, Sequence: 8, ArrivalIndex: 2, Kind: "select_hotbar", Slot: 1},
				},
			},
		},
	}
}

// commandOrderOutcome is the normalized outcome vocabulary this producer
// publishes, in the same JSON shape the corpus freezes for every other family.
//
// `Kind` is `ok` or `error`, and an accepted outcome names its own subject
// category. The fields are the executed evidence: nothing here is a value the
// producer chose.
type commandOrderOutcome struct {
	Kind     string         `json:"kind"`
	Category string         `json:"category,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
}

// commandOrderAssetRef identifies one corpus file and its reviewed digest.
type commandOrderAssetRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// commandOrderCaseSpec is the manifest entry one corpus case registers.
type commandOrderCaseSpec struct {
	ID           string               `json:"id"`
	Family       string               `json:"family"`
	Version      string               `json:"version"`
	Operation    string               `json:"operation"`
	Input        commandOrderAssetRef `json:"input"`
	InputFormat  string               `json:"input_format"`
	Expected     commandOrderAssetRef `json:"expected"`
	Checkpoints  []string             `json:"checkpoints"`
	RustConsumer string               `json:"rust_consumer"`
}

// commandOrderSessionSelection is one session's observed hotbar selection.
type commandOrderSessionSelection struct {
	Session string `json:"session"`
	Slot    int    `json:"slot"`
}

// commandOrderSessionSlotCount is one session's observed count in one slot.
type commandOrderSessionSlotCount struct {
	Session string `json:"session"`
	Slot    int    `json:"slot"`
	Count   int    `json:"count"`
}

// commandOrderSessionSequence is the highest sequence the authority admitted for
// one session, which the admission boundary probe reads directly.
type commandOrderSessionSequence struct {
	Session  string `json:"session"`
	Sequence string `json:"sequence"`
}

// commandOrderCommandRef names one command by its session, sequence, arrival
// index and normalized payload.
type commandOrderCommandRef struct {
	Session      string `json:"session"`
	Sequence     string `json:"sequence"`
	ArrivalIndex string `json:"arrival_index"`
	Kind         string `json:"kind"`
	Slot         int    `json:"slot"`
	Yaw          string `json:"yaw,omitempty"`
}

// commandOrderRejection is one authoritative rejection.
type commandOrderRejection struct {
	Session  string `json:"session"`
	Sequence string `json:"sequence"`
	Reason   string `json:"reason"`
}

// commandOrderRecord pairs one executed case with the input and the outcome the
// authority produced for it.
type commandOrderRecord struct {
	label   string
	input   commandOrderInput
	outcome commandOrderOutcome
}

// commandOrderRun is everything one batch submission to the real authority
// produced.
type commandOrderRun struct {
	// Selections is each session's hotbar selection after the batch tick.
	Selections []commandOrderSessionSelection
	// PlacementSlotCounts is each session's remaining count in the slot the
	// batch's placement command names, which is what a consumed placement
	// leaves behind.
	PlacementSlotCounts []commandOrderSessionSlotCount
	// PlacedBlocks counts the block changes the batch tick committed.
	PlacedBlocks int
	// ChunkRevision is the authoritative revision after the batch tick.
	ChunkRevision uint64
	// PlacementSuccesses and Rejections are the authority's own per-command
	// reports, in the order it applied them.
	PlacementSuccesses []commandOrderCommandRef
	Rejections         []commandOrderRejection
	// LastAdmittedSequence is each session's highest admitted sequence, read
	// through the admission boundary probe.
	LastAdmittedSequence []commandOrderSessionSequence
}

// TestCommandOrderOracle executes the frozen ordering case through the real
// authoritative engine and proves the committed corpus still matches what the
// authority produces.
//
// The recorded order is the order the packet states. The executed evidence pins
// the parts the authority's effects can observe directly: the contested
// same-sequence winner, the command the authority discarded, each session's
// last-write selection, and the highest sequence each session admitted. The
// reversed-arrival companion test pins the other half, that the winner follows
// the arrival index rather than the kind name.
func TestCommandOrderOracle(t *testing.T) {
	records := commandOrderExecute(t)
	commandOrderSyncCorpus(t, records)
	commandOrderExport(t, commandOrderRepoRoot(t), records)
}

// TestCommandOrderOracleReversedArrivalChoosesTheEarliestArrival proves the
// authority has no kind-name tiebreaker: submitting the same two commands with
// their arrival indices swapped moves the sequence nine winner from the hotbar
// selection to the block placement, and the observed effects follow.
func TestCommandOrderOracleReversedArrivalChoosesTheEarliestArrival(t *testing.T) {
	base := commandOrderCases()[0].input
	reversed := base
	reversed.Commands = append([]commandOrderInputCommand(nil), base.Commands...)
	reversed.Commands[1], reversed.Commands[2] = reversed.Commands[2], reversed.Commands[1]

	forward := commandOrderRunCase(t, base)
	backward := commandOrderRunCase(t, reversed)

	if forward.PlacedBlocks != 0 {
		t.Fatalf("red input placed %d blocks, want 0: the authority has to discard the later same-sequence placement", forward.PlacedBlocks)
	}
	if backward.PlacedBlocks != 1 {
		t.Fatalf("reversed arrival placed %d blocks, want 1: the earlier arrival has to win", backward.PlacedBlocks)
	}
	if got := commandOrderSelectionFor(t, forward, commandOrderSessionTwo); got != 0 {
		t.Fatalf("red input left session two selecting %d, want 0", got)
	}
	forwardSelection := commandOrderSelectionFor(t, forward, runtime.SessionID(1))
	backwardSelection := commandOrderSelectionFor(t, backward, runtime.SessionID(1))
	if forwardSelection != 2 {
		t.Fatalf("red input left session one selecting %d, want 2", forwardSelection)
	}
	if backwardSelection != 1 {
		t.Fatalf("reversed arrival left session one selecting %d, want 1: the discarded selection has to leave the earlier one in place", backwardSelection)
	}
}

// commandOrderExecute runs the whole case table through the real authority and
// returns one record per case in table order.
func commandOrderExecute(t *testing.T) []commandOrderRecord {
	t.Helper()

	cases := commandOrderCases()
	records := make([]commandOrderRecord, 0, len(cases))
	for _, entry := range cases {
		run := commandOrderRunCase(t, entry.input)
		records = append(records, commandOrderRecord{
			label:   entry.label,
			input:   entry.input,
			outcome: commandOrderOutcomeFor(t, entry.input, run),
		})
	}
	return records
}

// commandOrderRunCase submits one frozen batch to the real authority and
// observes what it did.
//
// The setup is the existing engine ordering test setup, so the batch runs
// against the same flat world, the same stocked hotbar and the same login-free
// registration the neighboring ordering tests use. The commands are submitted in
// the arrival order the case names, and the tick is advanced to the case's tick
// before the batch is drained, so the authority sees exactly one batch at
// exactly one tick.
func commandOrderRunCase(t *testing.T, spec commandOrderInput) commandOrderRun {
	t.Helper()

	engine, sessionOne, chunkPos := readyFlatEngineStocked(t, stockedHotbar(core.ItemStone))
	engine.RegisterPlayer(commandOrderSessionTwo, runtime.PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    chunkPos,
	})
	for engine.TickCount() < spec.Tick {
		engine.Step()
	}
	for _, command := range spec.Commands {
		engine.Enqueue(commandOrderRuntimeCommand(command))
	}
	result := engine.Step()

	run := commandOrderRun{
		PlacedBlocks:  commandOrderPlacedBlocks(result),
		ChunkRevision: commandOrderChunkRevision(t, engine, chunkPos),
	}
	sessions := []runtime.SessionID{sessionOne, commandOrderSessionTwo}
	for _, session := range sessions {
		snapshot, ok := engine.PlayerSnapshot(session)
		if !ok {
			t.Fatalf("session %d has no snapshot after the batch tick", session)
		}
		label := strconv.FormatUint(uint64(session), 10)
		run.Selections = append(run.Selections, commandOrderSessionSelection{
			Session: label,
			Slot:    int(snapshot.Inventory.Hotbar.Selected),
		})
		if slot, named := commandOrderPlacementSlot(spec, session); named {
			run.PlacementSlotCounts = append(run.PlacementSlotCounts, commandOrderSessionSlotCount{
				Session: label,
				Slot:    int(slot),
				Count:   int(snapshot.Inventory.Hotbar.Slots[slot].Count),
			})
		}
	}
	run.PlacementSuccesses = commandOrderPlacementSuccesses(result)
	run.Rejections = commandOrderRejections(result)
	run.LastAdmittedSequence = commandOrderProbeAdmissionBoundary(t, engine, spec)
	return run
}

// commandOrderRuntimeCommand renders one frozen command as the authoritative
// command value. The kind names are the corpus vocabulary; an unknown kind is a
// hard error rather than a silently skipped row.
func commandOrderRuntimeCommand(command commandOrderInputCommand) runtime.Command {
	rendered := runtime.Command{
		Session:  runtime.SessionID(command.Session),
		Sequence: command.Sequence,
		Slot:     command.Slot,
	}
	switch command.Kind {
	case "select_hotbar":
		rendered.Kind = runtime.CommandSelectHotbar
	case "place_block":
		rendered.Kind = runtime.CommandPlaceBlock
		yaw, err := strconv.ParseFloat(command.Yaw, 32)
		if err != nil {
			// The corpus carries a finite decimal look direction, so a value
			// that does not parse is a malformed case rather than a rejection.
			panic(fmt.Sprintf("runtime: case command yaw %q: %v", command.Yaw, err))
		}
		rendered.Yaw = float32(yaw)
	default:
		panic(fmt.Sprintf("runtime: case command kind %q is not one this producer executes", command.Kind))
	}
	return rendered
}

// commandOrderPlacementSlot reports the hotbar slot the batch's placement
// command names for one session, which is the slot a consumed placement would
// decrement.
func commandOrderPlacementSlot(spec commandOrderInput, session runtime.SessionID) (uint8, bool) {
	for _, command := range spec.Commands {
		if runtime.SessionID(command.Session) == session && command.Kind == "place_block" {
			return command.Slot, true
		}
	}
	return 0, false
}

// commandOrderPlacedBlocks counts the block changes one tick committed.
func commandOrderPlacedBlocks(result runtime.TickResult) int {
	placed := 0
	for _, batch := range result.Changes {
		placed += len(batch.Changes)
	}
	return placed
}

// commandOrderChunkRevision reads the authoritative revision of the shared
// chunk, which is what a committed block write advances.
func commandOrderChunkRevision(t *testing.T, engine *runtime.Engine, chunkPos core.ChunkPos) uint64 {
	t.Helper()
	_, revision, ok := engine.CloneReadyChunk(core.ChunkKey{
		Dimension: core.Overworld,
		Pos:       chunkPos,
	})
	if !ok {
		t.Fatalf("shared chunk %+v is not ready after the batch tick", chunkPos)
	}
	return revision
}

// commandOrderPlacementSuccesses renders the authority's own placement reports.
func commandOrderPlacementSuccesses(result runtime.TickResult) []commandOrderCommandRef {
	successes := make([]commandOrderCommandRef, 0, len(result.PlacementSuccesses))
	for _, success := range result.PlacementSuccesses {
		successes = append(successes, commandOrderCommandRef{
			Session:  strconv.FormatUint(uint64(success.Session), 10),
			Sequence: strconv.FormatUint(success.Sequence, 10),
			Kind:     "place_block",
		})
	}
	return successes
}

// commandOrderRejections renders the authority's own rejections.
func commandOrderRejections(result runtime.TickResult) []commandOrderRejection {
	rejections := make([]commandOrderRejection, 0, len(result.Rejected))
	for _, rejected := range result.Rejected {
		rejections = append(rejections, commandOrderRejection{
			Session:  strconv.FormatUint(uint64(rejected.Session), 10),
			Sequence: strconv.FormatUint(rejected.Sequence, 10),
			Reason:   strconv.FormatUint(uint64(rejected.Reason), 10),
		})
	}
	return rejections
}

// commandOrderProbeAdmissionBoundary reads each session's highest admitted
// sequence straight from the authority.
//
// A command at or below the session's last admitted sequence is dropped, and one
// above it is applied, so submitting the batch's own highest sequence and then
// its successor observes the boundary instead of assuming it. The probe runs in
// the ticks after the batch tick and never touches the batch's own evidence.
func commandOrderProbeAdmissionBoundary(
	t *testing.T,
	engine *runtime.Engine,
	spec commandOrderInput,
) []commandOrderSessionSequence {
	t.Helper()

	highest := make(map[runtime.SessionID]uint64)
	for _, command := range spec.Commands {
		session := runtime.SessionID(command.Session)
		if command.Sequence > highest[session] {
			highest[session] = command.Sequence
		}
	}
	sessions := make([]runtime.SessionID, 0, len(highest))
	for session := range highest {
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i] < sessions[j] })

	boundaries := make([]commandOrderSessionSequence, 0, len(sessions))
	for _, session := range sessions {
		atBoundary := highest[session]
		before, _ := engine.PlayerSnapshot(session)
		engine.Enqueue(runtime.Command{
			Session: session, Sequence: atBoundary, Kind: runtime.CommandSelectHotbar,
			Slot: commandOrderProbeSlot,
		})
		engine.Step()
		repeat, _ := engine.PlayerSnapshot(session)
		if repeat.Inventory.Hotbar.Selected != before.Inventory.Hotbar.Selected {
			t.Fatalf(
				"session %d applied a repeat of sequence %d, which has to be at or below its last admitted sequence",
				session, atBoundary)
		}

		engine.Enqueue(runtime.Command{
			Session: session, Sequence: atBoundary + 1, Kind: runtime.CommandSelectHotbar,
			Slot: commandOrderProbeSlot,
		})
		engine.Step()
		above, _ := engine.PlayerSnapshot(session)
		if above.Inventory.Hotbar.Selected != commandOrderProbeSlot {
			t.Fatalf(
				"session %d did not apply sequence %d, which has to be above its last admitted sequence",
				session, atBoundary+1)
		}
		boundaries = append(boundaries, commandOrderSessionSequence{
			Session:  strconv.FormatUint(uint64(session), 10),
			Sequence: strconv.FormatUint(atBoundary, 10),
		})
	}
	return boundaries
}

// commandOrderSelectionFor reads one session's observed selection out of a run.
func commandOrderSelectionFor(t *testing.T, run commandOrderRun, session runtime.SessionID) int {
	t.Helper()
	want := strconv.FormatUint(uint64(session), 10)
	for _, selection := range run.Selections {
		if selection.Session == want {
			return selection.Slot
		}
	}
	t.Fatalf("run recorded no selection for session %d", session)
	return 0
}

// commandOrderOutcomeFor renders one executed run in the normalized outcome
// vocabulary.
//
// Every value comes from the run. The command lists are normalized from the
// frozen input so the frozen evidence carries the full command set beside the
// outcome, and the admitted and discarded lists are the order the packet states,
// which the executed selections, the placement count and the admission boundary
// are asserted against.
func commandOrderOutcomeFor(t *testing.T, spec commandOrderInput, run commandOrderRun) commandOrderOutcome {
	t.Helper()

	commands := make([]commandOrderCommandRef, 0, len(spec.Commands))
	for _, command := range spec.Commands {
		commands = append(commands, commandOrderNormalizeCommand(command))
	}
	fields := map[string]any{
		"tick":                   strconv.FormatUint(spec.Tick, 10),
		"commands":               commands,
		"admitted":               commandOrderAdmitted(t, spec, run),
		"discarded":              commandOrderDiscarded(t, spec, run),
		"selected_hotbar":        run.Selections,
		"placement_slot_counts":  run.PlacementSlotCounts,
		"placed_blocks":          run.PlacedBlocks,
		"chunk_revision":         strconv.FormatUint(run.ChunkRevision, 10),
		"placement_successes":    run.PlacementSuccesses,
		"rejections":             run.Rejections,
		"last_admitted_sequence": run.LastAdmittedSequence,
	}
	commandOrderAssertConsistency(t, spec, run, fields)
	return commandOrderOutcome{Kind: "ok", Category: commandOrderCaseLabel, Fields: fields}
}

// commandOrderNormalizeCommand renders one frozen command in the normalized
// field map, matching the other domain families: the u64 intake metadata is a
// decimal string and a finite look direction is its exact float32 bits.
func commandOrderNormalizeCommand(command commandOrderInputCommand) commandOrderCommandRef {
	normalized := commandOrderCommandRef{
		Session:      strconv.FormatUint(command.Session, 10),
		Sequence:     strconv.FormatUint(command.Sequence, 10),
		ArrivalIndex: strconv.FormatUint(command.ArrivalIndex, 10),
		Kind:         command.Kind,
		Slot:         int(command.Slot),
	}
	if command.Yaw != "" {
		yaw, err := strconv.ParseFloat(command.Yaw, 32)
		if err != nil {
			panic(fmt.Sprintf("runtime: case command yaw %q: %v", command.Yaw, err))
		}
		normalized.Yaw = commandOrderFloatBits(float32(yaw))
	}
	return normalized
}

// commandOrderFloatBits renders one float32 as the lowercase hex of its IEEE-754
// bits, which is the form the other domain families freeze.
func commandOrderFloatBits(value float32) string {
	bits := math.Float32bits(value)
	return hex.EncodeToString([]byte{
		byte(bits),
		byte(bits >> 8),
		byte(bits >> 16),
		byte(bits >> 24),
	})
}

// commandOrderAdmitted renders the commands the authority applied, in the order
// the packet states: tick, then session, then sequence, then arrival index.
//
// Which commands were applied is decided by `commandOrderClassify` from the
// executed effects, never from this ordering key. The key only renders the order
// the packet states for the commands the authority already admitted, so the
// frozen evidence carries a claim the executed evidence is checked against
// rather than a value this producer chose.
func commandOrderAdmitted(t *testing.T, spec commandOrderInput, run commandOrderRun) []commandOrderCommandRef {
	admitted, _ := commandOrderClassify(t, spec, run)
	rendered := make([]commandOrderCommandRef, 0, len(admitted))
	for _, command := range admitted {
		rendered = append(rendered, commandOrderNormalizeCommand(command))
	}
	commandOrderSortRefs(rendered)
	return rendered
}

// commandOrderDiscarded renders the commands the authority dropped, in the same
// order the packet states.
func commandOrderDiscarded(t *testing.T, spec commandOrderInput, run commandOrderRun) []commandOrderCommandRef {
	_, discarded := commandOrderClassify(t, spec, run)
	rendered := make([]commandOrderCommandRef, 0, len(discarded))
	for _, command := range discarded {
		rendered = append(rendered, commandOrderNormalizeCommand(command))
	}
	commandOrderSortRefs(rendered)
	return rendered
}

// commandOrderSortRefs renders one normalized command list in the packet's
// stated key order.
func commandOrderSortRefs(refs []commandOrderCommandRef) {
	sort.SliceStable(refs, func(i, j int) bool {
		left, right := refs[i], refs[j]
		if left.Session != right.Session {
			return commandOrderUint(left.Session) < commandOrderUint(right.Session)
		}
		if left.Sequence != right.Sequence {
			return commandOrderUint(left.Sequence) < commandOrderUint(right.Sequence)
		}
		return commandOrderUint(left.ArrivalIndex) < commandOrderUint(right.ArrivalIndex)
	})
}

// commandOrderClassify decides, from the executed effects alone, which command of
// each contested same-sequence group the authority applied.
//
// A group is contested when more than one command names the same session and
// sequence, which is the only situation where the authority has to choose. The
// choice is read from the effects rather than assumed: a hotbar selection is the
// applied member when the authority left its slot selected, and a placement is
// the applied member when the authority reported that placement. Exactly one
// member of a contested group has to be explained, so a group whose effects name
// no winner or two winners fails instead of being recorded as a plausible
// outcome.
func commandOrderClassify(
	t *testing.T,
	spec commandOrderInput,
	run commandOrderRun,
) (admitted, discarded []commandOrderInputCommand) {
	t.Helper()

	observed := make(map[string]int, len(run.Selections))
	for _, selection := range run.Selections {
		observed[selection.Session] = selection.Slot
	}
	placed := make(map[string]bool, len(run.PlacementSuccesses))
	for _, success := range run.PlacementSuccesses {
		placed[success.Session+"/"+success.Sequence] = true
	}

	type groupKey struct {
		session  uint64
		sequence uint64
	}
	groups := make(map[groupKey][]commandOrderInputCommand)
	order := make([]groupKey, 0, len(spec.Commands))
	for _, command := range spec.Commands {
		key := groupKey{session: command.Session, sequence: command.Sequence}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], command)
	}

	losers := make(map[commandOrderInputCommand]bool, len(spec.Commands))
	for _, key := range order {
		members := groups[key]
		if len(members) < 2 {
			continue
		}
		session := strconv.FormatUint(key.session, 10)
		sequence := strconv.FormatUint(key.sequence, 10)
		winner := -1
		for index, member := range members {
			applied := false
			switch member.Kind {
			case "select_hotbar":
				applied = observed[session] == int(member.Slot)
			case "place_block":
				applied = placed[session+"/"+sequence]
			}
			if !applied {
				continue
			}
			if winner >= 0 {
				t.Fatalf(
					"session %d sequence %d has two commands the executed effects explain as applied: %+v and %+v",
					key.session, key.sequence, members[winner], member)
			}
			winner = index
		}
		if winner < 0 {
			t.Fatalf(
				"session %d sequence %d has no command the executed effects explain as applied: %+v",
				key.session, key.sequence, members)
		}
		for index, member := range members {
			if index != winner {
				losers[member] = true
			}
		}
	}

	for _, command := range spec.Commands {
		if losers[command] {
			discarded = append(discarded, command)
			continue
		}
		admitted = append(admitted, command)
	}
	return admitted, discarded
}

func commandOrderUint(text string) uint64 {
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("runtime: normalized command field %q: %v", text, err))
	}
	return value
}

// commandOrderAssertConsistency ties the stated order to the executed evidence.
//
// The recorded order is a claim about the authority, so the observations have to
// agree with it: the last command the recorded order applies to a session has to
// be the one whose selection the authority left in place, every contested
// same-sequence group has to have exactly one applied member and that member has
// to be the earlier arrival of the two, a discarded command has to leave no
// placement and no consumed item behind, and the admission boundary has to sit
// exactly at the highest sequence the recorded order admits for that session.
func commandOrderAssertConsistency(
	t *testing.T,
	spec commandOrderInput,
	run commandOrderRun,
	fields map[string]any,
) {
	t.Helper()

	admitted, _ := fields["admitted"].([]commandOrderCommandRef)
	discarded, _ := fields["discarded"].([]commandOrderCommandRef)
	if len(admitted)+len(discarded) != len(spec.Commands) {
		t.Fatalf("recorded %d admitted and %d discarded commands, want %d submitted",
			len(admitted), len(discarded), len(spec.Commands))
	}

	observed := make(map[string]int, len(run.Selections))
	for _, selection := range run.Selections {
		observed[selection.Session] = selection.Slot
	}
	for session, slot := range observed {
		last, ok := commandOrderLastSelection(admitted, session)
		if !ok {
			t.Fatalf("session %s selected %d but the recorded order admits no selection for it", session, slot)
		}
		if last.Slot != slot {
			t.Fatalf("session %s selected %d, which the recorded order cannot produce: its last applied selection names slot %d",
				session, slot, last.Slot)
		}
	}

	// The contested group's applied member has to be the earlier arrival of the
	// two, which is the whole point of the case: a kind-name ordering would
	// promote the later arrival instead.
	for _, command := range discarded {
		for _, applied := range admitted {
			if applied.Session != command.Session || applied.Sequence != command.Sequence {
				continue
			}
			if commandOrderUint(applied.ArrivalIndex) > commandOrderUint(command.ArrivalIndex) {
				t.Fatalf(
					"session %s sequence %s applied arrival %s and discarded arrival %s, so the later arrival won",
					command.Session, command.Sequence, applied.ArrivalIndex, command.ArrivalIndex)
			}
		}
	}

	for _, command := range discarded {
		if command.Kind != "place_block" {
			continue
		}
		if run.PlacedBlocks != 0 {
			t.Fatalf("a discarded placement left %d placed blocks behind", run.PlacedBlocks)
		}
		for _, count := range run.PlacementSlotCounts {
			if count.Session == command.Session && count.Slot == command.Slot && count.Count != core.MaxStackCount {
				t.Fatalf("a discarded placement consumed from slot %d of session %s", count.Slot, count.Session)
			}
		}
	}

	for _, boundary := range run.LastAdmittedSequence {
		highest := uint64(0)
		for _, command := range admitted {
			if command.Session != boundary.Session {
				continue
			}
			if sequence := commandOrderUint(command.Sequence); sequence > highest {
				highest = sequence
			}
		}
		if boundary.Sequence != strconv.FormatUint(highest, 10) {
			t.Fatalf("session %s admitted up to sequence %s, want %d from the recorded order",
				boundary.Session, boundary.Sequence, highest)
		}
	}
}

// commandOrderLastSelection reports the selection the recorded order applies last
// for one session, which is the selection the authority has to leave in place.
func commandOrderLastSelection(admitted []commandOrderCommandRef, session string) (commandOrderCommandRef, bool) {
	var last commandOrderCommandRef
	found := false
	for _, command := range admitted {
		if command.Session != session || command.Kind != "select_hotbar" {
			continue
		}
		if !found ||
			commandOrderUint(command.Sequence) > commandOrderUint(last.Sequence) ||
			(commandOrderUint(command.Sequence) == commandOrderUint(last.Sequence) &&
				commandOrderUint(command.ArrivalIndex) > commandOrderUint(last.ArrivalIndex)) {
			last = command
			found = true
		}
	}
	return last, found
}

// commandOrderSyncCorpus writes or verifies the frozen corpus files.
//
// An ordinary run is read-only: it proves the committed files still match what
// the authority produces now, so a drifted artifact fails instead of being
// regenerated. The explicit update flag rewrites them, which is the only way a
// frozen artifact changes.
func commandOrderSyncCorpus(t *testing.T, records []commandOrderRecord) {
	t.Helper()

	corpusDir := filepath.Join(commandOrderRepoRoot(t), filepath.FromSlash(commandOrderCorpusRelDir))
	want := make(map[string][]byte, len(records)*2)
	for _, record := range records {
		input, err := json.MarshalIndent(record.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", record.label, err)
		}
		outcome, err := json.MarshalIndent(record.outcome, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus outcome for %s: %v", record.label, err)
		}
		want[record.label+".input.json"] = append(input, '\n')
		want[record.label+".expected.json"] = append(outcome, '\n')
	}

	if *commandOrderUpdateCorpus {
		for relative, data := range want {
			target := filepath.Join(corpusDir, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create corpus directory: %v", err)
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				t.Fatalf("write %s: %v", relative, err)
			}
		}
		return
	}

	for relative, data := range want {
		target := filepath.Join(corpusDir, filepath.FromSlash(relative))
		committed, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read frozen corpus case %s: %v (rerun with -update-command-order-corpus after reviewing the change)", relative, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("frozen corpus case %s drifted from the executed authority (rerun with -update-command-order-corpus after reviewing the change)", relative)
		}
	}

	var committed []string
	if err := filepath.WalkDir(corpusDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		relative, relErr := filepath.Rel(corpusDir, path)
		if relErr != nil {
			return relErr
		}
		committed = append(committed, filepath.ToSlash(relative))
		return nil
	}); err != nil {
		t.Fatalf("walk corpus directory: %v", err)
	}
	sort.Strings(committed)
	for _, relative := range committed {
		if _, expected := want[relative]; !expected {
			t.Errorf("frozen corpus case %s is not produced by any executed table row", relative)
		}
	}
	for _, record := range records {
		input, err := os.ReadFile(filepath.Join(corpusDir, record.label+".input.json"))
		if err != nil {
			t.Fatalf("read frozen corpus input %s: %v", record.label, err)
		}
		envelope, err := commandOrderDecodeInput(input)
		if err != nil {
			t.Fatalf("decode frozen corpus input %s: %v", record.label, err)
		}
		if envelope.Consumer != commandOrderConsumer {
			t.Fatalf("frozen corpus case %s names consumer %q, want %q",
				record.label, envelope.Consumer, commandOrderConsumer)
		}
		if strings.TrimSpace(envelope.Rule) == "" {
			t.Fatalf("frozen corpus case %s names no rule", record.label)
		}
		if envelope.Tick != record.input.Tick || len(envelope.Commands) != len(record.input.Commands) {
			t.Fatalf("frozen corpus case %s describes a different batch than the executed table", record.label)
		}
	}
}

// commandOrderExport publishes the executed evidence to a caller-supplied fresh
// directory.
//
// The environment variable is the only export path and has no repository
// default, so an ordinary run exports nothing. The published files are the
// corpus input, the executed outcome and the report the controller merges, all
// with the digests reviewed against the committed files.
func commandOrderExport(t *testing.T, root string, records []commandOrderRecord) {
	t.Helper()

	dir := strings.TrimSpace(os.Getenv("RUNTIME_ORACLE_EXPORT_DIR"))
	if dir == "" {
		return
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("RUNTIME_ORACLE_EXPORT_DIR %s is not a directory: %v", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read RUNTIME_ORACLE_EXPORT_DIR %s: %v", dir, err)
	}
	if len(entries) != 0 {
		t.Fatalf("RUNTIME_ORACLE_EXPORT_DIR %s is not fresh", dir)
	}

	cases := make([]commandOrderCaseSpec, 0, len(records))
	for _, record := range records {
		input, err := json.MarshalIndent(record.input, "", "  ")
		if err != nil {
			t.Fatalf("encode exported input for %s: %v", record.label, err)
		}
		outcome, err := json.MarshalIndent(record.outcome, "", "  ")
		if err != nil {
			t.Fatalf("encode exported outcome for %s: %v", record.label, err)
		}
		input = append(input, '\n')
		outcome = append(outcome, '\n')
		if err := commandOrderWriteExport(filepath.Join(dir, record.label+".input.json"), input); err != nil {
			t.Fatalf("export input for %s: %v", record.label, err)
		}
		if err := commandOrderWriteExport(filepath.Join(dir, record.label+".expected.json"), outcome); err != nil {
			t.Fatalf("export outcome for %s: %v", record.label, err)
		}
		cases = append(cases, commandOrderCaseSpec{
			ID:        commandOrderFamily + "/" + commandOrderVersion + "/" + record.label,
			Family:    commandOrderFamily,
			Version:   commandOrderVersion,
			Operation: commandOrderOperation,
			Input: commandOrderAssetRef{
				Path:   commandOrderCorpusRelDir + "/" + record.label + ".input.json",
				SHA256: commandOrderDigest(input),
			},
			InputFormat: "json",
			Expected: commandOrderAssetRef{
				Path:   commandOrderCorpusRelDir + "/" + record.label + ".expected.json",
				SHA256: commandOrderDigest(outcome),
			},
			Checkpoints:  []string{"0"},
			RustConsumer: commandOrderConsumer,
		})
	}

	report := map[string]any{
		"family":             commandOrderFamily,
		"version":            commandOrderVersion,
		"operation":          commandOrderOperation,
		"consumer":           commandOrderConsumer,
		"numeric_semantics":  commandOrderNumericSemantics,
		"source":             commandOrderSource,
		"producer_test":      commandOrderProducerTestRelPath,
		"producer_test_name": commandOrderProducerExecuteName,
		"topic_test_name":    commandOrderProducerTopicName,
		"corpus_rel_dir":     commandOrderCorpusRelDir,
		"report_name":        commandOrderCorpusReportName,
		"case_id":            commandOrderCaseID,
		"cases":              cases,
		"repository_root":    root,
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("encode export report: %v", err)
	}
	if err := commandOrderWriteExport(filepath.Join(dir, commandOrderCorpusReportName), append(encoded, '\n')); err != nil {
		t.Fatalf("export report: %v", err)
	}
}

// commandOrderWriteExport writes one exported file and refuses a symlink target,
// so an export can never land outside the caller's directory.
func commandOrderWriteExport(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to export through a symlink: %s", path)
	}
	return nil
}

// commandOrderDigest renders one byte slice as the corpus digest form.
func commandOrderDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// commandOrderRepoRoot discovers the repository root from the test's working
// directory, so the corpus paths stay repository-relative without a shared
// helper package.
func commandOrderRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.work above the working directory %s", dir)
		}
		dir = parent
	}
}

// commandOrderDecodeInput reads one frozen corpus input and rejects trailing
// content, so a producer never executes bytes the case did not name.
func commandOrderDecodeInput(data []byte) (commandOrderInput, error) {
	var spec commandOrderInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&spec); err != nil {
		return commandOrderInput{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return commandOrderInput{}, fmt.Errorf("trailing content after the JSON value")
	}
	return spec, nil
}
