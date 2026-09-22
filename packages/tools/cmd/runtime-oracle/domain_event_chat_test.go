package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
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

	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// This file is the Go producer for the domain closed-chat event family. It
// executes every committed corpus case through the current Go protocol DTO:
// the verdict comes from `protocol.ValidateServerPacket`, which is the same
// Play-state validator the codec applies on both the encode and the decode
// side, and an admitted event is normalized through the frozen field map. No
// world authority, listener, model call or native ABI is involved: the
// producer only reads immutable corpus inputs and calls in-process
// validators, and the frozen corpus files it materializes are read-only
// inputs for later consumers.
//
// The sole rule this family executes is the Go `protocol.ChatEvent` record,
// the closed chat-addressing fact an authoritative session publishes to the
// one player that caused it. The event is a semantic union rather than one
// flat record: the wire's single text slot is reused by kind, so an admitted
// event publishes the exact semantic branch it carries (`accepted`, the four
// rejection branches, the five task-fact branches, `task-failed-<reason>` or
// `speech`) and only that branch's legal data. The classifier mirrors the
// exact branch order of `ChatEvent.Validate`: the global identity and name
// failures precede kind dispatch, a non-speech kind carrying speech fails
// before the kind-specific switch, and inside the switch the reason, the
// companion identity and name, and then the command or speech text decide in
// that order.
//
// Rejection categories stay inside the frozen corpus vocabulary:
// `invalid-identity` for a zero event or player identity, `invalid-enum` for
// an unknown kind and a reserved or out-of-domain reason, and `invalid-value`
// for every text-boundary failure and every illegal cross-field combination.
// Rule names begin `chat_event.` and name the failing field or combination;
// the reserved reject reason 3 publishes `chat_event.rejected.reason`, the
// failure reasons outside 16..20 publish `chat_event.task_failed.reason`, and
// an unknown kind publishes `chat_event.kind`. A rejection retains the raw
// semantic inputs, including the unknown numeric enum value, so a replay
// consumer sees exactly the value the record carried.
//
// The frozen cases this producer materializes are not yet registered in the
// canonical manifest: registration is a later node's work, so the only
// publication path is the explicit external export through
// `RUNTIME_ORACLE_EXPORT_DIR`.

const (
	// domainEventChatFamily is the corpus family this package executes. The
	// family is the existing `domain.event` row, whose eventual owner is the
	// Rust domain crate that owns the replay observation records.
	domainEventChatFamily = "domain.event"
	// domainEventChatVersion is the family's discovered version. A case has
	// to name its family's version, so the case identities carry this
	// segment rather than one this producer chose.
	domainEventChatVersion = "1"
	// domainEventChatOperation is the manifest operation name for a chat
	// event admission case.
	domainEventChatOperation = "admit"
	// domainEventChatConsumer is the manifest consumer the change pins for
	// this family: the Rust crate that owns these records.
	domainEventChatConsumer = "mornlea_domain"
	// domainEventChatCorpusRelDir is the repository-relative directory
	// holding the frozen event chat corpus cases.
	domainEventChatCorpusRelDir = "testdata/runtime-migration/cases/domain/event_chat"
	// domainEventChatProducerTestRelPath and the two producer test names
	// locate the package-local producer that executes the protocol DTO. The
	// topic name is the entry point the domain plan's filter requires; a
	// corpus whose producer test is missing has no independently executed
	// evidence at all.
	domainEventChatProducerTestRelPath = "packages/tools/cmd/runtime-oracle/domain_event_chat_test.go"
	domainEventChatProducerExecuteName = "TestDomainEventChatOracleExecutesEveryCase"
	domainEventChatProducerTopicName   = "TestDomainOracle_event_chat"
	// domainEventChatCorpusReportName is the published report file name for
	// the executed event chat evidence.
	domainEventChatCorpusReportName = "runtime-corpus-domain-event-chat.json"
	// domainEventChatProducerID is the exporter's producer identity for this
	// family's frozen assets.
	domainEventChatProducerID = "runtime-oracle/domain-event-chat"

	// domainEventChatRuleChat is the sole rule name a case names. The family
	// is shared with the world observations, the player and outcome records,
	// the inventory and container publications, the remote-player and
	// companion observations, the hostile and passive mob observations and
	// the projectile and item-drop observations, so the rule name is the
	// discriminator the manifest and the family router both read.
	domainEventChatRuleChat = "chat"

	// domainEventChatCaseTotal is the exact size of the executed case table,
	// pinned by the outcomes test so a dropped or duplicated row fails the
	// run rather than shrinking the evidence silently.
	domainEventChatCaseTotal = 44

	// The seed values every case starts from: a valid UUIDv4 player and
	// companion, the event identity 7, the original command and the one
	// speech line the speech branch carries.
	domainEventChatSeedPlayerHex     = "00112233445546778899aabbccddeeff"
	domainEventChatSeedCompanionHex  = "2233445546674889aabbccddeeff00ff"
	domainEventChatSeedPlayerName    = "Alice"
	domainEventChatSeedCompanionName = "Buddy"
	domainEventChatSeedEventID       = "7"
	domainEventChatSeedCommand       = "gather stone"
	domainEventChatSeedSpeech        = "Ready."

	// domainEventChatZeroHex is the zero companion UUID the wire sentinel
	// carries: 32 lowercase zeros, which a raw hex decode admits and the
	// validator decides about.
	domainEventChatZeroHex = "00000000000000000000000000000000"

	// domainEventChatUntrimmedPlayerName and domainEventChatSpacedName are
	// the two canonical name failures the boundary rows pin: a leading space
	// the display-name rule trims away, and an embedded space the companion
	// name rule forbids outright.
	domainEventChatUntrimmedPlayerName = " Alice"
	domainEventChatSpacedName          = "Bud dy"
)

// domainEventChatFamilySources is the merged provenance set the family
// records. Every entry is a file the producer's rule is read from, so a
// change to any of them is a change to the recorded evidence. The companion
// message file carries the record shape, the combination validator and the
// two text-slot bounds; the registry file decides the record is published in
// the play state; the core identity file owns the player identity and the
// display-name admission rule; and the companion identity file owns the
// companion identity and the companion-name rule.
var domainEventChatFamilySources = []string{
	"packages/shared/network/protocol/message_companion.go",
	"packages/shared/network/protocol/registry.go",
	"packages/shared/core/player_id.go",
	"packages/shared/companion/identity.go",
}

