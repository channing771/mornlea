package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// This file is the Go producer for the domain inventory, container and chat
// command family. It executes every committed corpus case through the current
// Go protocol command DTOs: the verdict comes from each DTO's `Validate`, and
// an admitted command is encoded and decoded again through the production
// codec so the frozen evidence carries the exact wire payload the encoder
// produced. No world authority, listener, model call or native ABI is
// involved: the producer only reads immutable corpus inputs, calls in-process
// validators and moves bytes through the real codec, and the frozen corpus
// files it materializes are read-only inputs for later consumers.

const (
	// domainCommandInventoryFamily is the corpus family this package executes.
	// Its eventual owner is the Rust domain crate, which owns the inventory,
	// container and chat payloads this family pins.
	domainCommandInventoryFamily = "domain.command_inventory"
	// domainCommandInventoryVersion labels the family as the current value
	// rules rather than a numbered schema: the command payload rules have no
	// version history, so a version number would imply a migration path that
	// does not exist.
	domainCommandInventoryVersion = "current"
	// domainCommandInventoryOperation is the manifest operation name for a
	// command admission case. An admitted case additionally round-trips through
	// the codec inside the same admission step, because the payload's wire
	// layout is part of what the case pins.
	domainCommandInventoryOperation = "admit"
	// domainCommandInventoryConsumer is the manifest consumer the change pins
	// for this family: the Rust crate that owns these rules.
	domainCommandInventoryConsumer = "mornlea_domain"
	// domainCommandInventoryCorpusRelDir is the repository-relative directory
	// holding the frozen inventory command corpus cases.
	domainCommandInventoryCorpusRelDir = "testdata/runtime-migration/cases/domain/command_inventory"
	// domainCommandInventoryProducerTestRelPath and the two producer test names
	// locate the package-local producer that executes the protocol command
	// DTOs. The topic name is the entry point the domain plan's filter
	// requires; a corpus whose producer test is missing has no independently
	// executed evidence at all.
	domainCommandInventoryProducerTestRelPath = "packages/tools/cmd/runtime-oracle/domain_command_inventory_test.go"
	domainCommandInventoryProducerExecuteName = "TestDomainCommandInventoryOracleExecutesEveryCase"
	domainCommandInventoryProducerTopicName   = "TestDomainOracle_command_inventory"
	// domainCommandInventoryCorpusReportName is the published report file name
	// for the executed inventory command evidence.
	domainCommandInventoryCorpusReportName = "runtime-corpus-domain-command-inventory.json"
	// domainCommandInventorySource is the primary provenance source of the
	// inventory command payload rules.
	domainCommandInventorySource = "packages/shared/network/protocol/message_inventory.go"
	// domainCommandInventoryNumericSemantics records the numeric contract this
	// family pins, in the same shape the other families use.
	domainCommandInventoryNumericSemantics = "inventory 0..35; crafting view 0..44 with one grid end; furnace view 0..38 output source only; chest view 0..62; partial/quick/drop share view bounds without the stricter crafting and furnace-output rules; reject same slot, malformed container reference and unknown view; chat text 1..1024 bytes"
)

// domainCommandInventoryFamilySources is the merged provenance set the family
// records. Every entry is a file the producer's rules or wire layout are read
// from, so a change to any of them is a change to the recorded evidence. The
// stack-splitting and drop-stack files belong to the set because the partial,
// quick-move and drop-stack commands share one validator and one wire shape,
// and the companion message file belongs because the chat command text rule is
// declared beside the chat events rather than with the other commands.
var domainCommandInventoryFamilySources = []string{
	"packages/shared/network/protocol/message_inventory.go",
	"packages/shared/network/protocol/message_container.go",
	"packages/shared/network/protocol/message_stack_splitting.go",
	"packages/shared/network/protocol/message_drop_stack.go",
	"packages/shared/network/protocol/message_companion.go",
	"packages/shared/network/protocol/packet.go",
	"packages/shared/network/codec/codec_client.go",
	"packages/shared/core/inventory.go",
	"packages/shared/core/furnace.go",
	"packages/shared/core/chest.go",
	"packages/shared/core/container.go",
}

// domainCommandInventoryLabelPattern is the shape a corpus label must have: a
// lowercase slug with an optional zero-padded numeric suffix, so a boundary row
// sorts in numeric order under a lexical sort.
var domainCommandInventoryLabelPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(-[0-9]+)?$`)

// updateDomainCommandInventoryCorpus rewrites the frozen inventory command
// corpus from the executed protocol DTOs. It follows the same discipline as the
// other fixture update flags: an ordinary run only compares, so a frozen
// artifact is never silently regenerated to match an implementation.
var updateDomainCommandInventoryCorpus = flag.Bool(
	"update-domain-command-inventory-corpus",
	false,
	"rewrite testdata/runtime-migration/cases/domain/command_inventory from the executed protocol command DTOs",
)

// domainCommandInventoryInput is the frozen, self-describing corpus input for
// one case.
//
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the authority. Every field is a pointer so an absent
// field and an explicit zero stay distinguishable, which matters because a zero
// sequence, a zero slot index and a false single flag are all meaningful
// values. The container reference is carried as its raw wire fields so a
// malformed reference stays expressible: the case names the chunk, the kind,
// the array slot and the generation separately, exactly as the encoder writes
// them.
type domainCommandInventoryInput struct {
	Consumer      string  `json:"consumer"`
	Rule          string  `json:"rule"`
	Sequence      *uint64 `json:"sequence,omitempty"`
	From          *uint8  `json:"from,omitempty"`
	To            *uint8  `json:"to,omitempty"`
	Slot          *uint8  `json:"slot,omitempty"`
	View          *uint8  `json:"view,omitempty"`
	Single        *bool   `json:"single,omitempty"`
	Kind          *string `json:"kind,omitempty"`
	ChunkX        *int32  `json:"chunk_x,omitempty"`
	ChunkZ        *int32  `json:"chunk_z,omitempty"`
	ContainerSlot *uint8  `json:"container_slot,omitempty"`
	Generation    *uint32 `json:"generation,omitempty"`
	Text          *string `json:"text,omitempty"`
}

func (in domainCommandInventoryInput) withSequence(sequence uint64) domainCommandInventoryInput {
	in.Sequence = &sequence
	return in
}

func (in domainCommandInventoryInput) withFrom(from uint8) domainCommandInventoryInput {
	in.From = &from
	return in
}

func (in domainCommandInventoryInput) withTo(to uint8) domainCommandInventoryInput {
	in.To = &to
	return in
}

func (in domainCommandInventoryInput) withSlot(slot uint8) domainCommandInventoryInput {
	in.Slot = &slot
	return in
}

func (in domainCommandInventoryInput) withView(view uint8) domainCommandInventoryInput {
	in.View = &view
	return in
}

func (in domainCommandInventoryInput) withSingle(single bool) domainCommandInventoryInput {
	in.Single = &single
	return in
}

// withContainer names the raw container reference fields one case carries. The
// generation is passed separately because a zero generation is one of the
// boundaries this family pins and must stay distinguishable from an absent
// field.
func (in domainCommandInventoryInput) withContainer(kind string, x, z int32, slot uint8, generation uint32) domainCommandInventoryInput {
	in.Kind = &kind
	in.ChunkX = &x
	in.ChunkZ = &z
	in.ContainerSlot = &slot
	in.Generation = &generation
	return in
}

func (in domainCommandInventoryInput) withText(text string) domainCommandInventoryInput {
	in.Text = &text
	return in
}

// domainCommandInventoryCase is one frozen corpus case: its label and its
// input, and nothing else.
type domainCommandInventoryCase struct {
	label string
	input domainCommandInventoryInput
}

// domainCommandInventoryCases is the ordered case table the producer executes.
//
// The rows are the boundaries the inventory, container and chat command rules
// name: the last slot of each unified view and the first index above it, the
// same-slot rejection, the crafting rule that refuses two inventory-region
// ends, the furnace rule that reserves the output slot as a source, the chest
// view that reserves nothing, the malformed container reference that has to be
// rejected before the slot bounds are consulted, the two rules the partial,
// quick-move and drop-stack commands deliberately do not publish, and the chat
// text rule that retains a mention verbatim. Each container case names its
// reference through `withContainer`, so a zero reference in a non-container view
// is never needed to express absence.
func domainCommandInventoryCases() []domainCommandInventoryCase {
	inventory := []domainCommandInventoryCase{
		{
			label: "move-inventory-last-slot-to-first",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-inventory"}.
				withSequence(6).withFrom(35).withTo(0),
		},
		{
			label: "move-inventory-first-to-last-slot",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-inventory"}.
				withSequence(6).withFrom(0).withTo(35),
		},
		{
			label: "move-inventory-source-above-array",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-inventory"}.
				withSequence(6).withFrom(36).withTo(0),
		},
		{
			label: "move-inventory-target-above-array",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-inventory"}.
				withSequence(6).withFrom(0).withTo(36),
		},
		{
			label: "move-inventory-same-slot",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-inventory"}.
				withSequence(6).withFrom(7).withTo(7),
		},
		{
			label: "move-crafting-grid-to-backpack",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-crafting"}.
				withSequence(7).withFrom(8).withTo(44),
		},
		{
			label: "move-crafting-backpack-to-grid",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-crafting"}.
				withSequence(7).withFrom(9).withTo(0),
		},
		{
			label: "move-crafting-both-ends-in-inventory",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-crafting"}.
				withSequence(7).withFrom(9).withTo(10),
		},
		{
			label: "move-crafting-slot-above-view",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-crafting"}.
				withSequence(7).withFrom(45).withTo(0),
		},
		{
			label: "move-crafting-same-slot",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-crafting"}.
				withSequence(7).withFrom(3).withTo(3),
		},
	}

	containers := []domainCommandInventoryCase{
		{
			label: "move-container-furnace-output-as-source",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-container"}.
				withSequence(9).withContainer("furnace", -2, 5, 7, 3).withFrom(38).withTo(0),
		},
		{
			label: "move-container-furnace-output-as-target",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-container"}.
				withSequence(9).withContainer("furnace", -2, 5, 7, 3).withFrom(0).withTo(38),
		},
		{
			label: "move-container-furnace-slot-above-view",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-container"}.
				withSequence(9).withContainer("furnace", -2, 5, 7, 3).withFrom(39).withTo(0),
		},
		{
			label: "move-container-chest-last-slot",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-container"}.
				withSequence(9).withContainer("chest", 4, -9, 5, 11).withFrom(62).withTo(0),
		},
		{
			label: "move-container-chest-last-slot-as-target",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-container"}.
				withSequence(9).withContainer("chest", 4, -9, 5, 11).withFrom(0).withTo(62),
		},
		{
			label: "move-container-chest-slot-above-view",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-container"}.
				withSequence(9).withContainer("chest", 4, -9, 5, 11).withFrom(63).withTo(0),
		},
		{
			label: "move-container-same-slot",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-container"}.
				withSequence(9).withContainer("furnace", -2, 5, 7, 3).withFrom(5).withTo(5),
		},
		{
			// The reference names a chest array slot that does not exist and both
			// move indices are far outside every view, so the recorded rejection
			// proves the reference is validated before the slot bounds.
			label: "move-container-malformed-ref-before-slots",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-container"}.
				withSequence(9).withContainer("chest", 0, 0, 16, 7).withFrom(200).withTo(201),
		},
		{
			label: "move-container-zero-generation-ref",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-container"}.
				withSequence(9).withContainer("furnace", 0, 0, 0, 0).withFrom(200).withTo(201),
		},
		{
			label: "move-container-unknown-kind-ref",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-container"}.
				withSequence(9).withContainer("crate", 0, 0, 0, 7).withFrom(0).withTo(1),
		},
		{
			label: "close-container",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "close-container"}.
				withSequence(10),
		},
		{
			label: "drop-selected-item",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "drop-selected-item"}.
				withSequence(11),
		},
		{
			label: "take-crafting-output",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "take-crafting-output"}.
				withSequence(15),
		},
		{
			label: "take-crafting-output-zero-sequence",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "take-crafting-output"}.
				withSequence(0),
		},
		{
			label: "equip-armor",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "equip-armor"}.
				withSequence(18),
		},
	}

	splits := []domainCommandInventoryCase{
		{
			// The protocol publishes no both-ends-in-inventory rule for a partial
			// move, so the stricter crafting validator must not be reused here.
			label: "move-partial-crafting-both-ends-in-inventory",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-partial"}.
				withSequence(19).withView(protocol.StackViewCrafting).withFrom(9).withTo(10).withSingle(false),
		},
		{
			// The protocol publishes no output-slot rule for a partial move
			// either, so slot 38 stays a legal destination.
			label: "move-partial-furnace-output-as-target",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-partial"}.
				withSequence(19).withView(protocol.StackViewContainer).withContainer("furnace", -2, 5, 7, 3).
				withFrom(0).withTo(38).withSingle(true),
		},
		{
			label: "move-partial-inventory-last-slot",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-partial"}.
				withSequence(19).withView(protocol.StackViewInventory).withFrom(0).withTo(35).withSingle(false),
		},
		{
			label: "move-partial-inventory-slot-above-view",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-partial"}.
				withSequence(19).withView(protocol.StackViewInventory).withFrom(35).withTo(36).withSingle(false),
		},
		{
			label: "move-partial-crafting-slot-above-view",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-partial"}.
				withSequence(19).withView(protocol.StackViewCrafting).withFrom(44).withTo(45).withSingle(false),
		},
		{
			label: "move-partial-same-slot",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-partial"}.
				withSequence(19).withView(protocol.StackViewCrafting).withFrom(9).withTo(9).withSingle(true),
		},
		{
			label: "move-partial-single-flag-true",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-partial"}.
				withSequence(19).withView(protocol.StackViewInventory).withFrom(0).withTo(1).withSingle(true),
		},
		{
			label: "move-partial-single-flag-false",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-partial"}.
				withSequence(19).withView(protocol.StackViewInventory).withFrom(0).withTo(1).withSingle(false),
		},
		{
			label: "move-partial-inventory-view-carries-container",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-partial"}.
				withSequence(19).withView(protocol.StackViewInventory).withContainer("furnace", -2, 5, 7, 3).
				withFrom(0).withTo(1).withSingle(false),
		},
		{
			label: "move-partial-crafting-view-carries-container",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-partial"}.
				withSequence(19).withView(protocol.StackViewCrafting).withContainer("chest", 4, -9, 5, 11).
				withFrom(0).withTo(1).withSingle(false),
		},
		{
			label: "move-partial-container-view-malformed-ref",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-partial"}.
				withSequence(19).withView(protocol.StackViewContainer).withContainer("chest", 0, 0, 16, 7).
				withFrom(0).withTo(1).withSingle(false),
		},
		{
			label: "move-partial-unknown-view",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "move-partial"}.
				withSequence(19).withView(3).withFrom(0).withTo(1).withSingle(false),
		},
		{
			label: "quick-move-inventory-last-slot",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "quick-move"}.
				withSequence(20).withView(protocol.StackViewInventory).withFrom(35),
		},
		{
			label: "quick-move-inventory-slot-above-view",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "quick-move"}.
				withSequence(20).withView(protocol.StackViewInventory).withFrom(36),
		},
		{
			label: "quick-move-crafting-last-slot",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "quick-move"}.
				withSequence(20).withView(protocol.StackViewCrafting).withFrom(44),
		},
		{
			label: "quick-move-furnace-output-slot",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "quick-move"}.
				withSequence(20).withView(protocol.StackViewContainer).withContainer("furnace", -2, 5, 7, 3).withFrom(38),
		},
		{
			label: "quick-move-chest-slot-above-view",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "quick-move"}.
				withSequence(20).withView(protocol.StackViewContainer).withContainer("chest", 4, -9, 5, 11).withFrom(63),
		},
		{
			label: "quick-move-container-view-malformed-ref",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "quick-move"}.
				withSequence(20).withView(protocol.StackViewContainer).withContainer("furnace", 0, 0, 32, 7).withFrom(0),
		},
		{
			label: "drop-stack-inventory-last-slot",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "drop-stack"}.
				withSequence(21).withView(protocol.StackViewInventory).withSlot(35),
		},
		{
			label: "drop-stack-inventory-slot-above-view",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "drop-stack"}.
				withSequence(21).withView(protocol.StackViewInventory).withSlot(36),
		},
		{
			label: "drop-stack-crafting-last-slot",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "drop-stack"}.
				withSequence(21).withView(protocol.StackViewCrafting).withSlot(44),
		},
		{
			label: "drop-stack-furnace-slot-above-view",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "drop-stack"}.
				withSequence(21).withView(protocol.StackViewContainer).withContainer("furnace", -2, 5, 7, 3).withSlot(39),
		},
		{
			label: "drop-stack-container-view-malformed-ref",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "drop-stack"}.
				withSequence(21).withView(protocol.StackViewContainer).withContainer("chest", 0, 0, 16, 7).withSlot(0),
		},
	}

	chat := []domainCommandInventoryCase{
		{
			// The mention is retained byte for byte: this payload performs no
			// addressing, warp, stop or queue policy, and it carries no sequence.
			label: "chat-intent-mention-retained",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "chat-intent"}.
				withText("@Bob 停止"),
		},
		{
			label: "chat-intent-plain-command",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "chat-intent"}.
				withText("mine stone"),
		},
		{
			label: "chat-intent-empty",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "chat-intent"}.
				withText(""),
		},
		{
			label: "chat-intent-untrimmed",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "chat-intent"}.
				withText(" mine "),
		},
		{
			label: "chat-intent-control",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "chat-intent"}.
				withText("mi\u0001ne"),
		},
		{
			label: "chat-intent-1025-bytes",
			input: domainCommandInventoryInput{Consumer: domainCommandInventoryConsumer, Rule: "chat-intent"}.
				withText(strings.Repeat("x", protocol.ChatCommandTextMaxBytes+1)),
		},
	}

	cases := make([]domainCommandInventoryCase, 0, len(inventory)+len(containers)+len(splits)+len(chat))
	cases = append(cases, inventory...)
	cases = append(cases, containers...)
	cases = append(cases, splits...)
	cases = append(cases, chat...)
	return cases
}

// runDomainCommandInventory executes one corpus case through the current Go
// protocol command DTOs.
//
// The verdict always comes from the DTO's own `Validate`, which is the same
// validator the codec applies on both the encode and the decode side. The rule
// name and the rejection category come from the same bounds the DTO reads, and
// the two are cross-checked against each other, so a classification that
// disagrees with the authority fails the run instead of publishing a
// plausible-looking rejection.
func runDomainCommandInventory(c CaseSpec, input []byte) (Outcome, []byte, error) {
	spec, err := domainCommandInventoryDecodeInput(input)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: decode input: %w", c.ID, err)
	}
	if spec.Consumer != domainCommandInventoryConsumer {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names consumer %q, want %q", c.ID, spec.Consumer, domainCommandInventoryConsumer)
	}

	switch spec.Rule {
	case "move-inventory":
		return domainCommandInventoryRunMoveInventory(c, spec)
	case "move-crafting":
		return domainCommandInventoryRunMoveCrafting(c, spec)
	case "move-container":
		return domainCommandInventoryRunMoveContainer(c, spec)
	case "close-container":
		return domainCommandInventoryRunSequenceOnly(c, spec, "close-container")
	case "drop-selected-item":
		return domainCommandInventoryRunSequenceOnly(c, spec, "drop-selected-item")
	case "take-crafting-output":
		return domainCommandInventoryRunTakeCraftingOutput(c, spec)
	case "equip-armor":
		return domainCommandInventoryRunSequenceOnly(c, spec, "equip-armor")
	case "move-partial":
		return domainCommandInventoryRunMovePartial(c, spec)
	case "quick-move":
		return domainCommandInventoryRunQuickMove(c, spec)
	case "drop-stack":
		return domainCommandInventoryRunDropStack(c, spec)
	case "chat-intent":
		return domainCommandInventoryRunChatIntent(c, spec)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown rule %q", c.ID, spec.Rule)
	}
}

// domainCommandInventoryRunMoveInventory admits one whole-stack inventory move,
// whose only payload bounds are the fixed slot range and the distinct-slot rule.
func domainCommandInventoryRunMoveInventory(c CaseSpec, spec domainCommandInventoryInput) (Outcome, []byte, error) {
	sequence, err := domainCommandInventorySequence(c, spec.Sequence)
	if err != nil {
		return Outcome{}, nil, err
	}
	from, err := domainCommandInventorySlot(c, "from", spec.From)
	if err != nil {
		return Outcome{}, nil, err
	}
	to, err := domainCommandInventorySlot(c, "to", spec.To)
	if err != nil {
		return Outcome{}, nil, err
	}

	command := protocol.MoveInventoryStack{Sequence: sequence, From: from, To: to}
	category, rule := domainCommandInventoryInventoryMoveRule(from, to)
	return domainCommandInventoryFinish(c, "move-inventory", command, domainCommandInventoryNormalizeMove(sequence, from, to), category, rule, command.Validate() == nil)
}

// domainCommandInventoryRunMoveCrafting admits one crafting move, which adds the
// rule that both ends may not sit inside the backpack region.
func domainCommandInventoryRunMoveCrafting(c CaseSpec, spec domainCommandInventoryInput) (Outcome, []byte, error) {
	sequence, err := domainCommandInventorySequence(c, spec.Sequence)
	if err != nil {
		return Outcome{}, nil, err
	}
	from, err := domainCommandInventorySlot(c, "from", spec.From)
	if err != nil {
		return Outcome{}, nil, err
	}
	to, err := domainCommandInventorySlot(c, "to", spec.To)
	if err != nil {
		return Outcome{}, nil, err
	}

	command := protocol.MoveCraftingStack{Sequence: sequence, From: from, To: to}
	category, rule := domainCommandInventoryCraftingMoveRule(from, to)
	return domainCommandInventoryFinish(c, "move-crafting", command, domainCommandInventoryNormalizeMove(sequence, from, to), category, rule, command.Validate() == nil)
}

// domainCommandInventoryRunMoveContainer admits one whole-stack container move.
// The reference is resolved from the raw case fields, so a malformed reference
// is rejected by the same validator the DTO applies before it looks at a slot.
func domainCommandInventoryRunMoveContainer(c CaseSpec, spec domainCommandInventoryInput) (Outcome, []byte, error) {
	sequence, err := domainCommandInventorySequence(c, spec.Sequence)
	if err != nil {
		return Outcome{}, nil, err
	}
	ref, err := domainCommandInventoryRef(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	from, err := domainCommandInventorySlot(c, "from", spec.From)
	if err != nil {
		return Outcome{}, nil, err
	}
	to, err := domainCommandInventorySlot(c, "to", spec.To)
	if err != nil {
		return Outcome{}, nil, err
	}

	command := protocol.MoveContainerStack{Sequence: sequence, Container: ref, From: from, To: to}
	fields := domainCommandInventoryNormalizeContainerMove(sequence, ref, from, to)
	category, rule := domainCommandInventoryContainerMoveRule(ref, from, to)
	return domainCommandInventoryFinish(c, "move-container", command, fields, category, rule, command.Validate() == nil)
}

// domainCommandInventoryRunSequenceOnly admits one command whose entire payload
// is the sequence. The protocol publishes no further bound, so the only rule
// these rows can break is a missing sequence in the case itself.
func domainCommandInventoryRunSequenceOnly(c CaseSpec, spec domainCommandInventoryInput, subject string) (Outcome, []byte, error) {
	sequence, err := domainCommandInventorySequence(c, spec.Sequence)
	if err != nil {
		return Outcome{}, nil, err
	}

	var command protocol.ClientPacket
	var admitted bool
	switch subject {
	case "close-container":
		closeContainer := protocol.CloseContainer{Sequence: sequence}
		command, admitted = closeContainer, closeContainer.Validate() == nil
	case "drop-selected-item":
		drop := protocol.DropSelectedItem{Sequence: sequence}
		command, admitted = drop, drop.Validate() == nil
	case "equip-armor":
		equip := protocol.EquipArmor{Sequence: sequence}
		command, admitted = equip, equip.Validate() == nil
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown sequence-only command %q", c.ID, subject)
	}
	fields := map[string]any{"sequence": strconv.FormatUint(sequence, 10)}
	return domainCommandInventoryFinish(c, subject, command, fields, "", "", admitted)
}

// domainCommandInventoryRunTakeCraftingOutput admits one take-crafting-output
// command, the one command in this family whose sequence may not be zero
// because it has to take part in command acknowledgement.
func domainCommandInventoryRunTakeCraftingOutput(c CaseSpec, spec domainCommandInventoryInput) (Outcome, []byte, error) {
	sequence, err := domainCommandInventorySequence(c, spec.Sequence)
	if err != nil {
		return Outcome{}, nil, err
	}

	command := protocol.TakeCraftingOutput{Sequence: sequence}
	category, rule := domainCommandInventoryTakeOutputRule(sequence)
	return domainCommandInventoryFinish(c, "take-crafting-output", command, map[string]any{"sequence": strconv.FormatUint(sequence, 10)}, category, rule, command.Validate() == nil)
}

// domainCommandInventoryRunMovePartial admits one partial move. The view bounds
// and the distinct-slot rule apply, and neither the crafting both-ends rule nor
// the furnace output rule does, because the protocol publishes neither here.
func domainCommandInventoryRunMovePartial(c CaseSpec, spec domainCommandInventoryInput) (Outcome, []byte, error) {
	sequence, err := domainCommandInventorySequence(c, spec.Sequence)
	if err != nil {
		return Outcome{}, nil, err
	}
	view, err := domainCommandInventoryView(c, spec.View)
	if err != nil {
		return Outcome{}, nil, err
	}
	ref, err := domainCommandInventoryRef(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	from, err := domainCommandInventorySlot(c, "from", spec.From)
	if err != nil {
		return Outcome{}, nil, err
	}
	to, err := domainCommandInventorySlot(c, "to", spec.To)
	if err != nil {
		return Outcome{}, nil, err
	}
	single, err := domainCommandInventoryFlag(c, "single", spec.Single)
	if err != nil {
		return Outcome{}, nil, err
	}

	command := protocol.MoveStackPartial{Sequence: sequence, Container: ref, View: view, From: from, To: to, Single: single}
	fields := domainCommandInventoryNormalizeSplit(sequence, view, ref, from, to, single)
	category, rule := domainCommandInventoryPartialMoveRule(view, ref, from, to)
	return domainCommandInventoryFinish(c, "move-partial", command, fields, category, rule, command.Validate() == nil)
}

// domainCommandInventoryRunQuickMove admits one quick-move command, which shares
// the partial view and reference bounds and carries no destination at all.
func domainCommandInventoryRunQuickMove(c CaseSpec, spec domainCommandInventoryInput) (Outcome, []byte, error) {
	sequence, err := domainCommandInventorySequence(c, spec.Sequence)
	if err != nil {
		return Outcome{}, nil, err
	}
	view, err := domainCommandInventoryView(c, spec.View)
	if err != nil {
		return Outcome{}, nil, err
	}
	ref, err := domainCommandInventoryRef(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	from, err := domainCommandInventorySlot(c, "from", spec.From)
	if err != nil {
		return Outcome{}, nil, err
	}

	command := protocol.QuickMoveStack{Sequence: sequence, Container: ref, View: view, From: from}
	fields := domainCommandInventoryNormalizeSource(sequence, view, ref, from)
	category, rule := domainCommandInventorySplitRule(view, ref, from, from)
	return domainCommandInventoryFinish(c, "quick-move", command, fields, category, rule, command.Validate() == nil)
}

// domainCommandInventoryRunDropStack admits one drop-stack command, which shares
// the same view and reference bounds as the quick move.
func domainCommandInventoryRunDropStack(c CaseSpec, spec domainCommandInventoryInput) (Outcome, []byte, error) {
	sequence, err := domainCommandInventorySequence(c, spec.Sequence)
	if err != nil {
		return Outcome{}, nil, err
	}
	view, err := domainCommandInventoryView(c, spec.View)
	if err != nil {
		return Outcome{}, nil, err
	}
	ref, err := domainCommandInventoryRef(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	slot, err := domainCommandInventorySlot(c, "slot", spec.Slot)
	if err != nil {
		return Outcome{}, nil, err
	}

	command := protocol.DropStack{Sequence: sequence, Container: ref, View: view, Slot: slot}
	fields := domainCommandInventoryNormalizeSource(sequence, view, ref, slot)
	category, rule := domainCommandInventorySplitRule(view, ref, slot, slot)
	return domainCommandInventoryFinish(c, "drop-stack", command, fields, category, rule, command.Validate() == nil)
}

// domainCommandInventoryRunChatIntent admits one chat command through the
// bounded text rule. The text is retained verbatim, so the recorded outcome
// carries the exact string the case named and no derived addressing.
func domainCommandInventoryRunChatIntent(c CaseSpec, spec domainCommandInventoryInput) (Outcome, []byte, error) {
	if spec.Text == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: chat-intent requires text", c.ID)
	}

	command := protocol.ChatCommand{Text: *spec.Text}
	category, rule := domainCommandInventoryChatRule(command.Text)
	return domainCommandInventoryFinish(c, "chat-intent", command, map[string]any{"text": command.Text}, category, rule, command.Validate() == nil)
}

// domainCommandInventoryFinish records one command admission.
//
// The classification is cross-checked against the authority's verdict, a
// rejection publishes the broken rule beside its category, and an admitted
// command is round-tripped through the production codec so the recorded outcome
// carries the exact wire payload the encoder produced.
func domainCommandInventoryFinish(c CaseSpec, subject string, packet protocol.ClientPacket, fields map[string]any, category, rule string, admitted bool) (Outcome, []byte, error) {
	if admitted == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with the protocol validator (admitted=%v)", c.ID, rule, admitted)
	}
	if !admitted {
		fields["rule"] = rule
		return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
	}
	wire, err := domainCommandInventoryRoundTrip(c, packet, fields)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields["wire"] = wire
	return Outcome{Kind: "ok", Category: subject, Fields: fields}, nil, nil
}

// domainCommandInventoryRoundTrip encodes one admitted command through the
// production codec and decodes it back, proving the wire layout carries every
// field the DTO validated. The encode and decode paths both run the production
// validation, so a payload the DTO admits but the wire cannot represent fails
// here instead of being recorded as accepted.
func domainCommandInventoryRoundTrip(c CaseSpec, packet protocol.ClientPacket, fields map[string]any) (string, error) {
	instance, err := codec.NewCodec()
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: case %s: create codec: %w", c.ID, err)
	}
	defer func() { _ = instance.Close() }()

	packetID, payload, err := instance.EncodeClient(protocol.StatePlay, packet)
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: case %s: encode command: %w", c.ID, err)
	}
	decoded, err := instance.DecodeClient(protocol.StatePlay, packetID, payload)
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: case %s: decode command: %w", c.ID, err)
	}
	roundTripped, err := domainCommandInventoryNormalizePacket(decoded)
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: case %s: normalize decoded command: %w", c.ID, err)
	}
	if !reflect.DeepEqual(roundTripped, fields) {
		return "", fmt.Errorf("runtime-oracle: case %s: codec round-trip changed the normalized command: got %#v, want %#v", c.ID, roundTripped, fields)
	}
	return hex.EncodeToString(payload), nil
}

// domainCommandInventoryNormalizePacket renders one decoded command in the
// normalized field map, so a decoded payload is compared field by field against
// the command that was encoded.
func domainCommandInventoryNormalizePacket(packet protocol.ClientPacket) (map[string]any, error) {
	switch command := packet.(type) {
	case protocol.MoveInventoryStack:
		return domainCommandInventoryNormalizeMove(command.Sequence, command.From, command.To), nil
	case protocol.MoveCraftingStack:
		return domainCommandInventoryNormalizeMove(command.Sequence, command.From, command.To), nil
	case protocol.MoveContainerStack:
		return domainCommandInventoryNormalizeContainerMove(command.Sequence, command.Container, command.From, command.To), nil
	case protocol.CloseContainer:
		return domainCommandInventoryNormalizeSequenceOnly(command.Sequence), nil
	case protocol.DropSelectedItem:
		return domainCommandInventoryNormalizeSequenceOnly(command.Sequence), nil
	case protocol.TakeCraftingOutput:
		return domainCommandInventoryNormalizeSequenceOnly(command.Sequence), nil
	case protocol.EquipArmor:
		return domainCommandInventoryNormalizeSequenceOnly(command.Sequence), nil
	case protocol.MoveStackPartial:
		return domainCommandInventoryNormalizeSplit(command.Sequence, command.View, command.Container, command.From, command.To, command.Single), nil
	case protocol.QuickMoveStack:
		return domainCommandInventoryNormalizeSource(command.Sequence, command.View, command.Container, command.From), nil
	case protocol.DropStack:
		return domainCommandInventoryNormalizeSource(command.Sequence, command.View, command.Container, command.Slot), nil
	case protocol.ChatCommand:
		return map[string]any{"text": command.Text}, nil
	default:
		return nil, fmt.Errorf("decoded packet %T is not a command this family executes", packet)
	}
}

// domainCommandInventoryNormalizeMove renders one two-slot move. The u64
// sequence is a decimal string, matching the other domain families.
func domainCommandInventoryNormalizeMove(sequence uint64, from, to uint8) map[string]any {
	return map[string]any{
		"sequence": strconv.FormatUint(sequence, 10),
		"from":     int(from),
		"to":       int(to),
	}
}

// domainCommandInventoryNormalizeContainerMove renders one container move,
// flattening the reference into its raw wire fields so the normalized record
// stays comparable with the values family's container-reference rows.
func domainCommandInventoryNormalizeContainerMove(sequence uint64, ref core.ContainerRef, from, to uint8) map[string]any {
	fields := domainCommandInventoryNormalizeRef(ref)
	fields["sequence"] = strconv.FormatUint(sequence, 10)
	fields["from"] = int(from)
	fields["to"] = int(to)
	return fields
}

// domainCommandInventoryNormalizeSplit renders one partial move. The view and
// the reference are recorded beside the two indices and the single flag, so the
// looser rules this command publishes stay observable in the frozen evidence.
func domainCommandInventoryNormalizeSplit(sequence uint64, view uint8, ref core.ContainerRef, from, to uint8, single bool) map[string]any {
	fields := domainCommandInventoryNormalizeView(view, ref)
	fields["sequence"] = strconv.FormatUint(sequence, 10)
	fields["from"] = int(from)
	fields["to"] = int(to)
	fields["single"] = single
	return fields
}

// domainCommandInventoryNormalizeSource renders one command that addresses a
// single slot and carries no destination.
func domainCommandInventoryNormalizeSource(sequence uint64, view uint8, ref core.ContainerRef, slot uint8) map[string]any {
	fields := domainCommandInventoryNormalizeView(view, ref)
	fields["sequence"] = strconv.FormatUint(sequence, 10)
	fields["slot"] = int(slot)
	return fields
}

// domainCommandInventoryNormalizeSequenceOnly renders one command whose entire
// payload is the sequence.
func domainCommandInventoryNormalizeSequenceOnly(sequence uint64) map[string]any {
	return map[string]any{"sequence": strconv.FormatUint(sequence, 10)}
}

// domainCommandInventoryNormalizeView renders the view number and the reference
// one split command carries. A non-container view records an all-zero
// reference, which is the wire's sentinel form and the only form the encoder
// can write for it; the domain crate expresses the same absence with its
// `StackView` variant instead.
func domainCommandInventoryNormalizeView(view uint8, ref core.ContainerRef) map[string]any {
	fields := domainCommandInventoryNormalizeRef(ref)
	fields["view"] = int(view)
	return fields
}

// domainCommandInventoryNormalizeRef renders one container reference as its raw
// wire fields.
func domainCommandInventoryNormalizeRef(ref core.ContainerRef) map[string]any {
	return map[string]any{
		"chunk_x":        int(ref.Chunk.X),
		"chunk_z":        int(ref.Chunk.Z),
		"dimension":      int(ref.Dimension),
		"kind":           domainCommandInventoryKindText(ref.Kind),
		"container_slot": int(ref.Slot),
		"generation":     int(ref.Generation),
	}
}

// domainCommandInventoryKindText renders one container kind as the stable text
// the normalized corpus vocabulary uses. An unknown kind is rejected before a
// command is published, so the text never has to name one.
func domainCommandInventoryKindText(kind core.ContainerKind) string {
	switch kind {
	case core.ContainerKindFurnace:
		return "furnace"
	case core.ContainerKindChest:
		return "chest"
	default:
		return "unknown"
	}
}

// domainCommandInventoryInventoryMoveRule names the first rule an inventory
// move breaks, in the same order the DTO checks them: the slot range, then the
// distinct-slot rule.
func domainCommandInventoryInventoryMoveRule(from, to uint8) (string, string) {
	if from >= core.InventorySlots || to >= core.InventorySlots {
		return "invalid-value", "move_inventory.slot_range"
	}
	if from == to {
		return "invalid-value", "move_inventory.same_slot"
	}
	return "", ""
}

// domainCommandInventoryCraftingMoveRule names the first rule a crafting move
// breaks. The both-ends-in-inventory rule is the one the partial move does not
// publish, so it is pinned here and only here.
func domainCommandInventoryCraftingMoveRule(from, to uint8) (string, string) {
	if from >= protocol.GridCraftingViewSlots || to >= protocol.GridCraftingViewSlots {
		return "invalid-value", "move_crafting.slot_range"
	}
	if from == to {
		return "invalid-value", "move_crafting.same_slot"
	}
	if from >= core.CraftingGridSlots && to >= core.CraftingGridSlots {
		return "invalid-value", "move_crafting.both_ends_in_inventory"
	}
	return "", ""
}

// domainCommandInventoryContainerMoveRule names the first rule a container move
// breaks. The reference is checked first, exactly as the DTO does, so a
// malformed reference is never reported as a slot problem.
func domainCommandInventoryContainerMoveRule(ref core.ContainerRef, from, to uint8) (string, string) {
	if category, rule := domainCommandInventoryRefRule(ref); rule != "" {
		return category, rule
	}
	if from == to {
		return "invalid-value", "move_container.same_slot"
	}
	switch ref.Kind {
	case core.ContainerKindFurnace:
		if from >= core.FurnaceViewSlots || to >= core.FurnaceViewSlots {
			return "invalid-value", "move_container.slot_range"
		}
		if to == core.FurnaceOutputSlot {
			return "invalid-value", "move_container.furnace_output_target"
		}
	case core.ContainerKindChest:
		if from >= core.ChestViewSlots || to >= core.ChestViewSlots {
			return "invalid-value", "move_container.slot_range"
		}
	}
	return "", ""
}

// domainCommandInventoryTakeOutputRule names the one rule the take-crafting-
// output command publishes: a zero sequence cannot take part in acknowledgement.
func domainCommandInventoryTakeOutputRule(sequence uint64) (string, string) {
	if sequence == 0 {
		return "invalid-value", "take_crafting_output.zero_sequence"
	}
	return "", ""
}

// domainCommandInventoryPartialMoveRule names the first rule a partial move
// breaks. It is the shared split bound plus the distinct-slot rule the partial
// command adds on top of it, and it deliberately omits both stricter rules the
// whole-stack commands publish, because the protocol does not publish them for
// this command.
func domainCommandInventoryPartialMoveRule(view uint8, ref core.ContainerRef, from, to uint8) (string, string) {
	if category, rule := domainCommandInventorySplitRule(view, ref, from, to); rule != "" {
		return category, rule
	}
	if from == to {
		return "invalid-value", "move_stack_partial.same_slot"
	}
	return "", ""
}

// domainCommandInventorySplitRule names the first rule one split command
// breaks. It is the shared static bound of the partial, quick-move and
// drop-stack commands: the view domain, the reference that has to match it, and
// the index bound the view dispatches. It deliberately omits both stricter
// rules, because the protocol does not publish them for these commands.
func domainCommandInventorySplitRule(view uint8, ref core.ContainerRef, from, to uint8) (string, string) {
	switch view {
	case protocol.StackViewInventory:
		if ref != (core.ContainerRef{}) {
			return "invalid-value", "stack_split.inventory_view_carries_container"
		}
		if from >= core.InventorySlots || to >= core.InventorySlots {
			return "invalid-value", "stack_split.slot_range"
		}
	case protocol.StackViewCrafting:
		if ref != (core.ContainerRef{}) {
			return "invalid-value", "stack_split.crafting_view_carries_container"
		}
		if from >= protocol.GridCraftingViewSlots || to >= protocol.GridCraftingViewSlots {
			return "invalid-value", "stack_split.slot_range"
		}
	case protocol.StackViewContainer:
		if category, rule := domainCommandInventoryRefRule(ref); rule != "" {
			return category, rule
		}
		switch ref.Kind {
		case core.ContainerKindFurnace:
			if from >= core.FurnaceViewSlots || to >= core.FurnaceViewSlots {
				return "invalid-value", "stack_split.slot_range"
			}
		case core.ContainerKindChest:
			if from >= core.ChestViewSlots || to >= core.ChestViewSlots {
				return "invalid-value", "stack_split.slot_range"
			}
		}
	default:
		return "invalid-enum", "stack_split.view"
	}
	return "", ""
}

// domainCommandInventoryRefRule names the first rule a container reference
// breaks, mirroring the protocol's container-neutral validator: the kind has to
// be a known one, the dimension has to be the overworld, the array slot has to
// fit the kind's fixed per-chunk array, and the generation may not be zero.
func domainCommandInventoryRefRule(ref core.ContainerRef) (string, string) {
	switch ref.Kind {
	case core.ContainerKindFurnace:
		if ref.Dimension != core.Overworld {
			return "invalid-enum", "container_ref.dimension"
		}
		if ref.Slot >= core.FurnacesPerChunk {
			return "invalid-value", "container_ref.slot_range"
		}
		if ref.Generation == 0 {
			return "invalid-value", "container_ref.generation"
		}
		return "", ""
	case core.ContainerKindChest:
		if ref.Dimension != core.Overworld {
			return "invalid-enum", "container_ref.dimension"
		}
		if ref.Slot >= core.ChestsPerChunk {
			return "invalid-value", "container_ref.slot_range"
		}
		if ref.Generation == 0 {
			return "invalid-value", "container_ref.generation"
		}
		return "", ""
	default:
		return "invalid-enum", "container_ref.kind"
	}
}

// domainCommandInventoryChatRule names the first rule the chat command text
// breaks, in the same order the protocol validator checks them: the byte range
// and the surrounding whitespace, then the control characters.
func domainCommandInventoryChatRule(text string) (string, string) {
	if len(text) < 1 || len(text) > protocol.ChatCommandTextMaxBytes || !utf8.ValidString(text) || strings.TrimSpace(text) != text {
		return "invalid-value", "chat_command.text_bounds"
	}
	for _, r := range text {
		if r == 0 || unicode.IsControl(r) {
			return "invalid-value", "chat_command.control_character"
		}
	}
	return "", ""
}

// domainCommandInventorySequence resolves the sequence field one case names.
func domainCommandInventorySequence(c CaseSpec, value *uint64) (uint64, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires a sequence", c.ID)
	}
	return *value, nil
}

// domainCommandInventorySlot resolves one unified slot index. The DTO's own
// range check decides whether the value is legal.
func domainCommandInventorySlot(c CaseSpec, name string, value *uint8) (uint8, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	return *value, nil
}

// domainCommandInventoryView resolves the raw view number one case names.
func domainCommandInventoryView(c CaseSpec, value *uint8) (uint8, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires a view", c.ID)
	}
	return *value, nil
}

// domainCommandInventoryFlag resolves one boolean payload field. A false flag
// is a value, not an absence, so the pointer is required rather than defaulted.
func domainCommandInventoryFlag(c CaseSpec, name string, value *bool) (bool, error) {
	if value == nil {
		return false, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	return *value, nil
}

// domainCommandInventoryRef resolves the raw container reference one case
// names. A case that names no reference at all carries the all-zero form, which
// is what a non-container view has to carry on the wire; the kind is resolved
// through the same table the codec writes, so an unknown kind reaches the DTO
// instead of being filtered out here.
func domainCommandInventoryRef(c CaseSpec, spec domainCommandInventoryInput) (core.ContainerRef, error) {
	if spec.Kind == nil {
		return core.ContainerRef{}, nil
	}
	if spec.ChunkX == nil || spec.ChunkZ == nil {
		return core.ContainerRef{}, fmt.Errorf("runtime-oracle: case %s names a container kind without chunk coordinates", c.ID)
	}
	if spec.ContainerSlot == nil {
		return core.ContainerRef{}, fmt.Errorf("runtime-oracle: case %s names a container kind without an array slot", c.ID)
	}
	if spec.Generation == nil {
		return core.ContainerRef{}, fmt.Errorf("runtime-oracle: case %s names a container kind without a generation", c.ID)
	}
	kind, known := domainCommandInventoryKind(*spec.Kind)
	if !known {
		// An unknown kind is one of the boundaries this family pins, so it is
		// written as a raw byte the DTO has to reject rather than as a value the
		// resolver refuses to build.
		return core.ContainerRef{
			Dimension:  core.Overworld,
			Chunk:      core.ChunkPos{X: *spec.ChunkX, Z: *spec.ChunkZ},
			Kind:       core.ContainerKind(9),
			Slot:       *spec.ContainerSlot,
			Generation: *spec.Generation,
		}, nil
	}
	return core.ContainerRef{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: *spec.ChunkX, Z: *spec.ChunkZ},
		Kind:       kind,
		Slot:       *spec.ContainerSlot,
		Generation: *spec.Generation,
	}, nil
}

// domainCommandInventoryKind resolves the container kind text one case names.
func domainCommandInventoryKind(text string) (core.ContainerKind, bool) {
	switch text {
	case "furnace":
		return core.ContainerKindFurnace, true
	case "chest":
		return core.ContainerKindChest, true
	default:
		return core.ContainerKind(0), false
	}
}

// domainCommandInventoryDecodeInput reads one frozen corpus input and rejects
// trailing content, so a producer never executes bytes the case did not name.
func domainCommandInventoryDecodeInput(data []byte) (domainCommandInventoryInput, error) {
	var spec domainCommandInventoryInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&spec); err != nil {
		return domainCommandInventoryInput{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainCommandInventoryInput{}, fmt.Errorf("trailing content after the JSON value")
	}
	return spec, nil
}

// domainCommandInventoryRecord pairs one executed case with the input and
// outcome the producer produced for it.
type domainCommandInventoryRecord struct {
	label   string
	input   domainCommandInventoryInput
	outcome Outcome
}

// domainCommandInventoryExecute runs the whole case table through the producer
// and returns one record per case in table order.
func domainCommandInventoryExecute(t *testing.T) []domainCommandInventoryRecord {
	t.Helper()

	cases := domainCommandInventoryCases()
	records := make([]domainCommandInventoryRecord, 0, len(cases))
	for _, entry := range cases {
		input, err := json.MarshalIndent(entry.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", entry.label, err)
		}
		input = append(input, '\n')
		outcome, _, err := runDomainCommandInventory(CaseSpec{ID: domainCommandInventoryCaseID(entry.label)}, input)
		if err != nil {
			t.Fatalf("execute case %s: %v", entry.label, err)
		}
		records = append(records, domainCommandInventoryRecord{label: entry.label, input: entry.input, outcome: outcome})
	}
	return records
}

// domainCommandInventorySyncCorpus writes or verifies the frozen corpus files.
//
// An ordinary run is read-only: it proves the committed files still match what
// the protocol DTOs and the codec produce now, so a drifted artifact fails
// instead of being regenerated. The explicit update flag rewrites them, which
// is the only way a frozen artifact changes.
func domainCommandInventorySyncCorpus(t *testing.T, records []domainCommandInventoryRecord) {
	t.Helper()

	root := mustRepoRoot(t)
	corpusDir := filepath.Join(root, filepath.FromSlash(domainCommandInventoryCorpusRelDir))
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

	if *updateDomainCommandInventoryCorpus {
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
			t.Fatalf("read frozen corpus case %s: %v (rerun with -update-domain-command-inventory-corpus after reviewing the change)", relative, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("frozen corpus case %s drifted from the executed protocol DTO (rerun with -update-domain-command-inventory-corpus after reviewing the change)", relative)
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
}

// domainCommandInventoryCaseID renders the manifest case identity one corpus
// label carries.
func domainCommandInventoryCaseID(label string) string {
	return domainCommandInventoryFamily + "/" + domainCommandInventoryVersion + "/" + label
}

// domainCommandInventoryWorkingManifest assembles the manifest this node
// executes inside a harness-owned temporary directory.
//
// The frozen manifest does not carry this family, because the controller merges
// manifest fragments after acceptance. The working manifest is therefore a
// family-scoped selection: `Cases` holds exactly the
// `domainCommandInventoryFamily` cases the committed corpus registers, every
// other family's case list is cleared because `Reconcile` requires each
// family's list to match the cases this selection registers for it, and the
// family itself is appended with the provenance the producer reads. Registering
// the family in the live registry is deliberately left to the manifest merge:
// adding it here would make the frozen corpus fail reconciliation as an
// uncovered family before the fragment lands.
func domainCommandInventoryWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	cases := domainCommandInventoryCorpusCases(t, root)

	cloned := Inventory{
		SchemaVersion:  frozen.SchemaVersion,
		SourceRevision: frozen.SourceRevision,
		Identities:     frozen.Identities,
		Families:       append([]Family(nil), frozen.Families...),
		Cases:          cases,
	}
	caseIDs := make([]string, 0, len(cases))
	for _, c := range cases {
		caseIDs = append(caseIDs, c.ID)
	}
	sort.Strings(caseIDs)
	sources := make([]SourceSpec, 0, len(domainCommandInventoryFamilySources))
	for _, relative := range domainCommandInventoryFamilySources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", relative, err)
		}
		sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
	}
	cloned.Families = append(cloned.Families, Family{
		ID:                domainCommandInventoryFamily,
		Kind:              "domain",
		Role:              "input",
		CurrentVersion:    domainCommandInventoryVersion,
		SupportedVersions: []string{domainCommandInventoryVersion},
		Source:            domainCommandInventorySource,
		EventualOwner:     ownerDomain,
		NumericSemantics:  domainCommandInventoryNumericSemantics,
		Sources:           sources,
		Cases:             caseIDs,
	})
	for index := range cloned.Families {
		// A family this selection does not execute registers no case, so its
		// list must be empty for the manifest to describe itself.
		if cloned.Families[index].ID != domainCommandInventoryFamily {
			cloned.Families[index].Cases = nil
		}
	}

	encoded, err := encodeInventory(cloned)
	if err != nil {
		t.Fatalf("encode working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load working manifest: %v", err)
	}
	return loaded
}

// domainCommandInventoryCorpusCases reads the frozen corpus and registers one
// case per committed input, with digests proven against the files on disk.
func domainCommandInventoryCorpusCases(t *testing.T, root string) []CaseSpec {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(domainCommandInventoryCorpusRelDir))
	var inputs []string
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".input.json") {
			inputs = append(inputs, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk command inventory corpus directory: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no command inventory case under %s", domainCommandInventoryCorpusRelDir)
	}
	sort.Strings(inputs)

	cases := make([]CaseSpec, 0, len(inputs))
	for _, input := range inputs {
		relative, relErr := filepath.Rel(dir, input)
		if relErr != nil {
			t.Fatalf("relative corpus path: %v", relErr)
		}
		label := strings.TrimSuffix(filepath.ToSlash(relative), ".input.json")
		if label == "" || !domainCommandInventoryLabelPattern.MatchString(label) {
			t.Fatalf("corpus case %s has label %q, which is not a lowercase slug with an optional numeric suffix", filepath.ToSlash(relative), label)
		}
		envelope := domainCommandInventoryReadEnvelope(t, input)
		if envelope.Consumer != domainCommandInventoryConsumer {
			t.Fatalf("corpus case %s names consumer %q, want %q", label, envelope.Consumer, domainCommandInventoryConsumer)
		}
		if strings.TrimSpace(envelope.Rule) == "" {
			t.Fatalf("corpus case %s names no rule", label)
		}

		inputHash, err := hashFile(input)
		if err != nil {
			t.Fatalf("hash %s: %v", label, err)
		}
		expectedPath := strings.TrimSuffix(input, ".input.json") + ".expected.json"
		expectedHash, err := hashFile(expectedPath)
		if err != nil {
			t.Fatalf("hash %s: %v", label+".expected.json", err)
		}
		cases = append(cases, CaseSpec{
			ID:           domainCommandInventoryCaseID(label),
			Family:       domainCommandInventoryFamily,
			Version:      domainCommandInventoryVersion,
			Operation:    domainCommandInventoryOperation,
			Input:        AssetRef{Path: domainCommandInventoryCorpusPath(root, input), SHA256: inputHash},
			InputFormat:  "json",
			Expected:     AssetRef{Path: domainCommandInventoryCorpusPath(root, expectedPath), SHA256: expectedHash},
			Checkpoints:  []string{"0"},
			RustConsumer: domainCommandInventoryConsumer,
		})
	}
	return cases
}

// domainCommandInventoryReadEnvelope reads the provenance envelope of one
// frozen corpus input.
func domainCommandInventoryReadEnvelope(t *testing.T, path string) domainCommandInventoryInput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var envelope domainCommandInventoryInput
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return envelope
}

// domainCommandInventoryCorpusPath renders one absolute corpus path as the
// repository-relative slash path the manifest requires.
func domainCommandInventoryCorpusPath(root, absolute string) string {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return absolute
	}
	return filepath.ToSlash(relative)
}

// TestDomainCommandInventoryOracleExecutesEveryCase runs the whole case table
// through the real Go protocol DTOs and the production codec, proves the frozen
// corpus still matches what they produce, and then runs the same cases through
// the production runner so the executed evidence satisfies the completeness
// rules a published trace report does.
func TestDomainCommandInventoryOracleExecutesEveryCase(t *testing.T) {
	records := domainCommandInventoryExecute(t)
	domainCommandInventorySyncCorpus(t, records)

	root := mustRepoRoot(t)
	manifest := domainCommandInventoryWorkingManifest(t, root)
	if len(manifest.Cases) != len(records) {
		t.Fatalf("working manifest registers %d cases, want %d (one per executed table row)", len(manifest.Cases), len(records))
	}
	domainCommandInventoryAssertCaseSpecs(t, root, manifest)
	domainCommandInventoryAssertOutcomesDistinguishCases(t, records)

	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(observations) != len(manifest.Cases) {
		t.Fatalf("produced %d observations, want %d", len(observations), len(manifest.Cases))
	}
	for _, obs := range observations {
		c := domainCommandInventoryCaseByID(t, manifest, obs.CaseID)
		if obs.ExpectedDigest != c.Expected.SHA256 {
			t.Fatalf("observation for %s carries expected digest %s, want %s", obs.CaseID, obs.ExpectedDigest, c.Expected.SHA256)
		}
		expected := readExpectedOutcome(t, root, c)
		if !outcomesEqual(obs.Outcome, expected) {
			t.Fatalf("case %s produced %#v, want %#v", obs.CaseID, obs.Outcome, expected)
		}
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTrace(trace, manifest); err != nil {
		t.Fatalf("executed command inventory evidence failed trace validation: %v", err)
	}
}

// TestDomainCommandInventoryOracleCaseIdentitiesAreTheRustDomainConsumer pins
// the manifest identity of every inventory command case: the operation, the
// version, the family and the consumer the change names for the Rust crate that
// owns these rules.
func TestDomainCommandInventoryOracleCaseIdentitiesAreTheRustDomainConsumer(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainCommandInventoryWorkingManifest(t, root)

	labels := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Operation != domainCommandInventoryOperation {
			t.Fatalf("case %s declares operation %q, want %q", c.ID, c.Operation, domainCommandInventoryOperation)
		}
		if c.Version != domainCommandInventoryVersion {
			t.Fatalf("case %s declares version %q, want %q", c.ID, c.Version, domainCommandInventoryVersion)
		}
		if c.RustConsumer != domainCommandInventoryConsumer {
			t.Fatalf("case %s declares consumer %q, want %q", c.ID, c.RustConsumer, domainCommandInventoryConsumer)
		}
		if c.InputFormat != "json" {
			t.Fatalf("case %s declares input_format %q, want json", c.ID, c.InputFormat)
		}
		prefix := c.Family + "/" + c.Version + "/"
		if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
			t.Fatalf("case %s does not match %s<label>", c.ID, prefix)
		}
		label := strings.TrimPrefix(c.ID, prefix)
		if !domainCommandInventoryLabelPattern.MatchString(label) {
			t.Fatalf("case %s label %q is not a lowercase slug with an optional numeric suffix", c.ID, label)
		}
		if labels[label] {
			t.Fatalf("label %q is registered twice", label)
		}
		labels[label] = true

		envelope := domainCommandInventoryReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		if envelope.Consumer != domainCommandInventoryConsumer {
			t.Fatalf("case %s was produced for consumer %q, want %q", c.ID, envelope.Consumer, domainCommandInventoryConsumer)
		}
	}
}

// TestDomainCommandInventoryOracleWorkingManifestDescribesItself proves the
// working manifest is internally consistent without claiming a registry entry
// the frozen corpus does not carry yet: every case passes the production case
// validation, the family's provenance hashes match disk, and the family's case
// list matches exactly the cases the selection registers.
func TestDomainCommandInventoryOracleWorkingManifestDescribesItself(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainCommandInventoryWorkingManifest(t, root)
	domainCommandInventoryAssertCaseSpecs(t, root, manifest)

	family, ok := domainCommandInventoryFamilySpec(manifest)
	if !ok {
		t.Fatalf("working manifest has no %s family", domainCommandInventoryFamily)
	}
	if len(family.Sources) != len(domainCommandInventoryFamilySources) {
		t.Fatalf("%s records %d provenance sources, want %d", domainCommandInventoryFamily, len(family.Sources), len(domainCommandInventoryFamilySources))
	}
	for _, source := range family.Sources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(source.Path)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", source.Path, err)
		}
		if hash != source.SHA256 {
			t.Fatalf("provenance source %s records %s, disk has %s", source.Path, source.SHA256, hash)
		}
	}

	registered := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		registered[c.ID] = true
	}
	if len(family.Cases) != len(registered) {
		t.Fatalf("%s lists %d cases, want %d", domainCommandInventoryFamily, len(family.Cases), len(registered))
	}
	for _, id := range family.Cases {
		if !registered[id] {
			t.Fatalf("%s lists case %s which the selection does not register", domainCommandInventoryFamily, id)
		}
	}
}

// TestDomainCommandInventoryOracleOutcomesDistinguishAcceptedAndRejected pins
// that the producer is not returning one constant answer, that every rule
// records both an admitted command and a rejection, and that the boundaries the
// Rust command test enumerates are each present in the executed evidence.
func TestDomainCommandInventoryOracleOutcomesDistinguishAcceptedAndRejected(t *testing.T) {
	records := domainCommandInventoryExecute(t)
	domainCommandInventoryAssertOutcomesDistinguishCases(t, records)

	tally := make(map[string]*domainCommandInventoryRuleTally)
	for _, record := range records {
		entry, ok := tally[record.input.Rule]
		if !ok {
			entry = &domainCommandInventoryRuleTally{}
			tally[record.input.Rule] = entry
		}
		switch record.outcome.Kind {
		case "ok":
			entry.accepted = append(entry.accepted, record.outcome.Fields)
		case "error":
			entry.rejected = append(entry.rejected, record.outcome.Fields)
		}
	}
	// The rules that publish a payload bound have to record both an admitted
	// command and a rejection, so a boundary cannot be pinned by one side only.
	boundedRules := []string{
		"move-inventory",
		"move-crafting",
		"move-container",
		"take-crafting-output",
		"move-partial",
		"quick-move",
		"drop-stack",
		"chat-intent",
	}
	for _, rule := range boundedRules {
		entry, ok := tally[rule]
		if !ok {
			t.Fatalf("rule %s records no executed case", rule)
		}
		if len(entry.accepted) == 0 || len(entry.rejected) == 0 {
			t.Fatalf("rule %s records %d accepted and %d rejected cases, want both non-zero",
				rule, len(entry.accepted), len(entry.rejected))
		}
	}
	// The remaining rules are total: their DTO publishes no payload bound, so a
	// rejection would mean the protocol tightened without this family noticing.
	totalRules := []string{"close-container", "drop-selected-item", "equip-armor"}
	for _, rule := range totalRules {
		entry, ok := tally[rule]
		if !ok {
			t.Fatalf("rule %s records no executed case", rule)
		}
		if len(entry.accepted) == 0 {
			t.Fatalf("rule %s records no admitted case, so the total command is not pinned", rule)
		}
		if len(entry.rejected) != 0 {
			t.Fatalf("rule %s records %d rejected cases, want none: the command publishes no payload bound",
				rule, len(entry.rejected))
		}
	}

	if !domainCommandInventoryFieldsContain(tally["move-inventory"].accepted, "from", 35) {
		t.Fatal("no admitted inventory move starts at slot 35, which is inside the range")
	}
	if !domainCommandInventoryFieldsContain(tally["move-inventory"].rejected, "from", 36) {
		t.Fatal("no rejected inventory move starts at slot 36, which is outside the range")
	}
	if !domainCommandInventoryFieldsContain(tally["move-crafting"].accepted, "from", 8) ||
		!domainCommandInventoryFieldsContain(tally["move-crafting"].accepted, "to", 44) {
		t.Fatal("no admitted crafting move crosses from the grid to the last backpack slot")
	}
	if !domainCommandInventoryRejectionNamesRule(tally, "move_crafting.both_ends_in_inventory") {
		t.Fatal("no rejection names the crafting both-ends rule, so the stricter rule is not pinned")
	}
	if !domainCommandInventoryRejectionNamesRule(tally, "move_container.furnace_output_target") {
		t.Fatal("no rejection names the furnace output target rule, so the reserved slot is not pinned")
	}
	if !domainCommandInventoryFieldsContain(tally["move-container"].accepted, "from", 38) {
		t.Fatal("no admitted container move starts at the furnace output slot, which is a legal source")
	}
	if !domainCommandInventoryFieldsContain(tally["move-container"].accepted, "to", 62) {
		t.Fatal("no admitted container move lands on chest slot 62, which is the last chest view slot")
	}
	// The two rules the whole-stack commands publish but the split commands do
	// not are the point of this family: the partial rows record the looser
	// verdicts, so a future tightening cannot pass unnoticed.
	if !domainCommandInventoryFieldsContain(tally["move-partial"].accepted, "from", 9) ||
		!domainCommandInventoryFieldsContain(tally["move-partial"].accepted, "to", 10) {
		t.Fatal("no admitted partial move has both ends inside the inventory region, which the protocol admits")
	}
	if !domainCommandInventoryFieldsContain(tally["move-partial"].accepted, "to", 38) {
		t.Fatal("no admitted partial move lands on the furnace output slot, which the protocol admits")
	}
	if !domainCommandInventoryRejectionNamesRule(tally, "container_ref.slot_range") {
		t.Fatal("no rejection names the container reference slot rule, so the reference-first order is not pinned")
	}
	for _, single := range []bool{false, true} {
		if !domainCommandInventoryFieldsContain(tally["move-partial"].accepted, "single", single) {
			t.Fatalf("no admitted partial move records single=%v, so the split flag is not preserved", single)
		}
	}
	if !domainCommandInventoryFieldsContain(tally["chat-intent"].accepted, "text", "@Bob 停止") {
		t.Fatal("no admitted chat intent retains the mention verbatim")
	}
	if !domainCommandInventoryRejectionNamesRule(tally, "take_crafting_output.zero_sequence") {
		t.Fatal("no rejection names the take-crafting-output zero sequence rule")
	}

	for _, entry := range tally {
		for _, fields := range entry.accepted {
			if _, ok := fields["wire"].(string); !ok {
				t.Fatalf("rule %s publishes an admitted command with no codec round-trip payload", entry.rule)
			}
		}
		for _, fields := range entry.rejected {
			if _, ok := fields["wire"]; ok {
				t.Fatalf("rule %s publishes a rejection carrying codec round-trip bytes", entry.rule)
			}
		}
	}
}

// TestDomainCommandInventoryOracleRunnerRejectsUnregisteredFamily pins that a
// manifest naming a family this package cannot execute fails the run.
func TestDomainCommandInventoryOracleRunnerRejectsUnregisteredFamily(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainCommandInventoryWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Family = "domain.unknown"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "family domain.unknown has no registered Go producer") {
		t.Fatalf("expected an unregistered-family failure, got: %v", err)
	}
}

// TestDomainCommandInventoryOracleRunnerRejectsOperationFamilyMismatch pins
// that a case cannot declare one operation and be executed by another.
func TestDomainCommandInventoryOracleRunnerRejectsOperationFamilyMismatch(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainCommandInventoryWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Operation = "decode"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "declares operation") {
		t.Fatalf("expected an operation/family mismatch failure, got: %v", err)
	}
}

// TestDomainCommandInventoryOracleRunnerRejectsTamperedInput pins that a
// producer never executes bytes the manifest does not name.
func TestDomainCommandInventoryOracleRunnerRejectsTamperedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainCommandInventoryWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// TestDomainCommandInventoryOracleReportPublishesAndValidates assembles the
// executed evidence into a report, publishes it through the production atomic
// exporter, reloads it and validates it against the working manifest. This is
// the identity and content check a later Rust acceptance step performs; it does
// not claim any Rust behaviour.
func TestDomainCommandInventoryOracleReportPublishesAndValidates(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainCommandInventoryWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTrace(trace, manifest); err != nil {
		t.Fatalf("executed command inventory evidence failed trace validation: %v", err)
	}

	workspace, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	defer cleanup()
	target := filepath.Join(workspace, domainCommandInventoryCorpusReportName)
	if err := ExportTrace(root, target, trace, manifest); err != nil {
		t.Fatalf("ExportTrace: %v", err)
	}
	loaded, err := LoadTrace(target, manifest)
	if err != nil {
		t.Fatalf("published report does not validate: %v", err)
	}
	if loaded.SchemaVersion != traceSchemaVersion {
		t.Fatalf("report schema_version = %d, want %d", loaded.SchemaVersion, traceSchemaVersion)
	}
	if loaded.SourceRevision != manifest.SourceRevision {
		t.Fatalf("report source revision = %s, want %s", loaded.SourceRevision, manifest.SourceRevision)
	}
	if len(loaded.Inputs) != len(manifest.Cases) || len(loaded.Observations) != len(manifest.Cases) {
		t.Fatalf("report carries %d inputs and %d observations, want %d each",
			len(loaded.Inputs), len(loaded.Observations), len(manifest.Cases))
	}
}

// TestDomainCommandInventoryOracleRejectsMissingProducerTest pins that the
// executed evidence has a real producer behind it, including the topic-named
// entry point the domain plan's filter selects. A corpus whose producer test is
// gone has no independent execution, only frozen files, and a missing topic
// entry point would make the plan filter pass without running anything.
func TestDomainCommandInventoryOracleRejectsMissingProducerTest(t *testing.T) {
	root := mustRepoRoot(t)
	path := filepath.Join(root, filepath.FromSlash(domainCommandInventoryProducerTestRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read producer test %s: %v", domainCommandInventoryProducerTestRelPath, err)
	}
	for _, name := range []string{domainCommandInventoryProducerExecuteName, domainCommandInventoryProducerTopicName} {
		declaration := "func " + name + "(t *testing.T) {"
		if !strings.Contains(string(data), declaration) {
			t.Fatalf("%s does not declare %s", domainCommandInventoryProducerTestRelPath, declaration)
		}
	}
}

// TestDomainOracle_command_inventory is the topic-named entry point the domain
// plan names for this node. It delegates to the same executed table, so the two
// filters select one source of expected results rather than two.
func TestDomainOracle_command_inventory(t *testing.T) {
	TestDomainCommandInventoryOracleExecutesEveryCase(t)
}

// domainCommandInventoryRuleTally collects the accepted and rejected normalized
// outcomes one rule records, so a boundary assertion can look for the field
// value it names.
type domainCommandInventoryRuleTally struct {
	rule     string
	accepted []map[string]any
	rejected []map[string]any
}

// domainCommandInventoryFieldsContain reports whether one recorded outcome set
// holds a record whose named field carries the value a boundary assertion
// names.
func domainCommandInventoryFieldsContain(records []map[string]any, field string, want any) bool {
	for _, fields := range records {
		if reflect.DeepEqual(fields[field], want) {
			return true
		}
	}
	return false
}

// domainCommandInventoryRejectionNamesRule reports whether any recorded
// rejection names the broken rule a boundary assertion pins.
func domainCommandInventoryRejectionNamesRule(tally map[string]*domainCommandInventoryRuleTally, rule string) bool {
	for _, entry := range tally {
		for _, fields := range entry.rejected {
			if fields["rule"] == rule {
				return true
			}
		}
	}
	return false
}

// domainCommandInventoryAssertCaseSpecs runs the production case validation
// over the whole selection, so a case with a bad path, digest, format or
// checkpoint fails here rather than surfacing later as a confusing coverage
// failure.
func domainCommandInventoryAssertCaseSpecs(t *testing.T, root string, manifest Inventory) {
	t.Helper()
	families := make(map[string]Family, len(manifest.Families))
	for _, family := range manifest.Families {
		families[family.ID] = family
	}
	for _, c := range manifest.Cases {
		if err := validateCaseSpec(root, c, families); err != nil {
			t.Fatalf("case %s: %v", c.ID, err)
		}
	}
}

// domainCommandInventoryAssertOutcomesDistinguishCases proves the executed
// evidence separates admitted commands from rejections instead of publishing
// one constant answer, and that the rejections name a rule the authority
// publishes.
func domainCommandInventoryAssertOutcomesDistinguishCases(t *testing.T, records []domainCommandInventoryRecord) {
	t.Helper()
	if len(records) == 0 {
		t.Fatal("executed evidence records no case")
	}
	accepted, rejected := 0, 0
	for _, record := range records {
		switch record.outcome.Kind {
		case "ok":
			accepted++
			if record.outcome.Category == "" {
				t.Fatalf("record %s publishes an accepted outcome with no category", record.label)
			}
		case "error":
			rejected++
			if !corpusStructuralCategories[record.outcome.Category] &&
				!corpusAdmissionCategories[record.outcome.Category] &&
				!corpusStorageCategories[record.outcome.Category] {
				t.Fatalf("record %s publishes rejection category %q, which is not in the frozen vocabulary", record.label, record.outcome.Category)
			}
			if record.outcome.Fields["rule"] == nil {
				t.Fatalf("record %s publishes a rejection with no rule name", record.label)
			}
		default:
			t.Fatalf("record %s publishes kind %q, want ok or error", record.label, record.outcome.Kind)
		}
	}
	if accepted == 0 || rejected == 0 {
		t.Fatalf("executed evidence records %d accepted and %d rejected cases, want both non-zero", accepted, rejected)
	}
}

// domainCommandInventoryFamilySpec resolves one family from a manifest
// selection.
func domainCommandInventoryFamilySpec(manifest Inventory) (Family, bool) {
	for _, family := range manifest.Families {
		if family.ID == domainCommandInventoryFamily {
			return family, true
		}
	}
	return Family{}, false
}

// domainCommandInventoryCaseByID indexes a manifest selection by case identity.
func domainCommandInventoryCaseByID(t *testing.T, manifest Inventory, id string) CaseSpec {
	t.Helper()
	for _, c := range manifest.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %s is missing from the working manifest", id)
	return CaseSpec{}
}