// domainEventChatLabelPattern is the shape a corpus label must have: a
// lowercase slug with an optional zero-padded numeric suffix, so a boundary
// row sorts in numeric order under a lexical sort.
var domainEventChatLabelPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(-[0-9]+)?$`)

// domainEventChatExportPublished keeps the explicit export to one
// publication per test process. The topic entry delegates to the
// execute-every-case test, so one filter selecting both entries executes the
// same deterministic candidate twice, and the exporter's exclusive producer
// child would otherwise treat the second publication as a preexisting
// directory rather than as the same evidence.
var domainEventChatExportPublished bool

// domainEventChatInput is the frozen, self-describing corpus input for one
// case.
//
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the authority. Every key is present in every raw
// input, including the empty strings and the zero companion UUID, so the
// union decision never depends on an absent JSON field: the event identity is
// a decimal string because a JSON number cannot carry the full `u64` range
// losslessly, the two UUID identities are 32 lowercase hexadecimal
// characters, the kind and reason stay the Go wire values as JSON integers,
// and a rejected outcome retains them exactly as the record carried them,
// including an unknown numeric enum.
type domainEventChatInput struct {
	Consumer      string `json:"consumer"`
	Rule          string `json:"rule"`
	EventID       string `json:"event_id"`
	PlayerID      string `json:"player_id"`
	CompanionID   string `json:"companion_id"`
	PlayerName    string `json:"player_name"`
	CompanionName string `json:"companion_name"`
	Kind          int    `json:"kind"`
	Reason        int    `json:"reason"`
	Command       string `json:"command"`
	Speech        string `json:"speech"`
}

// domainEventChatCase is one frozen corpus case: its label and its input, and
// nothing else.
type domainEventChatCase struct {
	label string
	input domainEventChatInput
}

// domainEventChatBaseInput is the envelope every case starts from: the
// consumer, the chat rule, the seed player and companion identities and
// names, the event identity, the original command, an empty speech slot, and
// the named kind and reason. A branch whose legal shape differs from this
// base clears exactly the fields its branch forbids, so a boundary row is
// always one legal branch seed plus the one illegal change it names.
func domainEventChatBaseInput(kind protocol.ChatEventKind, reason protocol.ChatRejectReason) domainEventChatInput {
	return domainEventChatInput{
		Consumer:      domainEventChatConsumer,
		Rule:          domainEventChatRuleChat,
		EventID:       domainEventChatSeedEventID,
		PlayerID:      domainEventChatSeedPlayerHex,
		CompanionID:   domainEventChatSeedCompanionHex,
		PlayerName:    domainEventChatSeedPlayerName,
		CompanionName: domainEventChatSeedCompanionName,
		Kind:          int(kind),
		Reason:        int(reason),
		Command:       domainEventChatSeedCommand,
		Speech:        "",
	}
}

// domainEventChatAcceptedSeed is the accepted seed: the full companion
// identity, the valid names, reason None and the original command.
func domainEventChatAcceptedSeed() domainEventChatInput {
	return domainEventChatBaseInput(protocol.ChatEventAccepted, protocol.ChatRejectNone)
}

// domainEventChatInvalidFormatSeed is the invalid-format rejection seed: the
// exact zero companion identity plus the empty companion name, command and
// speech, because a malformed command never addressed a companion.
func domainEventChatInvalidFormatSeed() domainEventChatInput {
	input := domainEventChatBaseInput(protocol.ChatEventRejected, protocol.ChatRejectInvalidFormat)
	input.CompanionID = domainEventChatZeroHex
	input.CompanionName = ""
	input.Command = ""
	return input
}

// domainEventChatUnknownCompanionSeed is the unknown-companion rejection
// seed: the exact zero identity, the valid target name the issuing player
// can check against their spelling, and the empty command and speech.
func domainEventChatUnknownCompanionSeed() domainEventChatInput {
	input := domainEventChatBaseInput(protocol.ChatEventRejected, protocol.ChatRejectUnknownCompanion)
	input.CompanionID = domainEventChatZeroHex
	input.Command = ""
	return input
}

// domainEventChatQueueFullSeed is the queue-full rejection seed, which keeps
// the full companion identity and the command so the issuing player can
// locate the rejected instruction.
func domainEventChatQueueFullSeed() domainEventChatInput {
	return domainEventChatBaseInput(protocol.ChatEventRejected, protocol.ChatRejectQueueFull)
}

// domainEventChatNotFollowingSeed is the not-following rejection seed, which
// carries the same companion identity and command requirements as queue-full.
func domainEventChatNotFollowingSeed() domainEventChatInput {
	return domainEventChatBaseInput(protocol.ChatEventRejected, protocol.ChatRejectNotFollowing)
}

// domainEventChatTaskSeed is a task-fact seed: reason stays None, the full
// companion identity is carried and the original command is restated, for
// each of the five task kinds that publish a fact rather than a failure.
func domainEventChatTaskSeed(kind protocol.ChatEventKind) domainEventChatInput {
	return domainEventChatBaseInput(kind, protocol.ChatRejectNone)
}

// domainEventChatTaskFailedSeed is a task-failure seed: the shared reason
// slot carries the `TaskFailReason` the branch requires.
func domainEventChatTaskFailedSeed(reason protocol.TaskFailReason) domainEventChatInput {
	return domainEventChatBaseInput(protocol.ChatEventTaskFailed, protocol.ChatRejectReason(reason))
}

// domainEventChatSpeechSeed is the speech seed: the full companion identity,
// an empty command slot and the one speech line the branch carries.
func domainEventChatSpeechSeed() domainEventChatInput {
	input := domainEventChatBaseInput(protocol.ChatEventCompanionSpeech, protocol.ChatRejectNone)
	input.Command = ""
	input.Speech = domainEventChatSeedSpeech
	return input
}

// domainEventChatCase renders one row from a branch seed with exactly one
// field changed, so a boundary row is always a legal branch plus the value
// the rule names.
func domainEventChatRow(label string, seed func() domainEventChatInput, mutate func(*domainEventChatInput)) domainEventChatCase {
	input := seed()
	if mutate != nil {
		mutate(&input)
	}
	return domainEventChatCase{label: label, input: input}
}

// domainEventChatRepeatedText renders one boundary text of the named length
// from a single ASCII letter, so the boundary rows stay valid UTF-8 with no
// control characters and no trimmable whitespace; only the byte length
// changes across the boundary.
func domainEventChatRepeatedText(count int, letter byte) string {
	return string(bytes.Repeat([]byte{letter}, count))
}

// domainEventChatCases is the ordered case table the producer executes.
//
// The first sixteen rows execute every legal semantic branch once: the
// accepted event, the four rejection branches, the five task-fact branches,
// the five task-failure reasons and the speech event. The twenty-eight
// boundary rows then pin the admission edges: the zero event and player
// identities, the untrimmed player name, the reason and companion and text
// combinations each branch forbids, the reserved reject reason 3, the
// failure reasons one below and one above the published domain, the speech
// leak on a task fact, the command and emptiness rules of the speech branch,
// the unknown kind, and the 1,024-byte command and 256-byte speech bounds
// with their plus-one rejections. The two boundary rows at the exact bounds
// stay admitted; every other boundary row is a rejection.
func domainEventChatCases() []domainEventChatCase {
	cases := []domainEventChatCase{
		domainEventChatRow("chat-accepted", domainEventChatAcceptedSeed, nil),
		domainEventChatRow("chat-rejected-invalid-format", domainEventChatInvalidFormatSeed, nil),
		domainEventChatRow("chat-rejected-unknown-companion", domainEventChatUnknownCompanionSeed, nil),
		domainEventChatRow("chat-rejected-queue-full", domainEventChatQueueFullSeed, nil),
		domainEventChatRow("chat-rejected-not-following", domainEventChatNotFollowingSeed, nil),
		domainEventChatRow("chat-task-started", func() domainEventChatInput {
			return domainEventChatTaskSeed(protocol.ChatEventTaskStarted)
		}, nil),
		domainEventChatRow("chat-task-progress", func() domainEventChatInput {
			return domainEventChatTaskSeed(protocol.ChatEventTaskProgress)
		}, nil),
		domainEventChatRow("chat-task-completed", func() domainEventChatInput {
			return domainEventChatTaskSeed(protocol.ChatEventTaskCompleted)
		}, nil),
		domainEventChatRow("chat-task-timed-out", func() domainEventChatInput {
			return domainEventChatTaskSeed(protocol.ChatEventTaskTimedOut)
		}, nil),
		domainEventChatRow("chat-task-stopped", func() domainEventChatInput {
			return domainEventChatTaskSeed(protocol.ChatEventTaskStopped)
		}, nil),
		domainEventChatRow("chat-task-failed-planner-unavailable", func() domainEventChatInput {
			return domainEventChatTaskFailedSeed(protocol.TaskFailPlannerUnavailable)
		}, nil),
		domainEventChatRow("chat-task-failed-invalid-plan", func() domainEventChatInput {
			return domainEventChatTaskFailedSeed(protocol.TaskFailInvalidPlan)
		}, nil),
		domainEventChatRow("chat-task-failed-path-unreachable", func() domainEventChatInput {
			return domainEventChatTaskFailedSeed(protocol.TaskFailPathUnreachable)
		}, nil),
		domainEventChatRow("chat-task-failed-world-changed", func() domainEventChatInput {
			return domainEventChatTaskFailedSeed(protocol.TaskFailWorldChanged)
		}, nil),
		domainEventChatRow("chat-task-failed-inventory-full", func() domainEventChatInput {
			return domainEventChatTaskFailedSeed(protocol.TaskFailInventoryFull)
		}, nil),
		domainEventChatRow("chat-speech", domainEventChatSpeechSeed, nil),

		domainEventChatRow("chat-event-id-zero", domainEventChatAcceptedSeed, func(in *domainEventChatInput) {
			in.EventID = "0"
		}),
		domainEventChatRow("chat-player-id-zero", domainEventChatAcceptedSeed, func(in *domainEventChatInput) {
			in.PlayerID = domainEventChatZeroHex
		}),
		domainEventChatRow("chat-player-name-untrimmed", domainEventChatAcceptedSeed, func(in *domainEventChatInput) {
			in.PlayerName = domainEventChatUntrimmedPlayerName
		}),
		domainEventChatRow("chat-accepted-reason-not-none", domainEventChatAcceptedSeed, func(in *domainEventChatInput) {
			in.Reason = int(protocol.ChatRejectInvalidFormat)
		}),
		domainEventChatRow("chat-accepted-zero-companion", domainEventChatAcceptedSeed, func(in *domainEventChatInput) {
			in.CompanionID = domainEventChatZeroHex
		}),
		domainEventChatRow("chat-accepted-spaced-companion-name", domainEventChatAcceptedSeed, func(in *domainEventChatInput) {
			in.CompanionName = domainEventChatSpacedName
		}),
		domainEventChatRow("chat-accepted-empty-command", domainEventChatAcceptedSeed, func(in *domainEventChatInput) {
			in.Command = ""
		}),
		domainEventChatRow("chat-accepted-speech-leak", domainEventChatAcceptedSeed, func(in *domainEventChatInput) {
			in.Speech = domainEventChatSeedSpeech
		}),
		domainEventChatRow("chat-invalid-format-leaks-companion-id", domainEventChatInvalidFormatSeed, func(in *domainEventChatInput) {
			in.CompanionID = domainEventChatSeedCompanionHex
		}),
		domainEventChatRow("chat-invalid-format-leaks-companion-name", domainEventChatInvalidFormatSeed, func(in *domainEventChatInput) {
			in.CompanionName = domainEventChatSeedCompanionName
		}),
		domainEventChatRow("chat-invalid-format-leaks-command", domainEventChatInvalidFormatSeed, func(in *domainEventChatInput) {
			in.Command = domainEventChatSeedCommand
		}),
		domainEventChatRow("chat-unknown-companion-invalid-name", domainEventChatUnknownCompanionSeed, func(in *domainEventChatInput) {
			in.CompanionName = domainEventChatSpacedName
		}),
		domainEventChatRow("chat-unknown-companion-leaks-id", domainEventChatUnknownCompanionSeed, func(in *domainEventChatInput) {
			in.CompanionID = domainEventChatSeedCompanionHex
		}),
		domainEventChatRow("chat-queue-full-missing-companion", domainEventChatQueueFullSeed, func(in *domainEventChatInput) {
			in.CompanionID = domainEventChatZeroHex
		}),
		domainEventChatRow("chat-not-following-empty-command", domainEventChatNotFollowingSeed, func(in *domainEventChatInput) {
			in.Command = ""
		}),
		domainEventChatRow("chat-rejected-reserved-reason-three", domainEventChatQueueFullSeed, func(in *domainEventChatInput) {
			in.Reason = 3
		}),
		domainEventChatRow("chat-task-started-reason-not-none", func() domainEventChatInput {
			return domainEventChatTaskSeed(protocol.ChatEventTaskStarted)
		}, func(in *domainEventChatInput) {
			in.Reason = int(protocol.ChatRejectInvalidFormat)
		}),
		domainEventChatRow("chat-task-failed-reason-fifteen", func() domainEventChatInput {
			return domainEventChatTaskFailedSeed(protocol.TaskFailPlannerUnavailable)
		}, func(in *domainEventChatInput) {
			in.Reason = 15
		}),
		domainEventChatRow("chat-task-failed-reason-twenty-one", func() domainEventChatInput {
			return domainEventChatTaskFailedSeed(protocol.TaskFailPlannerUnavailable)
		}, func(in *domainEventChatInput) {
			in.Reason = 21
		}),
		domainEventChatRow("chat-task-event-speech-leak", func() domainEventChatInput {
			return domainEventChatTaskSeed(protocol.ChatEventTaskStarted)
		}, func(in *domainEventChatInput) {
			in.Speech = domainEventChatSeedSpeech
		}),
		domainEventChatRow("chat-speech-has-command", domainEventChatSpeechSeed, func(in *domainEventChatInput) {
			in.Command = domainEventChatSeedCommand
		}),
		domainEventChatRow("chat-speech-empty", domainEventChatSpeechSeed, func(in *domainEventChatInput) {
			in.Speech = ""
		}),
		domainEventChatRow("chat-speech-reason-not-none", domainEventChatSpeechSeed, func(in *domainEventChatInput) {
			in.Reason = int(protocol.ChatRejectInvalidFormat)
		}),
		domainEventChatRow("chat-kind-unknown", domainEventChatAcceptedSeed, func(in *domainEventChatInput) {
			in.Kind = 10
		}),
		domainEventChatRow("chat-accepted-command-1024-bytes", domainEventChatAcceptedSeed, func(in *domainEventChatInput) {
			in.Command = domainEventChatRepeatedText(int(protocol.ChatCommandTextMaxBytes), 'a')
		}),
		domainEventChatRow("chat-accepted-command-1025-bytes", domainEventChatAcceptedSeed, func(in *domainEventChatInput) {
			in.Command = domainEventChatRepeatedText(int(protocol.ChatCommandTextMaxBytes)+1, 'a')
		}),
		domainEventChatRow("chat-speech-256-bytes", domainEventChatSpeechSeed, func(in *domainEventChatInput) {
			in.Speech = domainEventChatRepeatedText(int(protocol.ChatSpeechTextMaxBytes), 'b')
		}),
		domainEventChatRow("chat-speech-257-bytes", domainEventChatSpeechSeed, func(in *domainEventChatInput) {
			in.Speech = domainEventChatRepeatedText(int(protocol.ChatSpeechTextMaxBytes)+1, 'b')
		}),
	}
	return cases
}

// domainEventChatRejectionExpectations is the single source of the exact
// category and rule every boundary rejection publishes, in the table's
// boundary-row order. The two admitted boundary rows at the exact text bounds
// are deliberately absent, and both the outcomes test and the
// illegal-combinations test read this table so the two can never drift.
var domainEventChatRejectionExpectations = []struct {
	label    string
	category string
	rule     string
}{
	{"chat-event-id-zero", "invalid-identity", "chat_event.event_id"},
	{"chat-player-id-zero", "invalid-identity", "chat_event.player_id"},
	{"chat-player-name-untrimmed", "invalid-value", "chat_event.player_name"},
	{"chat-accepted-reason-not-none", "invalid-value", "chat_event.reason"},
	{"chat-accepted-zero-companion", "invalid-value", "chat_event.companion_id"},
	{"chat-accepted-spaced-companion-name", "invalid-value", "chat_event.companion_name"},
	{"chat-accepted-empty-command", "invalid-value", "chat_event.command"},
	{"chat-accepted-speech-leak", "invalid-value", "chat_event.speech"},
	{"chat-invalid-format-leaks-companion-id", "invalid-value", "chat_event.companion_id"},
	{"chat-invalid-format-leaks-companion-name", "invalid-value", "chat_event.companion_name"},
	{"chat-invalid-format-leaks-command", "invalid-value", "chat_event.command"},
	{"chat-unknown-companion-invalid-name", "invalid-value", "chat_event.companion_name"},
	{"chat-unknown-companion-leaks-id", "invalid-value", "chat_event.companion_id"},
	{"chat-queue-full-missing-companion", "invalid-value", "chat_event.companion_id"},
	{"chat-not-following-empty-command", "invalid-value", "chat_event.command"},
	{"chat-rejected-reserved-reason-three", "invalid-enum", "chat_event.rejected.reason"},
	{"chat-task-started-reason-not-none", "invalid-value", "chat_event.reason"},
	{"chat-task-failed-reason-fifteen", "invalid-enum", "chat_event.task_failed.reason"},
	{"chat-task-failed-reason-twenty-one", "invalid-enum", "chat_event.task_failed.reason"},
	{"chat-task-event-speech-leak", "invalid-value", "chat_event.speech"},
	{"chat-speech-has-command", "invalid-value", "chat_event.command"},
	{"chat-speech-empty", "invalid-value", "chat_event.speech"},
	{"chat-speech-reason-not-none", "invalid-value", "chat_event.reason"},
	{"chat-kind-unknown", "invalid-enum", "chat_event.kind"},
	{"chat-accepted-command-1025-bytes", "invalid-value", "chat_event.command"},
	{"chat-speech-257-bytes", "invalid-value", "chat_event.speech"},
}

// runDomainEventChat executes one corpus case through the current Go protocol
// chat DTO.
//
// The verdict always comes from `protocol.ValidateServerPacket` in the play
// state, which is the same validator the codec applies on both the encode and
// the decode side. The rule name and the rejection category come from the
// same bounds the DTO reads, and the two are cross-checked against each
// other, so a classification that disagrees with the authority fails the run
// instead of publishing a plausible-looking rejection. An admitted event
// publishes its exact semantic branch as the accepted category, because the
// branch is the fact the record carries rather than a field of it.
func runDomainEventChat(c CaseSpec, input []byte) (Outcome, []byte, error) {
	spec, err := domainEventChatDecodeInput(input)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: decode input: %w", c.ID, err)
	}
	if spec.Consumer != domainEventChatConsumer {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names consumer %q, want %q", c.ID, spec.Consumer, domainEventChatConsumer)
	}
	if spec.Rule != domainEventChatRuleChat {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown rule %q", c.ID, spec.Rule)
	}

	event, err := domainEventChatEvent(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	category, rule := domainEventChatClassify(event)
	admitted := protocol.ValidateServerPacket(protocol.StatePlay, event) == nil
	if admitted == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with the protocol validator (admitted=%v)", c.ID, rule, admitted)
	}
	if !admitted {
		fields := domainEventChatRejectedFields(spec)
		fields["rule"] = rule
		return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
	}
	return Outcome{Kind: "ok", Category: domainEventChatBranch(event), Fields: domainEventChatAcceptedFields(event)}, nil, nil
}

// domainEventChatEvent resolves one Go chat DTO from its frozen envelope.
// The two UUID fields are decoded raw, because the corpus deliberately
// carries the zero companion UUID and the identity rules belong to the
// validator rather than to the decoding; the kind and reason are bounded to
// the u8 the wire carries so a wider raw value cannot silently truncate into
// a different enum entry.
func domainEventChatEvent(c CaseSpec, spec domainEventChatInput) (protocol.ChatEvent, error) {
	eventID, err := strconv.ParseUint(spec.EventID, 10, 64)
	if err != nil {
		return protocol.ChatEvent{}, fmt.Errorf("runtime-oracle: case %s names event_id %q, which is not a decimal u64", c.ID, spec.EventID)
	}
	playerRaw, err := domainEventChatHexUUID(c, "player_id", spec.PlayerID)
	if err != nil {
		return protocol.ChatEvent{}, err
	}
	companionRaw, err := domainEventChatHexUUID(c, "companion_id", spec.CompanionID)
	if err != nil {
		return protocol.ChatEvent{}, err
	}
	if spec.Kind < 0 || spec.Kind > math.MaxUint8 {
		return protocol.ChatEvent{}, fmt.Errorf("runtime-oracle: case %s names kind %d, which is outside 0..255", c.ID, spec.Kind)
	}
	if spec.Reason < 0 || spec.Reason > math.MaxUint8 {
		return protocol.ChatEvent{}, fmt.Errorf("runtime-oracle: case %s names reason %d, which is outside 0..255", c.ID, spec.Reason)
	}
	return protocol.ChatEvent{
		EventID:       eventID,
		PlayerID:      core.PlayerID(playerRaw),
		PlayerName:    spec.PlayerName,
		CompanionID:   companion.ID(companionRaw),
		CompanionName: spec.CompanionName,
		Kind:          protocol.ChatEventKind(spec.Kind),
		RejectReason:  protocol.ChatRejectReason(spec.Reason),
		Command:       spec.Command,
		Speech:        spec.Speech,
	}, nil
}

// domainEventChatHexUUID decodes one 32-character lowercase hexadecimal UUID
// into its raw 16 bytes. It performs no UUIDv4 and no nonzero check, so the
// zero companion UUID and an unusual identity reach the DTO exactly as the
// corpus names them and the validator decides.
func domainEventChatHexUUID(c CaseSpec, name, text string) ([16]byte, error) {
	var id [16]byte
	if len(text) != 32 {
		return id, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not 32 hexadecimal characters", c.ID, name, text)
	}
	if strings.ToLower(text) != text {
		return id, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not lowercase", c.ID, name, text)
	}
	decoded, err := hex.Decode(id[:], []byte(text))
	if err != nil || decoded != len(id) {
		return id, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not hexadecimal", c.ID, name, text)
	}
	return id, nil
}

// domainEventChatClassify names the first rule one chat event breaks, in the
// exact order the Go `ChatEvent.Validate` checks them: the global identity
// and name failures, then the speech slot's kind exclusivity, then inside the
// kind switch the reason, the companion identity and name, and the command or
// speech text.
//
// A rejection classifies as `invalid-identity` for a zero event or player
// identity, as `invalid-enum` for an unknown kind, the reserved reject reason
// 3 and any other reject reason the rejected branch does not publish, and for
// a failure reason outside the 16..20 domain, and as `invalid-value` for
// every text-boundary failure and every illegal cross-field combination.
// That is the classification rule a Rust consumer of this family applies to
// its own rejection variants, so the two implementations cannot disagree
// about a category.
func domainEventChatClassify(event protocol.ChatEvent) (string, string) {
	if event.EventID == 0 {
		return "invalid-identity", "chat_event.event_id"
	}
	if !event.PlayerID.Valid() {
		return "invalid-identity", "chat_event.player_id"
	}
	if !domainEventChatValidPlayerName(event.PlayerName) {
		return "invalid-value", "chat_event.player_name"
	}
	// The speech slot belongs to CompanionSpeech alone, so any other kind
	// carrying speech fails before the kind-specific switch.
	if event.Kind != protocol.ChatEventCompanionSpeech && event.Speech != "" {
		return "invalid-value", "chat_event.speech"
	}
	switch event.Kind {
	case protocol.ChatEventAccepted:
		if event.RejectReason != protocol.ChatRejectNone {
			return "invalid-value", "chat_event.reason"
		}
		if !event.CompanionID.Valid() {
			return "invalid-value", "chat_event.companion_id"
		}
		if companion.ValidateName(event.CompanionName) != nil {
			return "invalid-value", "chat_event.companion_name"
		}
		if !domainEventChatValidCommandText(event.Command) {
			return "invalid-value", "chat_event.command"
		}
	case protocol.ChatEventRejected:
		switch event.RejectReason {
		case protocol.ChatRejectInvalidFormat:
			if event.CompanionID != (companion.ID{}) {
				return "invalid-value", "chat_event.companion_id"
			}
			if event.Command != "" {
				return "invalid-value", "chat_event.command"
			}
			if event.CompanionName != "" {
				return "invalid-value", "chat_event.companion_name"
			}
		case protocol.ChatRejectUnknownCompanion:
			if event.CompanionID != (companion.ID{}) {
				return "invalid-value", "chat_event.companion_id"
			}
			if event.Command != "" {
				return "invalid-value", "chat_event.command"
			}
			if companion.ValidateName(event.CompanionName) != nil {
				return "invalid-value", "chat_event.companion_name"
			}
		case protocol.ChatRejectQueueFull, protocol.ChatRejectNotFollowing:
			if !event.CompanionID.Valid() {
				return "invalid-value", "chat_event.companion_id"
			}
			if companion.ValidateName(event.CompanionName) != nil {
				return "invalid-value", "chat_event.companion_name"
			}
			if !domainEventChatValidCommandText(event.Command) {
				return "invalid-value", "chat_event.command"
			}
		default:
			return "invalid-enum", "chat_event.rejected.reason"
		}
	case protocol.ChatEventTaskStarted, protocol.ChatEventTaskProgress,
		protocol.ChatEventTaskCompleted, protocol.ChatEventTaskTimedOut, protocol.ChatEventTaskStopped:
		if event.RejectReason != protocol.ChatRejectNone {
			return "invalid-value", "chat_event.reason"
		}
		if !event.CompanionID.Valid() {
			return "invalid-value", "chat_event.companion_id"
		}
		if companion.ValidateName(event.CompanionName) != nil {
			return "invalid-value", "chat_event.companion_name"
		}
		if !domainEventChatValidCommandText(event.Command) {
			return "invalid-value", "chat_event.command"
		}
	case protocol.ChatEventTaskFailed:
		if !domainEventChatValidTaskFailReason(protocol.TaskFailReason(event.RejectReason)) {
			return "invalid-enum", "chat_event.task_failed.reason"
		}
		if !event.CompanionID.Valid() {
			return "invalid-value", "chat_event.companion_id"
		}
		if companion.ValidateName(event.CompanionName) != nil {
			return "invalid-value", "chat_event.companion_name"
		}
		if !domainEventChatValidCommandText(event.Command) {
			return "invalid-value", "chat_event.command"
		}
	case protocol.ChatEventCompanionSpeech:
		if event.RejectReason != protocol.ChatRejectNone {
			return "invalid-value", "chat_event.reason"
		}
		if event.Command != "" {
			return "invalid-value", "chat_event.command"
		}
		if !event.CompanionID.Valid() {
			return "invalid-value", "chat_event.companion_id"
		}
		if companion.ValidateName(event.CompanionName) != nil {
			return "invalid-value", "chat_event.companion_name"
		}
		if !domainEventChatValidSpeechText(event.Speech) {
			return "invalid-value", "chat_event.speech"
		}
	default:
		return "invalid-enum", "chat_event.kind"
	}
	return "", ""
}

// domainEventChatBranch names the exact semantic branch one admitted event
// carries, which is the accepted outcome's category. The function is only
// called on an event the authority admitted, so every default arm names the
// single legal remainder of its switch rather than an unknown value.
func domainEventChatBranch(event protocol.ChatEvent) string {
	switch event.Kind {
	case protocol.ChatEventAccepted:
		return "accepted"
	case protocol.ChatEventRejected:
		switch event.RejectReason {
		case protocol.ChatRejectInvalidFormat:
			return "invalid-format"
		case protocol.ChatRejectUnknownCompanion:
			return "unknown-companion"
		case protocol.ChatRejectQueueFull:
			return "queue-full"
		default:
			return "not-following"
		}
	case protocol.ChatEventTaskStarted:
		return "task-started"
	case protocol.ChatEventTaskProgress:
		return "task-progress"
	case protocol.ChatEventTaskCompleted:
		return "task-completed"
	case protocol.ChatEventTaskTimedOut:
		return "task-timed-out"
	case protocol.ChatEventTaskStopped:
		return "task-stopped"
	case protocol.ChatEventTaskFailed:
		switch protocol.TaskFailReason(event.RejectReason) {
		case protocol.TaskFailPlannerUnavailable:
			return "task-failed-planner-unavailable"
		case protocol.TaskFailInvalidPlan:
			return "task-failed-invalid-plan"
		case protocol.TaskFailPathUnreachable:
			return "task-failed-path-unreachable"
		case protocol.TaskFailWorldChanged:
			return "task-failed-world-changed"
		default:
			return "task-failed-inventory-full"
		}
	default:
		return "speech"
	}
}

// domainEventChatAcceptedFields renders one admitted event in the normalized
// field map: the event and player identity every branch carries, plus exactly
// the companion, name, command or speech data the branch's rules keep legal.
// The empty wire sentinels a branch forbids are not retained, so an
// invalid-format rejection publishes no companion fields at all and an
// unknown-companion rejection publishes only the target name.
func domainEventChatAcceptedFields(event protocol.ChatEvent) map[string]any {
	fields := map[string]any{
		"event_id":    strconv.FormatUint(event.EventID, 10),
		"player_id":   domainEventChatHexText(event.PlayerID[:]),
		"player_name": event.PlayerName,
	}
	switch event.Kind {
	case protocol.ChatEventAccepted,
		protocol.ChatEventTaskStarted, protocol.ChatEventTaskProgress,
		protocol.ChatEventTaskCompleted, protocol.ChatEventTaskTimedOut,
		protocol.ChatEventTaskStopped, protocol.ChatEventTaskFailed:
		fields["companion_id"] = domainEventChatHexText(event.CompanionID[:])
		fields["companion_name"] = event.CompanionName
		fields["command"] = event.Command
	case protocol.ChatEventRejected:
		switch event.RejectReason {
		case protocol.ChatRejectInvalidFormat:
			// The zero identity and the empty texts are wire sentinels the
			// branch forbids, so none of them is retained.
		case protocol.ChatRejectUnknownCompanion:
			fields["companion_name"] = event.CompanionName
		default:
			fields["companion_id"] = domainEventChatHexText(event.CompanionID[:])
			fields["companion_name"] = event.CompanionName
			fields["command"] = event.Command
		}
	default:
		fields["companion_id"] = domainEventChatHexText(event.CompanionID[:])
		fields["companion_name"] = event.CompanionName
		fields["speech"] = event.Speech
	}
	return fields
}

// domainEventChatRejectedFields renders one rejected event's raw semantic
// inputs exactly as the case named them, including an unknown numeric kind
// or reason, so a replay consumer sees the value the record carried rather
// than a normalized one.
func domainEventChatRejectedFields(spec domainEventChatInput) map[string]any {
	return map[string]any{
		"event_id":       spec.EventID,
		"player_id":      spec.PlayerID,
		"companion_id":   spec.CompanionID,
		"player_name":    spec.PlayerName,
		"companion_name": spec.CompanionName,
		"kind":           spec.Kind,
		"reason":         spec.Reason,
		"command":        spec.Command,
		"speech":         spec.Speech,
	}
}

// domainEventChatHexText renders one raw 16-byte identity as the 32
// lowercase hexadecimal characters the corpus vocabulary uses.
func domainEventChatHexText(id []byte) string {
	return hex.EncodeToString(id)
}

// domainEventChatValidPlayerName mirrors the protocol package's private
// `validPlayerName`: the display-name admission rule accepts the name and the
// canonical form equals the input, so a trimmable name is not a legal player
// name. The copy is pinned by the same untrimmed boundary the domain test
// pins.
func domainEventChatValidPlayerName(name string) bool {
	canonical, err := core.NormalizeDisplayName(name)
	return err == nil && canonical == name
}

// domainEventChatValidTaskFailReason mirrors the protocol package's private
// `validTaskFailReason`: the fixed 16..20 failure enum the TaskFailed branch
// requires, with the whole reject-reason interval and every other value
// outside the domain.
func domainEventChatValidTaskFailReason(reason protocol.TaskFailReason) bool {
	switch reason {
	case protocol.TaskFailPlannerUnavailable, protocol.TaskFailInvalidPlan,
		protocol.TaskFailPathUnreachable, protocol.TaskFailWorldChanged,
		protocol.TaskFailInventoryFull:
		return true
	default:
		return false
	}
}

// domainEventChatValidCommandText mirrors the protocol package's private
// `validateCommandText` with the bound taken from the exported
// `protocol.ChatCommandTextMaxBytes`, so the copied rule cannot drift from
// the authority's bound: 1..max bytes, valid UTF-8, no NUL or Unicode
// control character, and no leading or trailing whitespace.
func domainEventChatValidCommandText(text string) bool {
	return domainEventChatValidBoundedText(text, int(protocol.ChatCommandTextMaxBytes))
}

// domainEventChatValidSpeechText mirrors the protocol package's private
// `validateSpeechText` with the bound taken from the exported
// `protocol.ChatSpeechTextMaxBytes`: the same text discipline as the command
// slot with the tighter speech bound.
func domainEventChatValidSpeechText(text string) bool {
	return domainEventChatValidBoundedText(text, int(protocol.ChatSpeechTextMaxBytes))
}

// domainEventChatValidBoundedText is the shared text discipline the two text
// slots keep: a nonempty byte range inside the named bound, valid UTF-8, no
// NUL or Unicode control character, and no leading or trailing whitespace.
func domainEventChatValidBoundedText(text string, maxBytes int) bool {
	if len(text) < 1 || len(text) > maxBytes || !utf8.ValidString(text) || strings.TrimSpace(text) != text {
		return false
	}
	for _, r := range text {
		if r == 0 || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// domainEventChatDecodeInput reads one frozen corpus input and rejects
// trailing content, so a producer never executes bytes the case did not name.
func domainEventChatDecodeInput(data []byte) (domainEventChatInput, error) {
	var spec domainEventChatInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&spec); err != nil {
		return domainEventChatInput{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainEventChatInput{}, fmt.Errorf("trailing content after the JSON value")
	}
	return spec, nil
}

// domainEventChatRecord pairs one executed case with the input and outcome
// the producer produced for it.
type domainEventChatRecord struct {
	label   string
	input   domainEventChatInput
	outcome Outcome
}

// domainEventChatExecute runs the whole case table through the producer and
// returns one record per case in table order.
func domainEventChatExecute(t *testing.T) []domainEventChatRecord {
	t.Helper()

	cases := domainEventChatCases()
	records := make([]domainEventChatRecord, 0, len(cases))
	for _, entry := range cases {
		input, err := json.MarshalIndent(entry.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", entry.label, err)
		}
		input = append(input, '\n')
		outcome, _, err := runDomainEventChat(CaseSpec{ID: domainEventChatCaseID(entry.label)}, input)
		if err != nil {
			t.Fatalf("execute case %s: %v", entry.label, err)
		}
		records = append(records, domainEventChatRecord{label: entry.label, input: entry.input, outcome: outcome})
	}
	return records
}

// `domainEventChatSyncCorpus` compares committed assets and optionally
// exports a complete producer candidate for controller review.
//
// Ordinary runs compare committed assets read-only against current producer
// output. The explicit export publishes the complete candidate to a fresh
// external directory before the committed bytes are compared, so an initial
// export run creates every candidate file even while the tracked directory is
// still absent, and never mutates tracked assets.
func domainEventChatSyncCorpus(t *testing.T, records []domainEventChatRecord) {
	t.Helper()

	root := mustRepoRoot(t)
	corpusDir := filepath.Join(root, filepath.FromSlash(domainEventChatCorpusRelDir))
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

	var relatives []string
	for relative := range want {
		relatives = append(relatives, relative)
	}
	sort.Strings(relatives)
	assets := make([]generatedAsset, 0, len(relatives))
	for _, relative := range relatives {
		assets = append(assets, generatedAsset{
			RelativePath: relative,
			Data:         want[relative],
		})
	}
	domainEventChatExport(t, root, assets)

	for relative, data := range want {
		target := filepath.Join(corpusDir, filepath.FromSlash(relative))
		committed, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read frozen corpus case %s: %v", relative, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("frozen corpus case %s drifted from the executed protocol DTO", relative)
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

// domainEventChatExport publishes the complete candidate once per test
// process when the export directory is named. The topic entry delegates to
// the execute-every-case test, so one filter selecting both entries executes
// the same deterministic candidate twice, and the exporter's exclusive
// producer child must not treat the second publication as a conflict.
func domainEventChatExport(t *testing.T, root string, assets []generatedAsset) {
	t.Helper()
	if strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv)) == "" {
		return
	}
	if domainEventChatExportPublished {
		return
	}
	domainEventChatExportPublished = true
	exportGeneratedAssetsFromEnvironment(t, root, domainEventChatProducerID, assets)
}

// domainEventChatCaseID renders the manifest case identity one corpus label
// carries. The version segment is the family's published version, because a
// case has to name the version of the family it belongs to.
func domainEventChatCaseID(label string) string {
	return domainEventChatFamily + "/" + domainEventChatVersion + "/" + label
}

// `domainEventChatWorkingManifest` clones the merged committed manifest into
// a producer-scoped selection stored in harness-owned temporary storage.
// `Cases` is narrowed to the chat cases and unrelated family case lists are
// cleared. The existing `domain.event` identity is retained while its
// provenance and case list are replaced with this producer's current
// selection, because the canonical manifest does not yet register these
// cases; that registration is a later node's work.
func domainEventChatWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	cases := domainEventChatCorpusCases(t, root)

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
	sources := make([]SourceSpec, 0, len(domainEventChatFamilySources))
	for _, relative := range domainEventChatFamilySources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", relative, err)
		}
		sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
	}
	registered := false
	for index := range cloned.Families {
		if cloned.Families[index].ID != domainEventChatFamily {
			cloned.Families[index].Cases = nil
			continue
		}
		cloned.Families[index].Sources = sources
		cloned.Families[index].Cases = caseIDs
		registered = true
	}
	if !registered {
		t.Fatalf("working manifest has no %s family", domainEventChatFamily)
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

// domainEventChatCorpusCases reads the frozen corpus and registers one case
// per committed input, with digests proven against the files on disk.
func domainEventChatCorpusCases(t *testing.T, root string) []CaseSpec {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(domainEventChatCorpusRelDir))
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
		t.Fatalf("walk event chat corpus directory: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no event chat case under %s", domainEventChatCorpusRelDir)
	}
	sort.Strings(inputs)

	cases := make([]CaseSpec, 0, len(inputs))
	for _, input := range inputs {
		relative, relErr := filepath.Rel(dir, input)
		if relErr != nil {
			t.Fatalf("relative corpus path: %v", relErr)
		}
		label := strings.TrimSuffix(filepath.ToSlash(relative), ".input.json")
		if label == "" || !domainEventChatLabelPattern.MatchString(label) {
			t.Fatalf("corpus case %s has label %q, which is not a lowercase slug with an optional numeric suffix", filepath.ToSlash(relative), label)
		}
		envelope := domainEventChatReadEnvelope(t, input)
		if envelope.Consumer != domainEventChatConsumer {
			t.Fatalf("corpus case %s names consumer %q, want %q", label, envelope.Consumer, domainEventChatConsumer)
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
			ID:           domainEventChatCaseID(label),
			Family:       domainEventChatFamily,
			Version:      domainEventChatVersion,
			Operation:    domainEventChatOperation,
			Input:        AssetRef{Path: domainEventChatCorpusPath(root, input), SHA256: inputHash},
			InputFormat:  "json",
			Expected:     AssetRef{Path: domainEventChatCorpusPath(root, expectedPath), SHA256: expectedHash},
			Checkpoints:  []string{"0"},
			RustConsumer: domainEventChatConsumer,
		})
	}
	return cases
}

// domainEventChatReadEnvelope reads the provenance envelope of one frozen
// corpus input.
func domainEventChatReadEnvelope(t *testing.T, path string) domainEventChatInput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var envelope domainEventChatInput
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return envelope
}

// domainEventChatCorpusPath renders one absolute corpus path as the
// repository-relative slash path the manifest requires.
func domainEventChatCorpusPath(root, absolute string) string {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return absolute
	}
	return filepath.ToSlash(relative)
}

// TestDomainEventChatOracleExecutesEveryCase runs the whole case table
// through the real Go protocol DTO, proves the frozen corpus still matches
// what it produces, and then runs the same cases through the production
// runner so the executed evidence satisfies the completeness rules a
// published trace report does.
func TestDomainEventChatOracleExecutesEveryCase(t *testing.T) {
	records := domainEventChatExecute(t)
	domainEventChatSyncCorpus(t, records)

	root := mustRepoRoot(t)
	manifest := domainEventChatWorkingManifest(t, root)
	if len(manifest.Cases) != len(records) {
		t.Fatalf("working manifest registers %d cases, want %d (one per executed table row)", len(manifest.Cases), len(records))
	}
	domainEventChatAssertCaseSpecs(t, root, manifest)
	domainEventChatAssertOutcomesDistinguishCases(t, records)

	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(observations) != len(manifest.Cases) {
		t.Fatalf("produced %d observations, want %d", len(observations), len(manifest.Cases))
	}
	for _, obs := range observations {
		c := domainEventChatCaseByID(t, manifest, obs.CaseID)
		expected := readExpectedOutcome(t, root, c)
		if !outcomesEqual(obs.Outcome, expected) {
			t.Fatalf("case %s produced %#v, want %#v", obs.CaseID, obs.Outcome, expected)
		}
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	for _, obs := range trace.Observations {
		c := domainEventChatCaseByID(t, manifest, obs.CaseID)
		if obs.ExpectedDigest != c.Expected.SHA256 {
			t.Fatalf("observation for %s carries expected digest %s, want %s", obs.CaseID, obs.ExpectedDigest, c.Expected.SHA256)
		}
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed event chat evidence failed trace validation: %v", err)
	}
}

// TestDomainEventChatOracleCaseIdentitiesAreTheRustDomainConsumer pins the
// manifest identity of every event chat case: the operation, the family
// version, the family, the consumer and the sole rule the change names for
// the Rust crate that owns these records.
func TestDomainEventChatOracleCaseIdentitiesAreTheRustDomainConsumer(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventChatWorkingManifest(t, root)

	labels := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Operation != domainEventChatOperation {
			t.Fatalf("case %s declares operation %q, want %q", c.ID, c.Operation, domainEventChatOperation)
		}
		if c.Version != domainEventChatVersion {
			t.Fatalf("case %s declares version %q, want %q", c.ID, c.Version, domainEventChatVersion)
		}
		if c.Family != domainEventChatFamily {
			t.Fatalf("case %s declares family %q, want %q", c.ID, c.Family, domainEventChatFamily)
		}
		if c.RustConsumer != domainEventChatConsumer {
			t.Fatalf("case %s declares consumer %q, want %q", c.ID, c.RustConsumer, domainEventChatConsumer)
		}
		if c.InputFormat != "json" {
			t.Fatalf("case %s declares input_format %q, want json", c.ID, c.InputFormat)
		}
		prefix := c.Family + "/" + c.Version + "/"
		if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
			t.Fatalf("case %s does not match %s<label>", c.ID, prefix)
		}
		label := strings.TrimPrefix(c.ID, prefix)
		if !domainEventChatLabelPattern.MatchString(label) {
			t.Fatalf("case %s label %q is not a lowercase slug with an optional numeric suffix", c.ID, label)
		}
		if labels[label] {
			t.Fatalf("label %q is registered twice", label)
		}
		labels[label] = true

		envelope := domainEventChatReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		if envelope.Consumer != domainEventChatConsumer {
			t.Fatalf("case %s was produced for consumer %q, want %q", c.ID, envelope.Consumer, domainEventChatConsumer)
		}
		if envelope.Rule != domainEventChatRuleChat {
			t.Fatalf("case %s names rule %q, which is not the chat rule", c.ID, envelope.Rule)
		}
	}
}

// TestDomainEventChatOracleWorkingManifestDescribesItself proves the working
// manifest is internally consistent: every case passes the production case
// validation, the family's provenance hashes match disk, and the family's case
// list matches exactly the cases the selection registers.
func TestDomainEventChatOracleWorkingManifestDescribesItself(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventChatWorkingManifest(t, root)
	domainEventChatAssertCaseSpecs(t, root, manifest)

	family, ok := domainEventChatFamilySpec(manifest)
	if !ok {
		t.Fatalf("working manifest has no %s family", domainEventChatFamily)
	}
	if family.Role != "event" || family.Kind != "domain" {
		t.Fatalf("%s declares kind %q role %q, want domain event", domainEventChatFamily, family.Kind, family.Role)
	}
	if family.CurrentVersion != domainEventChatVersion {
		t.Fatalf("%s declares version %q, want %q", domainEventChatFamily, family.CurrentVersion, domainEventChatVersion)
	}
	if len(family.Sources) != len(domainEventChatFamilySources) {
		t.Fatalf("%s records %d provenance sources, want %d", domainEventChatFamily, len(family.Sources), len(domainEventChatFamilySources))
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
		t.Fatalf("%s lists %d cases, want %d", domainEventChatFamily, len(family.Cases), len(registered))
	}
	for _, id := range family.Cases {
		if !registered[id] {
			t.Fatalf("%s lists case %s which the selection does not register", domainEventChatFamily, id)
		}
	}
}

// TestDomainEventChatOracleOutcomesDistinguishAcceptedAndRejected pins the
// exact shape of the executed evidence: the 44 unique labels, both admitted
// records and rejections, all three rejection categories of the frozen
// vocabulary, every rule name the classifier publishes, and the raw boundary
// values a rejection retains.
func TestDomainEventChatOracleOutcomesDistinguishAcceptedAndRejected(t *testing.T) {
	records := domainEventChatExecute(t)
	domainEventChatAssertOutcomesDistinguishCases(t, records)

	labels := make(map[string]bool, len(records))
	for _, record := range records {
		if labels[record.label] {
			t.Fatalf("label %q executes twice", record.label)
		}
		labels[record.label] = true
	}
	if len(labels) != domainEventChatCaseTotal {
		t.Fatalf("executed evidence carries %d unique labels, want %d", len(labels), domainEventChatCaseTotal)
	}

	tally := &domainEventChatRuleTally{}
	for _, record := range records {
		switch record.outcome.Kind {
		case "ok":
			tally.accepted = append(tally.accepted, record.outcome.Fields)
		case "error":
			tally.rejected = append(tally.rejected, record.outcome.Fields)
		}
	}
	// The sixteen branch rows plus the two exact-bound rows are admitted;
	// the other twenty-six boundary rows are rejections.
	if len(tally.accepted) != 18 || len(tally.rejected) != 26 {
		t.Fatalf("executed evidence records %d accepted and %d rejected cases, want 18 and 26", len(tally.accepted), len(tally.rejected))
	}

	categories := make(map[string]bool)
	for _, fields := range tally.rejected {
		categories[domainEventChatRejectionCategoryOf(t, fields)] = true
	}
	for _, category := range []string{"invalid-identity", "invalid-enum", "invalid-value"} {
		if !categories[category] {
			t.Fatalf("no rejection publishes category %q", category)
		}
	}

	// Every rule name the classifier publishes is pinned by at least one
	// executed rejection, so a renamed or dropped boundary fails here rather
	// than surviving as an unpinned rule.
	for _, expectation := range domainEventChatRejectionExpectations {
		if !domainEventChatRejectionNamesRule(tally, expectation.rule) {
			t.Fatalf("no rejection names the %s rule, so the boundary is not pinned", expectation.rule)
		}
	}

	// A rejection retains the raw input value, including the unknown numeric
	// enum and the exact reason values the reserved and failure boundaries
	// name.
	if !domainEventChatFieldsContain(tally.rejected, "kind", 10) {
		t.Fatal("no rejection retains the unknown kind value 10")
	}
	if !domainEventChatFieldsContain(tally.rejected, "reason", 3) {
		t.Fatal("no rejection retains the reserved reason value 3")
	}
	if !domainEventChatFieldsContain(tally.rejected, "reason", 15) || !domainEventChatFieldsContain(tally.rejected, "reason", 21) {
		t.Fatal("no rejection retains the failure reason values 15 and 21")
	}
	if !domainEventChatFieldsContain(tally.rejected, "player_name", domainEventChatUntrimmedPlayerName) {
		t.Fatal("no rejection retains the untrimmed player name")
	}
}

// TestDomainEventChatOracleRunnerRejectsUnregisteredFamily pins that a case
// cannot pass by being ignored.
func TestDomainEventChatOracleRunnerRejectsUnregisteredFamily(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventChatWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Family = "domain.unknown"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "family domain.unknown has no registered Go producer") {
		t.Fatalf("expected an unregistered-family failure, got: %v", err)
	}
}

// TestDomainEventChatOracleRunnerRejectsOperationFamilyMismatch pins that a
// case cannot declare one operation and be executed by another.
func TestDomainEventChatOracleRunnerRejectsOperationFamilyMismatch(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventChatWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Operation = "decode"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "declares operation") {
		t.Fatalf("expected an operation-family mismatch failure, got: %v", err)
	}
}

// TestDomainEventChatOracleRunnerRejectsTamperedInput pins that a case whose
// committed bytes no longer match their recorded digest fails the run instead
// of executing bytes the manifest does not name.
func TestDomainEventChatOracleRunnerRejectsTamperedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventChatWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// TestDomainEventChatOracleReportPublishesAndValidates assembles the executed
// evidence into a report, publishes it through the production atomic
// exporter, reloads it and validates it against the working manifest. This is
// the identity and content check a later Rust acceptance step performs; it
// does not claim any Rust behaviour.
func TestDomainEventChatOracleReportPublishesAndValidates(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventChatWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed event chat evidence failed trace validation: %v", err)
	}

	workspace, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	defer cleanup()
	target := filepath.Join(workspace, domainEventChatCorpusReportName)
	if err := ExportTrace(root, target, trace, manifest); err != nil {
		t.Fatalf("ExportTrace: %v", err)
	}
	loaded, err := LoadTraceAtRoot(root, target, manifest)
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

// TestDomainEventChatOracleRejectsMissingProducerTest pins that the executed
// evidence has a real producer behind it, including the topic-named entry
// point the domain plan's filter selects. A corpus whose producer test is gone
// has no independent execution, only frozen files, and a missing topic entry
// point would make the plan filter pass without running anything.
func TestDomainEventChatOracleRejectsMissingProducerTest(t *testing.T) {
	root := mustRepoRoot(t)
	path := filepath.Join(root, filepath.FromSlash(domainEventChatProducerTestRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read producer test %s: %v", domainEventChatProducerTestRelPath, err)
	}
	for _, name := range []string{domainEventChatProducerExecuteName, domainEventChatProducerTopicName} {
		declaration := "func " + name + "(t *testing.T) {"
		if !strings.Contains(string(data), declaration) {
			t.Fatalf("%s does not declare %s", domainEventChatProducerTestRelPath, declaration)
		}
	}
}

// TestDomainEventChatExportRejectsRepositoryAndSymlinkTargets pins the
// publication boundaries of this producer's export identity: an export root
// inside the repository is refused before anything is created, and an export
// root reached through a symlinked ancestor is refused without writing through
// the link.
func TestDomainEventChatExportRejectsRepositoryAndSymlinkTargets(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	assets := []generatedAsset{{RelativePath: "chat-accepted.input.json", Data: []byte("{}\n")}}

	contained := filepath.Join(root, "chat-export-probe")
	_, err := exportGeneratedAssets(root, contained, domainEventChatProducerID, assets)
	if err == nil || !strings.Contains(err.Error(), "live-path") {
		t.Fatalf("expected a live-path rejection for a repository-contained root, got: %v", err)
	}
	if _, statErr := os.Lstat(contained); !os.IsNotExist(statErr) {
		t.Fatalf("export directory was created inside repository: %v", statErr)
	}

	parent := t.TempDir()
	realDir := filepath.Join(parent, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkDir := filepath.Join(parent, "symlink-ancestor")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Fatal(err)
	}
	_, err = exportGeneratedAssets(root, filepath.Join(symlinkDir, "export"), domainEventChatProducerID, assets)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected a symlink rejection for a symlinked ancestor, got: %v", err)
	}
	entries, readErr := os.ReadDir(realDir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("export wrote through the symlinked ancestor: %v", entries)
	}
}

// TestDomainEventChatEveryLegalBranchIsExecuted pins that the executed
// evidence covers every legal semantic branch exactly: the sixteen branch
// categories the union publishes, with the accepted and speech branches
// carrying their exact-bound rows beside their branch rows, and the admitted
// command and speech texts proving the 1,024 and 256 byte boundaries stay
// inside the domain.
func TestDomainEventChatEveryLegalBranchIsExecuted(t *testing.T) {
	records := domainEventChatExecute(t)

	branches := make(map[string]int)
	for _, record := range records {
		if record.outcome.Kind != "ok" {
			continue
		}
		branches[record.outcome.Category]++
	}
	want := make(map[string]int, 16)
	for _, branch := range []string{
		"accepted",
		"invalid-format",
		"unknown-companion",
		"queue-full",
		"not-following",
		"task-started",
		"task-progress",
		"task-completed",
		"task-timed-out",
		"task-stopped",
		"task-failed-planner-unavailable",
		"task-failed-invalid-plan",
		"task-failed-path-unreachable",
		"task-failed-world-changed",
		"task-failed-inventory-full",
		"speech",
	} {
		want[branch] = 1
	}
	// The two exact-bound rows are admitted members of the accepted and
	// speech branches.
	want["accepted"] = 2
	want["speech"] = 2
	if !reflect.DeepEqual(branches, want) {
		t.Fatalf("executed branch coverage = %v, want %v", branches, want)
	}

	for _, record := range records {
		if record.outcome.Kind != "ok" {
			continue
		}
		switch record.label {
		case "chat-accepted-command-1024-bytes":
			command, _ := record.outcome.Fields["command"].(string)
			if len(command) != int(protocol.ChatCommandTextMaxBytes) {
				t.Fatalf("exact-bound command carries %d bytes, want %d", len(command), protocol.ChatCommandTextMaxBytes)
			}
		case "chat-speech-256-bytes":
			speech, _ := record.outcome.Fields["speech"].(string)
			if len(speech) != int(protocol.ChatSpeechTextMaxBytes) {
				t.Fatalf("exact-bound speech carries %d bytes, want %d", len(speech), protocol.ChatSpeechTextMaxBytes)
			}
		}
	}
}

// TestDomainEventChatIllegalFieldCombinationsAreRejected pins the exact
// category and rule every boundary rejection publishes, so a classifier that
// renames a rule, reorders a branch check into a different first failure, or
// drifts a category outside the ruling fails here rather than in a later
// Rust acceptance step.
func TestDomainEventChatIllegalFieldCombinationsAreRejected(t *testing.T) {
	records := domainEventChatExecute(t)
	byLabel := make(map[string]domainEventChatRecord, len(records))
	for _, record := range records {
		byLabel[record.label] = record
	}

	seen := make(map[string]bool, len(domainEventChatRejectionExpectations))
	for _, expectation := range domainEventChatRejectionExpectations {
		record, ok := byLabel[expectation.label]
		if !ok {
			t.Fatalf("boundary case %s is missing from the executed table", expectation.label)
		}
		if record.outcome.Kind != "error" {
			t.Fatalf("boundary case %s publishes kind %q, want error", expectation.label, record.outcome.Kind)
		}
		if record.outcome.Category != expectation.category {
			t.Fatalf("boundary case %s publishes category %q, want %q", expectation.label, record.outcome.Category, expectation.category)
		}
		if record.outcome.Fields["rule"] != expectation.rule {
			t.Fatalf("boundary case %s publishes rule %v, want %q", expectation.label, record.outcome.Fields["rule"], expectation.rule)
		}
		seen[expectation.label] = true
	}
	// Every rejection the executed table records is named by exactly one
	// expectation row, so the table and the expectations stay the same set.
	for _, record := range records {
		if record.outcome.Kind != "error" {
			continue
		}
		if !seen[record.label] {
			t.Fatalf("executed rejection %s is not named by any expectation row", record.label)
		}
	}
}

// domainEventChatSelection is this producer's exact reviewed registration:
// the 44 committed case specifications with digests proven against the files
// on disk, plus the provenance paths those rules are read from. The shared
// manifest-candidate helper consumes it, so the candidate this node publishes
// is assembled from the same reviewed `CaseSpec` values the committed corpus
// freezes rather than from a second rendering.
func domainEventChatSelection(t *testing.T, root string) domainEventSelection {
	t.Helper()
	return domainEventSelection{
		Cases:   domainEventChatCorpusCases(t, root),
		Sources: append([]string(nil), domainEventChatFamilySources...),
	}
}

// The frozen manifest counts this node's candidate must reach: 533 total
// `mornlea_domain` cases, 321 of them registered under the shared
// `domain.event` family, and a family provenance union of 30 paths.
const (
	domainEventChatMergedDomainTotal = 533
	domainEventChatMergedFamilyTotal = 321
	domainEventChatMergedSourceTotal = 30
)

// TestDomainEventChatManifestCandidateClosesTheEventPartition merges this
// producer's selection into the tracked manifest, refreshes the execution
// baseline revision once to the captured checkout SHA, proves the merged
// candidate closes the exact 533-case domain partition, and publishes the
// candidate for controller review. A base that already registers the chat
// corpus accepts an idempotent re-merge of the same reviewed specs, so the
// same test gate covers both the first publication and every later ordinary
// run.
func TestDomainEventChatManifestCandidateClosesTheEventPartition(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	selection := domainEventChatSelection(t, root)
	merged := mergeDomainEventSelections(t, root, base, selection)
	// This node refreshes the execution baseline once: the captured checkout
	// SHA is the single source revision the manifest's `source_revision` and
	// the Go `BaselineSourceRevision` constant must both carry from here on.
	merged.SourceRevision = BaselineSourceRevision

	baseIDs := make(map[string]bool, len(base.Cases))
	for _, c := range base.Cases {
		baseIDs[c.ID] = true
	}
	mergedByID := make(map[string]CaseSpec, len(merged.Cases))
	for _, c := range merged.Cases {
		mergedByID[c.ID] = c
	}
	added := 0
	for _, want := range selection.Cases {
		got, ok := mergedByID[want.ID]
		if !ok {
			t.Fatalf("merged manifest drops chat case %s", want.ID)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("merged manifest rewrites chat case %s", want.ID)
		}
		if !baseIDs[want.ID] {
			added++
		}
	}
	if added != 0 && added != domainEventChatCaseTotal {
		t.Fatalf("merged manifest adds %d new chat cases, want 0 (already registered) or %d", added, domainEventChatCaseTotal)
	}
	if len(merged.Cases) != len(base.Cases)+added {
		t.Fatalf("merged manifest carries %d cases, want %d", len(merged.Cases), len(base.Cases)+added)
	}

	domainTotal, familyTopLevel := 0, 0
	for _, c := range merged.Cases {
		if c.RustConsumer == domainEventChatConsumer {
			domainTotal++
		}
		if c.Family == domainEventChatFamily {
			familyTopLevel++
		}
	}
	if domainTotal != domainEventChatMergedDomainTotal {
		t.Fatalf("merged manifest registers %d mornlea_domain cases, want %d", domainTotal, domainEventChatMergedDomainTotal)
	}
	if familyTopLevel != domainEventChatMergedFamilyTotal {
		t.Fatalf("merged manifest registers %d domain.event cases, want %d", familyTopLevel, domainEventChatMergedFamilyTotal)
	}
	family, ok := domainEventFamilySpec(merged)
	if !ok {
		t.Fatalf("merged manifest has no %s family", domainEventChatFamily)
	}
	if len(family.Cases) != domainEventChatMergedFamilyTotal {
		t.Fatalf("%s lists %d cases, want %d", domainEventChatFamily, len(family.Cases), domainEventChatMergedFamilyTotal)
	}
	if len(family.Sources) != domainEventChatMergedSourceTotal {
		t.Fatalf("%s records %d provenance sources, want %d", domainEventChatFamily, len(family.Sources), domainEventChatMergedSourceTotal)
	}
	// The provenance union is the exact sorted 30-path set: no duplicate
	// path, every recorded digest matching disk, and the five additions the
	// three producer stages contributed all present.
	paths := make(map[string]bool, len(family.Sources))
	for index, source := range family.Sources {
		if index > 0 && source.Path <= family.Sources[index-1].Path {
			t.Fatalf("%s provenance is not strictly sorted at %s", domainEventChatFamily, source.Path)
		}
		if paths[source.Path] {
			t.Fatalf("%s provenance lists the path %s twice", domainEventChatFamily, source.Path)
		}
		paths[source.Path] = true
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(source.Path)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", source.Path, err)
		}
		if hash != source.SHA256 {
			t.Fatalf("provenance source %s records %s, disk has %s", source.Path, source.SHA256, hash)
		}
	}
	for _, required := range []string{
		"packages/shared/core/drop.go",
		"packages/shared/network/protocol/message_drop.go",
		"packages/shared/network/protocol/message_hostile.go",
		"packages/shared/network/protocol/message_passive.go",
		"packages/shared/network/protocol/message_projectile.go",
	} {
		if !paths[required] {
			t.Fatalf("%s provenance misses the required path %s", domainEventChatFamily, required)
		}
	}
	if merged.SourceRevision != BaselineSourceRevision {
		t.Fatalf("merged manifest source_revision %s does not match the Go baseline %s", merged.SourceRevision, BaselineSourceRevision)
	}

	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	candidate := writeDomainEventManifestCandidate(t, root, exportRoot, merged)
	if exportRoot != "" && candidate == "" {
		t.Fatalf("manifest candidate export was rejected for %s", exportRoot)
	}
	if exportRoot != "" {
		reloaded, err := LoadInventory(candidate)
		if err != nil {
			t.Fatalf("reload published candidate: %v", err)
		}
		if reloaded.SourceRevision != BaselineSourceRevision {
			t.Fatalf("published candidate source_revision %s does not match the Go baseline %s", reloaded.SourceRevision, BaselineSourceRevision)
		}
	}
}

// TestDomainOracle_event_chat is the topic-named entry point the domain plan
// names for this node. It delegates to the same executed table, so the two
// filters select one source of expected results rather than two.
func TestDomainOracle_event_chat(t *testing.T) {
	TestDomainEventChatOracleExecutesEveryCase(t)
}

// domainEventChatRuleTally collects the accepted and rejected normalized
// outcomes the rule records, so a boundary assertion can look for the field
// value it names.
type domainEventChatRuleTally struct {
	accepted []map[string]any
	rejected []map[string]any
}

// domainEventChatFieldsContain reports whether one recorded outcome set holds
// a record whose named field carries the value a boundary assertion names.
func domainEventChatFieldsContain(records []map[string]any, field string, want any) bool {
	for _, fields := range records {
		if reflect.DeepEqual(fields[field], want) {
			return true
		}
	}
	return false
}

// domainEventChatRejectionNamesRule reports whether any recorded rejection
// names the broken rule a boundary assertion pins.
func domainEventChatRejectionNamesRule(tally *domainEventChatRuleTally, rule string) bool {
	for _, fields := range tally.rejected {
		if fields["rule"] == rule {
			return true
		}
	}
	return false
}

// domainEventChatRejectionCategoryOf reads one recorded rejection's category
// back from the executed outcome set for the vocabulary assertion; the map
// keys of the tally are not available because the family has one rule.
func domainEventChatRejectionCategoryOf(t *testing.T, fields map[string]any) string {
	t.Helper()
	rule, _ := fields["rule"].(string)
	for _, expectation := range domainEventChatRejectionExpectations {
		if expectation.rule == rule {
			return expectation.category
		}
	}
	t.Fatalf("recorded rejection names rule %q, which no expectation row pins", rule)
	return ""
}

// domainEventChatAssertCaseSpecs runs the production case validation over the
// whole selection, so a case with a bad path, digest, format or checkpoint
// fails here rather than surfacing later as a confusing coverage failure.
func domainEventChatAssertCaseSpecs(t *testing.T, root string, manifest Inventory) {
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

// domainEventChatAssertOutcomesDistinguishCases proves the executed evidence
// separates admitted records from rejections instead of publishing one
// constant answer, and that the rejections name a rule the authority
// publishes.
func domainEventChatAssertOutcomesDistinguishCases(t *testing.T, records []domainEventChatRecord) {
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
				t.Fatalf("record %s publishes rejection category %q, which is outside the frozen vocabulary", record.label, record.outcome.Category)
			}
			if strings.TrimSpace(record.outcome.Fields["rule"].(string)) == "" {
				t.Fatalf("record %s publishes a rejection that names no rule", record.label)
			}
		default:
			t.Fatalf("record %s publishes outcome kind %q, which is neither ok nor error", record.label, record.outcome.Kind)
		}
	}
	if accepted == 0 || rejected == 0 {
		t.Fatalf("executed evidence records %d accepted and %d rejected cases, want both non-zero", accepted, rejected)
	}
}

// domainEventChatFamilySpec indexes the working manifest by family identity.
func domainEventChatFamilySpec(manifest Inventory) (Family, bool) {
	for _, family := range manifest.Families {
		if family.ID == domainEventChatFamily {
			return family, true
		}
	}
	return Family{}, false
}

// domainEventChatCaseByID indexes a manifest selection by case identity.
func domainEventChatCaseByID(t *testing.T, manifest Inventory, id string) CaseSpec {
	t.Helper()
	for _, c := range manifest.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %s is missing from the working manifest", id)
	return CaseSpec{}
}
