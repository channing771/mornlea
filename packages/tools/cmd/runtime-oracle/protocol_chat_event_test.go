package main

// This file is the chat event publication packet producer group: the one
// Play server-to-client family whose text slot is reused by kind
// (`ChatEvent`, a companion speech line or the player's original command in
// one wire slot) registers a decode and an encode route through the shared
// packet-case runner, so every case is executed by the real Go codec rather
// than restated here.
//
// The decode cases are the Go decoder's own bytes: the twelve valid branch
// shapes, the reserved reject reason, the failed-task reason domain, the two
// text-slot boundaries, the identity and name gates, the event-identity gate
// and one trailing byte. The encode cases carry the canonical JSON fields and
// the reviewed wire the Go encoder publishes. Every negative carries exactly
// one violation.
//
// The combination rules are atomic on the DTO: a field that does not belong to
// the current kind and reason rejects the whole record. The wire itself cannot
// express two of those combinations, because it carries exactly one text slot
// and the decoder assigns it to the command or the speech field by the kind it
// has just read. The two slot-exclusivity cases therefore record the
// wire-observable form of the same rule: a non-speech branch whose text slot is
// empty, which that branch's own requirement refuses, and a speech branch whose
// text slot is empty, which the speech rule refuses. The DTO-level exclusivity
// itself is pinned by the Rust group test's mutated records.
//
// No case in this group declares a session recipient, a publish tick, a
// command sequence, a `/warp`, a targeting decision or a dialogue line: chat
// routing, event-id allocation and companion dialogue stay outside the
// protocol contract.

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

const (
	// chatEventFamily is the play chat event family this group registers.
	chatEventFamily = "protocol.server.ChatEvent"
	// chatEventVersion is the protocol version the family is pinned to.
	chatEventVersion = "45"
	// chatEventProducerID is the exporter's producer identity for this group's
	// candidate assets and merged manifest.
	chatEventProducerID = "runtime-oracle/protocol-chat-event"

	// chatEventCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	chatEventCorpusRelDir = corpusCasesRelDir + "/protocol/ChatEvent"
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	chatEventAcceptedDecodeID      = chatEventFamily + "/" + chatEventVersion + "/decode-accepted"
	chatEventInvalidFormatDecodeID = chatEventFamily + "/" + chatEventVersion + "/decode-invalid-format"
	chatEventUnknownCompDecodeID   = chatEventFamily + "/" + chatEventVersion + "/decode-unknown-companion"
	chatEventQueueFullDecodeID     = chatEventFamily + "/" + chatEventVersion + "/decode-queue-full"
	chatEventNotFollowingDecodeID  = chatEventFamily + "/" + chatEventVersion + "/decode-not-following"
	chatEventTaskStartedDecodeID   = chatEventFamily + "/" + chatEventVersion + "/decode-task-started"
	chatEventTaskProgressDecodeID  = chatEventFamily + "/" + chatEventVersion + "/decode-task-progress"
	chatEventTaskCompletedDecodeID = chatEventFamily + "/" + chatEventVersion + "/decode-task-completed"
	chatEventTaskFailedDecodeID    = chatEventFamily + "/" + chatEventVersion + "/decode-task-failed"
	chatEventTaskTimedOutDecodeID  = chatEventFamily + "/" + chatEventVersion + "/decode-task-timed-out"
	chatEventTaskStoppedDecodeID   = chatEventFamily + "/" + chatEventVersion + "/decode-task-stopped"
	chatEventSpeechDecodeID        = chatEventFamily + "/" + chatEventVersion + "/decode-speech"
	chatEventAcceptedEncodeID      = chatEventFamily + "/" + chatEventVersion + "/encode-accepted"
	chatEventSpeechEncodeID        = chatEventFamily + "/" + chatEventVersion + "/encode-speech"
	chatEventReservedReasonDecode  = chatEventFamily + "/" + chatEventVersion + "/decode-reserved-reason-three"
	chatEventFailReasonZeroDecode  = chatEventFamily + "/" + chatEventVersion + "/decode-failed-reason-zero"
	chatEventNonSpeechSpeechDecode = chatEventFamily + "/" + chatEventVersion + "/decode-non-speech-with-speech"
	chatEventSpeechCommandDecode   = chatEventFamily + "/" + chatEventVersion + "/decode-speech-with-command"
	chatEventZeroPlayerDecodeID    = chatEventFamily + "/" + chatEventVersion + "/decode-zero-player-uuid"
	chatEventNameDecodeID          = chatEventFamily + "/" + chatEventVersion + "/decode-noncanonical-player-name"
	chatEventCommandBoundEncodeID  = chatEventFamily + "/" + chatEventVersion + "/encode-command-above-bound"
	chatEventSpeechBoundDecodeID   = chatEventFamily + "/" + chatEventVersion + "/decode-speech-above-bound"
	chatEventZeroEventDecodeID     = chatEventFamily + "/" + chatEventVersion + "/decode-zero-event-id"
	chatEventTrailingDecodeID      = chatEventFamily + "/" + chatEventVersion + "/decode-trailing-byte"
)

// chatEventPlayerID is the reviewed player identity: a valid UUIDv4 whose last
// byte differs from the companion identity, so the two cannot be confused on
// the wire.
var chatEventPlayerID = core.PlayerID{
	0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
}

// chatEventCompanionID is the reviewed companion identity, absent only in the
// two rejection branches that never addressed a companion.
var chatEventCompanionID = companion.ID{
	0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x10,
}

// The reviewed scalar fields every branch carries.
const (
	chatEventEventID       uint64 = 0x0102_0304_0506_0708
	chatEventPlayerName           = "Alice"
	chatEventCompanionName        = "Mira"
	chatEventCommand              = "@mira follow"
	chatEventSpeech               = "On it."
)

// chatEventFamilyKeys pins the family's complete packet key. A case names its
// key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs.
var chatEventFamilyKeys = map[string]PacketKeySpec{
	chatEventFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 16},
}

// chatEventFamilies lists the family in registry order, with the corpus
// directory that holds its assets. The order is the review order, not a
// dispatch table.
func chatEventFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{chatEventFamily, chatEventCorpusRelDir},
	}
}

// chatEventBranch is one legal branch shape: the kind and reason slot the wire
// carries, the companion identity and name the branch requires, and the one
// text slot the kind selects.
type chatEventBranch struct {
	label         string
	kind          protocol.ChatEventKind
	reason        protocol.ChatRejectReason
	companionID   companion.ID
	companionName string
	command       string
	speech        string
}

// chatEventBranches lists the twelve legal branch shapes the wire publishes:
// the accepted command, the four rejections, the five plain task facts, the
// failed task fact with one failure reason, and the companion speech line.
//
// The five plain facts and the five failure reasons differ only in the kind and
// reason bytes, so the table enumerates them rather than deriving them from a
// loop; the remaining four failure reasons are pinned by the Rust group test and
// by the Rust domain chat-event suite rather than by corpus cases.
func chatEventBranches() []chatEventBranch {
	return []chatEventBranch{
		{
			label: "accepted", kind: protocol.ChatEventAccepted, reason: protocol.ChatRejectNone,
			companionID: chatEventCompanionID, companionName: chatEventCompanionName,
			command: chatEventCommand,
		},
		{
			// A malformed command never addressed a companion, so the branch
			// carries the absent identity, no name and no restated command.
			label: "invalid format", kind: protocol.ChatEventRejected,
			reason: protocol.ChatRejectInvalidFormat, companionID: companion.ID{},
		},
		{
			// Only the target name survives, so the player can check the
			// spelling.
			label: "unknown companion", kind: protocol.ChatEventRejected,
			reason: protocol.ChatRejectUnknownCompanion, companionID: companion.ID{},
			companionName: chatEventCompanionName,
		},
		{
			label: "queue full", kind: protocol.ChatEventRejected,
			reason: protocol.ChatRejectQueueFull, companionID: chatEventCompanionID,
			companionName: chatEventCompanionName, command: chatEventCommand,
		},
		{
			label: "not following", kind: protocol.ChatEventRejected,
			reason: protocol.ChatRejectNotFollowing, companionID: chatEventCompanionID,
			companionName: chatEventCompanionName, command: chatEventCommand,
		},
		{
			label: "task started", kind: protocol.ChatEventTaskStarted,
			reason: protocol.ChatRejectNone, companionID: chatEventCompanionID,
			companionName: chatEventCompanionName, command: chatEventCommand,
		},
		{
			label: "task progress", kind: protocol.ChatEventTaskProgress,
			reason: protocol.ChatRejectNone, companionID: chatEventCompanionID,
			companionName: chatEventCompanionName, command: chatEventCommand,
		},
		{
			label: "task completed", kind: protocol.ChatEventTaskCompleted,
			reason: protocol.ChatRejectNone, companionID: chatEventCompanionID,
			companionName: chatEventCompanionName, command: chatEventCommand,
		},
		{
			label: "task timed out", kind: protocol.ChatEventTaskTimedOut,
			reason: protocol.ChatRejectNone, companionID: chatEventCompanionID,
			companionName: chatEventCompanionName, command: chatEventCommand,
		},
		{
			label: "task stopped", kind: protocol.ChatEventTaskStopped,
			reason: protocol.ChatRejectNone, companionID: chatEventCompanionID,
			companionName: chatEventCompanionName, command: chatEventCommand,
		},
		{
			label: "task failed", kind: protocol.ChatEventTaskFailed,
			reason:      protocol.ChatRejectReason(protocol.TaskFailPlannerUnavailable),
			companionID: chatEventCompanionID, companionName: chatEventCompanionName,
			command: chatEventCommand,
		},
		{
			label: "speech", kind: protocol.ChatEventCompanionSpeech,
			reason: protocol.ChatRejectNone, companionID: chatEventCompanionID,
			companionName: chatEventCompanionName, speech: chatEventSpeech,
		},
	}
}

// chatEventDTO renders one branch as the Go DTO the encoder validates.
func chatEventDTO(branch chatEventBranch) protocol.ChatEvent {
	return protocol.ChatEvent{
		EventID:       chatEventEventID,
		PlayerID:      chatEventPlayerID,
		PlayerName:    chatEventPlayerName,
		CompanionID:   branch.companionID,
		CompanionName: branch.companionName,
		Kind:          branch.kind,
		RejectReason:  branch.reason,
		Command:       branch.command,
		Speech:        branch.speech,
	}
}

// chatEventWire renders one branch as the exact wire payload the Go encoder
// publishes: the event identity, the player identity and name, the companion
// identity and name, the kind, the reason, and the one text slot the kind
// selects.
//
// The builder writes the canonical uvarint length prefixes the Go primitive
// writes, so a literal built here is byte-equal to the encoder's own output,
// which `TestProtocolChatEventGoEncoderPublishesEveryBranchWire` pins.
func chatEventWire(branch chatEventBranch) []byte {
	wire := make([]byte, 0, protocol.ChatEventMaxWireBytes)
	wire = append(wire, chatEventU64(chatEventEventID)...)
	wire = append(wire, chatEventPlayerID[:]...)
	wire = append(wire, chatEventString(chatEventPlayerName)...)
	wire = append(wire, branch.companionID[:]...)
	wire = append(wire, chatEventString(branch.companionName)...)
	wire = append(wire, byte(branch.kind), byte(branch.reason))
	if branch.kind == protocol.ChatEventCompanionSpeech {
		wire = append(wire, chatEventString(branch.speech)...)
	} else {
		wire = append(wire, chatEventString(branch.command)...)
	}
	return wire
}

// chatEventU64 renders one little-endian u64.
func chatEventU64(value uint64) []byte {
	wire := make([]byte, 8)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// chatEventString renders one canonical uvarint length prefix followed by the
// text bytes, matching the Go string primitive's shortest form.
func chatEventString(value string) []byte {
	wire := append([]byte(nil), chatEventUvarint(uint32(len(value)))...)
	return append(wire, value...)
}

// chatEventUvarint renders the canonical uvarint of one value.
func chatEventUvarint(value uint32) []byte {
	var encoded []byte
	for value >= 1<<7 {
		encoded = append(encoded, byte(value)|0x80)
		value >>= 7
	}
	return append(encoded, byte(value))
}

// chatEventRejectionCategory resolves the language-neutral rejection category
// for one real Go codec failure.
//
// The structural boundaries are resolved from the Go codec's own messages. The
// combination boundaries are different: `ChatEvent.Validate` folds several
// relations into one message, so a message alone cannot tell an identity
// refusal from a name refusal. The mapping therefore records, per message, the
// closed set of categories that message can publish, and the case's own
// declared violation picks among them; a declared category outside the
// message's set is a hard error rather than a silently accepted classification.
//
// The folded player-identity message publishes either category, because the
// zero event identity and an invalid player identity are `invalid-identity`
// while a non-canonical player name is a text boundary. The failed-task message
// publishes either the reason-domain category or the value category, because
// the reason enum and the payload relations share it. The two string boundaries
// reuse the node-1.5 ruling: a declared length the payload cannot complete is
// answered by the Go string primitive with the same sentinel as a malformed
// UTF-8 text, so a rejected proper prefix of the reviewed payload is the
// truncated category and every other invalid-string rejection is a value
// boundary. The Rust consumer reports the same categories from its own error
// variants, so both implementations publish one category for this family.
func chatEventRejectionCategory(err error, declared string, derivedFrom, input []byte) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "payload exceeds fixed maximum"):
		return "capacity", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	case strings.Contains(message, "invalid string"):
		if isRejectedPrefix(derivedFrom, input) {
			return "truncated", true
		}
		return "invalid-value", true
	}
	allowed, classified := chatEventCategoryClasses(message)
	if !classified {
		return "", false
	}
	for _, category := range allowed {
		if category == declared {
			return declared, true
		}
	}
	return "", false
}

// chatEventCategoryClasses lists the closed category set each folded validator
// message can publish, and reports whether the message belongs to this family's
// validator at all.
func chatEventCategoryClasses(message string) ([]string, bool) {
	switch {
	case strings.Contains(message, "invalid chat event player identity"):
		// The zero event identity and an invalid player identity are the
		// identity boundary; the non-canonical player name is a text boundary.
		return []string{"invalid-identity", "invalid-value"}, true
	case strings.Contains(message, "invalid chat rejection reason"):
		return []string{"invalid-enum"}, true
	case strings.Contains(message, "invalid chat event kind"):
		return []string{"invalid-enum"}, true
	case strings.Contains(message, "invalid failed task chat event"):
		// The reason domain is the enum boundary; the payload relations are
		// value boundaries.
		return []string{"invalid-enum", "invalid-value"}, true
	case strings.Contains(message, "chat event kind carries companion speech"),
		strings.Contains(message, "invalid accepted chat event"),
		strings.Contains(message, "invalid-format chat event leaks"),
		strings.Contains(message, "unknown-companion chat event carries"),
		strings.Contains(message, "lacks companion identity or command"),
		strings.Contains(message, "invalid ") && strings.Contains(message, " task chat event"),
		strings.Contains(message, "invalid companion speech chat event"):
		return []string{"invalid-value"}, true
	}
	return nil, false
}

// chatEventDerivedFrom lists the reviewed payload the truncated case was
// derived from, so the boundary resolver can tell an incomplete payload from a
// mutated one.
func chatEventDerivedFrom(caseID string) []byte {
	if caseID == chatEventTrailingDecodeID {
		return append([]byte(nil), chatEventAcceptedWire()...)
	}
	return nil
}

// chatEventAcceptedWire is the reviewed accepted branch payload, the derivation
// base the truncated and trailing cases reuse.
func chatEventAcceptedWire() []byte {
	return chatEventWire(chatEventBranches()[0])
}

// chatEventPlayerNameWire renders the accepted branch payload with the player
// name slot replaced, so a name-boundary case carries exactly one violation and
// keeps every other field at its reviewed value.
//
// The player name is the first length-prefixed slot, so its prefix sits at the
// fixed offset after the event identity and the player identity.
func chatEventPlayerNameWire(playerName string) []byte {
	wire := append([]byte(nil), chatEventAcceptedWire()...)
	start := 8 + 16
	prefix, err := chatEventDecodePrefix(wire, start)
	if err != nil {
		panic("runtime-oracle: the reviewed payload names no player name prefix: " + err.Error())
	}
	replacement := chatEventString(playerName)
	rewritten := make([]byte, 0, len(wire)-prefix+len(replacement))
	rewritten = append(rewritten, wire[:start]...)
	rewritten = append(rewritten, replacement...)
	rewritten = append(rewritten, wire[start+prefix:]...)
	return rewritten
}

// chatEventDecodePrefix reports the byte length of the canonical uvarint length
// prefix that starts at one offset.
func chatEventDecodePrefix(wire []byte, offset int) (int, error) {
	for index := offset; index < len(wire) && index < offset+5; index++ {
		if wire[index] < 0x80 {
			return index - offset + 1, nil
		}
	}
	return 0, fmt.Errorf("no canonical uvarint prefix at offset %d", offset)
}

// chatEventPacketKey resolves one packet key to the Go state and numeric ID
// the codec dispatches on, and reports whether the packet travels
// server-to-client.
func chatEventPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := chatEventFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no chat event producer owns", c.ID, c.Family)
	}
	if *c.PacketKey != registered {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names key %+v, want %+v", c.ID, *c.PacketKey, registered)
	}
	if registered.State != packetStatePlay {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names state %q, want play", c.ID, registered.State)
	}
	if registered.Direction != packetDirectionServer {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names direction %q, want server-to-client", c.ID, registered.Direction)
	}
	return protocol.StatePlay, registered.ID, nil
}

// chatEventFields renders the semantic fields one chat event publishes.
//
// The event identity renders as a decimal string so the full u64 range stays
// lossless, both identities render as 32-lowercase-hexadecimal text, and the
// names, command and speech render verbatim. The companion identity stays the
// raw wire form, so the two branches that never addressed a companion publish
// the exact zero bytes instead of a pre-validated identity. The kind and reason
// render as the plain integers the wire carries, and both text slots are
// published with the one the kind selected filled and the other empty, because
// the reviewed record carries both fields and the wire carries one slot.
func chatEventFields(event protocol.ChatEvent) map[string]any {
	return map[string]any{
		"event_id":       strconv.FormatUint(event.EventID, 10),
		"player_id":      chatEventIDText(event.PlayerID),
		"player_name":    event.PlayerName,
		"companion_id":   chatEventIDText(event.CompanionID),
		"companion_name": event.CompanionName,
		"kind":           uint8(event.Kind),
		"reason":         uint8(event.RejectReason),
		"command":        event.Command,
		"speech":         event.Speech,
	}
}

// chatEventIDText renders one 16-byte identity as lowercase hexadecimal.
func chatEventIDText(id [16]byte) string {
	const digits = "0123456789abcdef"
	rendered := make([]byte, 0, 32)
	for _, byteValue := range id {
		rendered = append(rendered, digits[byteValue>>4], digits[byteValue&0x0f])
	}
	return string(rendered)
}

// newChatEventCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which this family never
// touches; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newChatEventCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// runChatEventDecode executes one chat event decode case through the real Go
// decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeServer` and classifies
// the failure it returns. It never reimplements the length, name or combination
// rules, so the recorded outcome is whatever the production codec decides about
// these exact bytes.
func runChatEventDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := chatEventPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newChatEventCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeServer(state, packetID, input)
	if err != nil {
		declared := chatEventDeclaredCategory(c.ID)
		category, classified := chatEventRejectionCategory(err, declared, chatEventDerivedFrom(c.ID), input)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	event, owned := packet.(protocol.ChatEvent)
	if !owned {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: chatEventFields(event)}, nil, nil
}

// chatEventDeclaredCategory lists the reviewed category each negative case
// declares, which is the boundary its single violation owns.
func chatEventDeclaredCategory(caseID string) string {
	switch caseID {
	case chatEventZeroEventDecodeID, chatEventZeroPlayerDecodeID:
		return "invalid-identity"
	case chatEventReservedReasonDecode, chatEventFailReasonZeroDecode:
		return "invalid-enum"
	default:
		return "invalid-value"
	}
}

// chatEventEncodeRequest is the canonical JSON field input one encode case
// carries: the reviewed record's own fields, with no session, publish tick or
// command sequence.
type chatEventEncodeRequest struct {
	EventID       uint64 `json:"event_id"`
	PlayerID      string `json:"player_id"`
	PlayerName    string `json:"player_name"`
	CompanionID   string `json:"companion_id"`
	CompanionName string `json:"companion_name"`
	Kind          uint8  `json:"kind"`
	Reason        uint8  `json:"reason"`
	Command       string `json:"command"`
	Speech        string `json:"speech"`
}

// chatEventRequest renders one branch as its canonical JSON request fields.
func chatEventRequest(branch chatEventBranch) chatEventEncodeRequest {
	return chatEventEncodeRequest{
		EventID:       chatEventEventID,
		PlayerID:      chatEventIDText(chatEventPlayerID),
		PlayerName:    chatEventPlayerName,
		CompanionID:   chatEventIDText(branch.companionID),
		CompanionName: branch.companionName,
		Kind:          uint8(branch.kind),
		Reason:        uint8(branch.reason),
		Command:       branch.command,
		Speech:        branch.speech,
	}
}

// runChatEventEncode executes one chat event encode case through the real Go
// encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which publishes the record through its own wire writer and then
// reports any primitive-level refusal. The read-back runs the decode path's
// full validator, so bytes the authority would refuse are never recorded as
// evidence.
func runChatEventEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := chatEventPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newChatEventCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var request chatEventEncodeRequest
	if err := decodeEncodeRequest(c, input, &request); err != nil {
		return Outcome{}, nil, err
	}
	playerID, err := chatEventParseID(request.PlayerID)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: player identity: %w", c.ID, err)
	}
	companionID, err := chatEventParseID(request.CompanionID)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: companion identity: %w", c.ID, err)
	}
	packet := protocol.ChatEvent{
		EventID:       request.EventID,
		PlayerID:      playerID,
		PlayerName:    request.PlayerName,
		CompanionID:   companionID,
		CompanionName: request.CompanionName,
		Kind:          protocol.ChatEventKind(request.Kind),
		RejectReason:  protocol.ChatRejectReason(request.Reason),
		Command:       request.Command,
		Speech:        request.Speech,
	}

	encodedID, payload, err := wireCodec.EncodeServer(state, packet)
	if err != nil {
		category, classified := chatEventRejectionCategory(err, chatEventDeclaredCategory(c.ID), nil, nil)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified encode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	if encodedID != packetID {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoder published packet ID %d, want %d", c.ID, encodedID, packetID)
	}
	decoded, err := wireCodec.DecodeServer(state, encodedID, payload)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoded payload does not decode back: %w", c.ID, err)
	}
	if !reflect.DeepEqual(decoded, packet) {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoded payload decodes to %#v, want %#v", c.ID, decoded, packet)
	}
	fields := chatEventFields(packet)
	sum := sha256.Sum256(payload)
	return Outcome{
		Kind:                 "ok",
		Category:             packetOutcomeCategory,
		Fields:               fields,
		EncodedPayloadDigest: fmt.Sprintf("sha256:%x", sum),
	}, payload, nil
}

// chatEventParseID decodes one 32-hexadecimal identity field.
func chatEventParseID(text string) ([16]byte, error) {
	var id [16]byte
	if len(text) != 32 {
		return id, fmt.Errorf("identity %q is not 32 hexadecimal digits", text)
	}
	for index := 0; index < len(id); index++ {
		high, ok := chatEventHexDigit(text[index*2])
		if !ok {
			return id, fmt.Errorf("identity %q is not hexadecimal", text)
		}
		low, ok := chatEventHexDigit(text[index*2+1])
		if !ok {
			return id, fmt.Errorf("identity %q is not hexadecimal", text)
		}
		id[index] = high<<4 | low
	}
	return id, nil
}

// chatEventHexDigit decodes one lowercase or uppercase hexadecimal digit.
func chatEventHexDigit(digit byte) (byte, bool) {
	switch {
	case digit >= '0' && digit <= '9':
		return digit - '0', true
	case digit >= 'a' && digit <= 'f':
		return digit - 'a' + 10, true
	case digit >= 'A' && digit <= 'F':
		return digit - 'A' + 10, true
	}
	return 0, false
}

// chatEventCorpusRoutes is the closed route map the family executes.
func chatEventCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range chatEventFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: chatEventVersion, Operation: "decode"}] = runChatEventDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: chatEventVersion, Operation: "encode"}] = runChatEventEncode
	}
	return routes
}

// chatEventCaseDefinition declares one case from literals before any producer
// runs, so the expectation is the review contract rather than a producer
// result.
type chatEventCaseDefinition struct {
	id     string
	family string
	relDir string
	op     string
	// input is the binary decode payload, already mutated where the case is a
	// malformed one.
	input []byte
	// request is the JSON encode payload, nil for a decode case.
	request any
	// wire is the expected encoded payload for an encode case.
	wire []byte
	// expect is the normalized outcome an execution has to reproduce.
	expect Outcome
}

// chatEventCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid cases
// use the branch payloads the Go encoder produces, and the malformed cases
// mutate one branch payload at one boundary each, so each rejection names the
// boundary that owns it and no case combines two violations.
//
// The two slot-exclusivity cases are the wire-observable form of the DTO rule
// the record's combination validator publishes: the wire carries exactly one
// text slot and the decoder assigns it to the command or the speech field by
// the kind it has just read, so a non-speech branch that also carried a line is
// not a constructible payload. What the wire can express is the branch's own
// requirement at that slot — a non-speech branch whose text slot is empty, and a
// speech branch whose text slot is empty — and both are single-violation
// refusals of the same rule. The DTO-level exclusivity is pinned by the Rust
// group test's mutated records.
func chatEventCaseDefinitions() []chatEventCaseDefinition {
	branches := chatEventBranches()
	accepted := branches[0]
	speech := branches[len(branches)-1]

	valid := func(branch chatEventBranch) Outcome {
		return Outcome{
			Kind:     "ok",
			Category: packetOutcomeCategory,
			Fields:   chatEventFields(chatEventDTO(branch)),
		}
	}
	invalidValue := Outcome{Kind: "error", Category: "invalid-value"}
	invalidEnum := Outcome{Kind: "error", Category: "invalid-enum"}
	invalidIdentity := Outcome{Kind: "error", Category: "invalid-identity"}
	trailing := Outcome{Kind: "error", Category: "trailing"}

	definitions := make([]chatEventCaseDefinition, 0, 24)
	for _, branch := range branches {
		definitions = append(definitions, chatEventCaseDefinition{
			id:     chatEventDecodeCaseID(branch),
			family: chatEventFamily,
			relDir: chatEventCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), chatEventWire(branch)...),
			expect: valid(branch),
		})
	}
	// Two encode cases: the accepted branch and the speech branch, whose text
	// slot the kind selects differently. Both carry the reviewed wire literal
	// and the digest of it.
	for _, branch := range []chatEventBranch{accepted, speech} {
		definitions = append(definitions, chatEventCaseDefinition{
			id:      chatEventEncodeCaseID(branch),
			family:  chatEventFamily,
			relDir:  chatEventCorpusRelDir,
			op:      "encode",
			request: chatEventRequest(branch),
			wire:    append([]byte(nil), chatEventWire(branch)...),
			expect:  valid(branch),
		})
	}

	// Reason 3 is reserved and unassigned, so a rejected branch carrying it is
	// refused by the reason enum before any payload relation.
	definitions = append(definitions, chatEventCaseDefinition{
		id:     chatEventReservedReasonDecode,
		family: chatEventFamily,
		relDir: chatEventCorpusRelDir,
		op:     "decode",
		input: chatEventWire(chatEventBranch{
			kind: protocol.ChatEventRejected, reason: protocol.ChatRejectReason(3),
			companionID: chatEventCompanionID, companionName: chatEventCompanionName,
			command: chatEventCommand,
		}),
		expect: invalidEnum,
	})
	// A failed task carries a task failure reason, so reason zero is outside
	// the reason domain the failed branch publishes.
	definitions = append(definitions, chatEventCaseDefinition{
		id:     chatEventFailReasonZeroDecode,
		family: chatEventFamily,
		relDir: chatEventCorpusRelDir,
		op:     "decode",
		input: chatEventWire(chatEventBranch{
			kind: protocol.ChatEventTaskFailed, reason: protocol.ChatRejectNone,
			companionID: chatEventCompanionID, companionName: chatEventCompanionName,
			command: chatEventCommand,
		}),
		expect: invalidEnum,
	})
	// The two slot-exclusivity boundaries at their wire-observable form.
	emptyAccepted := accepted
	emptyAccepted.command = ""
	definitions = append(definitions, chatEventCaseDefinition{
		id:     chatEventNonSpeechSpeechDecode,
		family: chatEventFamily,
		relDir: chatEventCorpusRelDir,
		op:     "decode",
		input:  append([]byte(nil), chatEventWire(emptyAccepted)...),
		expect: invalidValue,
	})
	emptySpeech := speech
	emptySpeech.speech = ""
	definitions = append(definitions, chatEventCaseDefinition{
		id:     chatEventSpeechCommandDecode,
		family: chatEventFamily,
		relDir: chatEventCorpusRelDir,
		op:     "decode",
		input:  append([]byte(nil), chatEventWire(emptySpeech)...),
		expect: invalidValue,
	})
	// A zero player identity is refused by the global identity gate.
	definitions = append(definitions, chatEventCaseDefinition{
		id:     chatEventZeroPlayerDecodeID,
		family: chatEventFamily,
		relDir: chatEventCorpusRelDir,
		op:     "decode",
		input: func() []byte {
			wire := append([]byte(nil), chatEventAcceptedWire()...)
			copy(wire[8:24], make([]byte, 16))
			return wire
		}(),
		expect: invalidIdentity,
	})
	// A name the authority would have to trim is a text boundary, not an
	// identity boundary, even though the Go validator folds both into one
	// message.
	definitions = append(definitions, chatEventCaseDefinition{
		id:     chatEventNameDecodeID,
		family: chatEventFamily,
		relDir: chatEventCorpusRelDir,
		op:     "decode",
		input:  chatEventPlayerNameWire(" Alice"),
		expect: invalidValue,
	})
	// A command above the byte bound is refused by the encoder's string
	// primitive, which is the boundary both sides publish.
	overBound := accepted
	overBound.command = strings.Repeat("a", 1025)
	definitions = append(definitions, chatEventCaseDefinition{
		id:      chatEventCommandBoundEncodeID,
		family:  chatEventFamily,
		relDir:  chatEventCorpusRelDir,
		op:      "encode",
		request: chatEventRequest(overBound),
		expect:  invalidValue,
	})
	// The decode side cannot carry the command bound: the payload ceiling
	// answers a length the decoder cannot reach first.
	definitions = append(definitions, chatEventCaseDefinition{
		id:     chatEventSpeechBoundDecodeID,
		family: chatEventFamily,
		relDir: chatEventCorpusRelDir,
		op:     "decode",
		input: chatEventWire(chatEventBranch{
			kind: protocol.ChatEventCompanionSpeech, reason: protocol.ChatRejectNone,
			companionID: chatEventCompanionID, companionName: chatEventCompanionName,
			speech: strings.Repeat("a", 257),
		}),
		expect: invalidValue,
	})
	// The event identity names the acknowledgment, so a zero value is absent.
	definitions = append(definitions, chatEventCaseDefinition{
		id:     chatEventZeroEventDecodeID,
		family: chatEventFamily,
		relDir: chatEventCorpusRelDir,
		op:     "decode",
		input: func() []byte {
			wire := append([]byte(nil), chatEventAcceptedWire()...)
			copy(wire[:8], make([]byte, 8))
			return wire
		}(),
		expect: invalidIdentity,
	})
	definitions = append(definitions, chatEventCaseDefinition{
		id:     chatEventTrailingDecodeID,
		family: chatEventFamily,
		relDir: chatEventCorpusRelDir,
		op:     "decode",
		input:  append(append([]byte(nil), chatEventAcceptedWire()...), 0x00),
		expect: trailing,
	})

	return definitions
}

// chatEventDecodeCaseID names one branch's decode case identity.
func chatEventDecodeCaseID(branch chatEventBranch) string {
	return chatEventFamily + "/" + chatEventVersion + "/decode-" + strings.ReplaceAll(branch.label, " ", "-")
}

// chatEventEncodeCaseID names one branch's encode case identity.
func chatEventEncodeCaseID(branch chatEventBranch) string {
	return chatEventFamily + "/" + chatEventVersion + "/encode-" + strings.ReplaceAll(branch.label, " ", "-")
}

// chatEventLabel renders one case's asset label from its identity.
func chatEventLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// buildChatEventCandidate builds one case's manifest entry and asset bytes from
// its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildChatEventCandidate(t *testing.T, definition chatEventCaseDefinition) chatEventCandidate {
	t.Helper()

	label := chatEventLabel(definition.id)
	inputPath := filepath.ToSlash(filepath.Join(definition.relDir, label+".input"+inputExtension(definition.op)))
	expectedPath := filepath.ToSlash(filepath.Join(definition.relDir, label+".expected.json"))

	var input []byte
	switch {
	case definition.op == "decode":
		input = append([]byte(nil), definition.input...)
	case definition.request != nil:
		rendered, err := json.MarshalIndent(definition.request, "", "  ")
		if err != nil {
			t.Fatalf("case %s: marshal encode input: %v", definition.id, err)
		}
		input = append(rendered, '\n')
	default:
		t.Fatalf("case %s: neither a decode payload nor an encode request", definition.id)
	}

	expect := definition.expect
	if definition.wire != nil {
		sum := sha256.Sum256(definition.wire)
		expect.EncodedPayloadDigest = fmt.Sprintf("sha256:%x", sum)
	}
	expected, err := marshalIndentedOutcome(expect)
	if err != nil {
		t.Fatalf("case %s: render expectation: %v", definition.id, err)
	}
	assets := map[string][]byte{
		inputPath:    input,
		expectedPath: append(expected, '\n'),
	}

	spec := CaseSpec{
		ID:           definition.id,
		Family:       definition.family,
		Version:      chatEventVersion,
		Operation:    definition.op,
		PacketKey:    chatEventKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return chatEventCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// chatEventKeyPointer resolves one family's reviewed packet key for a case spec.
func chatEventKeyPointer(family string) *PacketKeySpec {
	key, owned := chatEventFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// chatEventCandidate is one reviewed case: its manifest specification, the
// exact asset bytes it publishes, and the expectation an independent execution
// has to reproduce.
type chatEventCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which the
	// producer's own bytes are compared against.
	Wire []byte
}

// chatEventCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func chatEventCandidates(t *testing.T, root string) []chatEventCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := chatEventCaseDefinitions()
	candidates := make([]chatEventCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildChatEventCandidate(t, definition)
		if candidate.Spec.PacketKey == nil {
			t.Fatalf("case %s names family %q, which has no reviewed packet key", candidate.Spec.ID, candidate.Spec.Family)
		}
		for relative, want := range candidate.Assets {
			tracked, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
			if readErr != nil {
				if !os.IsNotExist(readErr) {
					t.Fatalf("read tracked asset %s: %v", relative, readErr)
				}
				continue
			}
			if !bytes.Equal(tracked, want) {
				t.Fatalf("tracked asset %s differs from the reviewed candidate", relative)
			}
		}
		if existing, ok := registered[candidate.Spec.ID]; ok {
			if !reflect.DeepEqual(existing, candidate.Spec) {
				t.Fatalf("tracked case %s is %#v, want the reviewed candidate %#v", candidate.Spec.ID, existing, candidate.Spec)
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

// chatEventRegisteredCase reports whether the base manifest already carries one
// case identity, so a re-merge of an integrated candidate adds nothing.
func chatEventRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// chatEventSelection is this group's registration: its cases, the Go sources
// its rules are read from, and the routes the family executes.
func chatEventSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := chatEventCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range chatEventFamilies() {
		sources[family.id] = chatEventServerSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(chatEventFamilies())*2)
	for _, family := range chatEventFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: chatEventVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: chatEventVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  chatEventProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// chatEventServerSources lists the Go sources the chat event family reads its
// rules from: the shared server codec dispatch with its payload ceiling, the
// family's own message file, which owns the combination validator, and the
// family's wire codec, which owns the field order and the kind-selected text
// slot.
func chatEventServerSources(family string) []string {
	shared := []string{
		"packages/shared/network/codec/codec_server.go",
		"packages/shared/network/protocol/message_companion.go",
		"packages/shared/network/codec/companion_wire.go",
	}
	if family != chatEventFamily {
		return nil
	}
	return shared
}

// chatEventManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func chatEventManifest(t *testing.T, root string, candidates []chatEventCandidate) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)

	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	cloned := Inventory{
		SchemaVersion:  frozen.SchemaVersion,
		SourceRevision: frozen.SourceRevision,
		Identities:     frozen.Identities,
		Families:       append([]Family(nil), frozen.Families...),
		Cases:          cases,
	}
	for index := range cloned.Families {
		family := cloned.Families[index].ID
		if !chatEventOwnsFamily(family) {
			cloned.Families[index].Cases = nil
			continue
		}
		var listed []string
		for _, c := range cloned.Cases {
			if c.Family == family {
				listed = append(listed, c.ID)
			}
		}
		sort.Strings(listed)
		cloned.Families[index].Cases = listed
	}

	encoded, err := encodeInventory(cloned)
	if err != nil {
		t.Fatalf("encode chat event working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write chat event working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load chat event working manifest: %v", err)
	}
	return loaded
}

// chatEventOwnsFamily reports whether this group registers cases for one family.
func chatEventOwnsFamily(family string) bool {
	_, owned := chatEventFamilyKeys[family]
	return owned
}

// chatEventScratchRoot stages this group's candidate assets in a
// harness-owned temporary directory, because a corpus case has to resolve under
// the root the runner is given.
func chatEventScratchRoot(t *testing.T, root string, candidates []chatEventCandidate) string {
	t.Helper()
	staged := t.TempDir()
	for _, candidate := range candidates {
		for relative, data := range candidate.Assets {
			target := filepath.Join(staged, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create staged parent for %s: %v", relative, err)
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				t.Fatalf("stage candidate asset %s: %v", relative, err)
			}
		}
	}
	return staged
}

// chatEventCandidateByID resolves one candidate by its case identity.
func chatEventCandidateByID(t *testing.T, candidates []chatEventCandidate, id string) chatEventCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return chatEventCandidate{}
}

// chatEventObservation resolves one executed observation by its case identity.
func chatEventObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// chatEventExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var chatEventExportPublished bool

// chatEventCandidatesExport publishes the reviewed candidates and the complete
// merged manifest candidate through the existing external exporter and returns
// the published producer directory. An unset export variable publishes nothing
// and returns "", so an ordinary test run never writes outside its own temporary
// storage.
func chatEventCandidatesExport(t *testing.T, root string, candidates []chatEventCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if chatEventExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-chat-event")
	}
	chatEventExportPublished = true

	manifest, err := encodeInventory(merged)
	if err != nil {
		t.Fatalf("encode manifest candidate: %v", err)
	}
	assets := []generatedAsset{{RelativePath: "contracts.json", Data: append(manifest, '\n')}}
	relatives := make([]string, 0)
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		for relative := range candidate.Assets {
			if seen[relative] {
				continue
			}
			seen[relative] = true
			relatives = append(relatives, relative)
		}
	}
	sort.Strings(relatives)
	for _, relative := range relatives {
		for _, candidate := range candidates {
			if data, ok := candidate.Assets[relative]; ok {
				assets = append(assets, generatedAsset{RelativePath: relative, Data: data})
				break
			}
		}
	}
	published, err := exportGeneratedAssets(root, exportRoot, chatEventProducerID, assets)
	if err != nil {
		t.Fatalf("export chat event candidates: %v", err)
	}
	return published
}

// chatEventDecodeCaseIDs lists the valid decode cases the producer pins.
func chatEventDecodeCaseIDs() []string {
	branches := chatEventBranches()
	ids := make([]string, 0, len(branches))
	for _, branch := range branches {
		ids = append(ids, chatEventDecodeCaseID(branch))
	}
	return ids
}

// chatEventEncodeCaseIDs lists the valid encode cases the producer pins.
func chatEventEncodeCaseIDs() []string {
	return []string{
		chatEventAcceptedEncodeID,
		chatEventSpeechEncodeID,
	}
}

// chatEventRejectedDecodeCaseIDs lists the malformed decode cases.
func chatEventRejectedDecodeCaseIDs() []string {
	return []string{
		chatEventReservedReasonDecode,
		chatEventFailReasonZeroDecode,
		chatEventNonSpeechSpeechDecode,
		chatEventSpeechCommandDecode,
		chatEventZeroPlayerDecodeID,
		chatEventNameDecodeID,
		chatEventSpeechBoundDecodeID,
		chatEventZeroEventDecodeID,
		chatEventTrailingDecodeID,
	}
}

// chatEventRejectedEncodeCaseIDs lists the invalid encode cases.
func chatEventRejectedEncodeCaseIDs() []string {
	return []string{chatEventCommandBoundEncodeID}
}

// TestProtocolChatEventOracleDecodesEveryValidCase pins that the real Go decoder
// publishes the canonical fields for all twelve legal branch shapes, with the
// identities, names and the kind-selected text slot carried verbatim.
func TestProtocolChatEventOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := chatEventCandidates(t, root)

	for _, id := range chatEventDecodeCaseIDs() {
		candidate := chatEventCandidateByID(t, candidates, id)
		outcome, encoded, err := runChatEventDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runChatEventDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolChatEventOracleEncodesCanonicalWire pins the encode producer
// against the reviewed wire literals and against the recorded digests.
func TestProtocolChatEventOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := chatEventCandidates(t, root)

	for _, id := range chatEventEncodeCaseIDs() {
		candidate := chatEventCandidateByID(t, candidates, id)
		outcome, encoded, err := runChatEventEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runChatEventEncode(%s): %v", id, err)
		}
		if !bytes.Equal(encoded, candidate.Wire) {
			t.Fatalf("case %s encoded %x, want %x", id, encoded, candidate.Wire)
		}
		sum := sha256.Sum256(candidate.Wire)
		if outcome.EncodedPayloadDigest != fmt.Sprintf("sha256:%x", sum) {
			t.Fatalf("case %s digest = %s, want sha256:%x", id, outcome.EncodedPayloadDigest, sum)
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolChatEventOracleRejectsMalformedCasesAtTheirBoundary pins that
// every malformed decode case and the invalid encode case is refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolChatEventOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := chatEventCandidates(t, root)

	for _, id := range chatEventRejectedDecodeCaseIDs() {
		candidate := chatEventCandidateByID(t, candidates, id)
		outcome, encoded, err := runChatEventDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runChatEventDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range chatEventRejectedEncodeCaseIDs() {
		candidate := chatEventCandidateByID(t, candidates, id)
		outcome, encoded, err := runChatEventEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runChatEventEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolChatEventOracleExpectedFieldMutationFailsComparison pins that the
// recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer decoded, and replacing the encoded
// digest fails comparison against the reviewed wire.
func TestProtocolChatEventOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := chatEventCandidates(t, root)

	decode := chatEventCandidateByID(t, candidates, chatEventAcceptedDecodeID)
	produced, _, err := runChatEventDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runChatEventDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	fields["command"] = "chop oak"
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated command compares equal to the produced outcome")
	}

	encode := chatEventCandidateByID(t, candidates, chatEventAcceptedEncodeID)
	producedEncode, _, err := runChatEventEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runChatEventEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolChatEventOracleRoutesExecuteEveryCase executes this group's
// complete case set through the shared packet-case runner, once per case, and
// compares every observation with the reviewed expectation.
func TestProtocolChatEventOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := chatEventCandidates(t, root)
	manifest := chatEventManifest(t, root, candidates)
	staged := chatEventScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, chatEventCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := chatEventObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolChatEventOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so a
// case cannot claim coverage from its name alone.
func TestProtocolChatEventOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := chatEventCandidates(t, root)
	manifest := chatEventManifest(t, root, candidates)
	staged := chatEventScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{
		{FamilyID: chatEventFamily, Version: chatEventVersion, Operation: "decode"}: runChatEventDecode,
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	if !strings.Contains(err.Error(), chatEventFamily+"/"+chatEventVersion+"/encode") {
		t.Fatalf("rejection %v does not name the family's encode route", err)
	}
}

// TestProtocolChatEventOracleManifestMergeRegistersChatEventRoutes pins that the
// merged manifest registers the family's routes and case list, leaves the source
// revision alone, and records this group's provenance sources.
func TestProtocolChatEventOracleManifestMergeRegistersChatEventRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, chatEventSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range chatEventFamilies() {
		var row *Family
		for index := range merged.Families {
			if merged.Families[index].ID == family.id {
				row = &merged.Families[index]
				break
			}
		}
		if row == nil {
			t.Fatalf("merged manifest has no %s family", family.id)
		}
		var listed []string
		for _, c := range merged.Cases {
			if c.Family == family.id {
				listed = append(listed, c.ID)
			}
		}
		sort.Strings(listed)
		if !reflect.DeepEqual(row.Cases, listed) {
			t.Fatalf("%s family lists %v, want %v", family.id, row.Cases, listed)
		}
		if len(listed) == 0 {
			t.Fatalf("%s family registers no case", family.id)
		}
		sources := make(map[string]bool, len(row.Sources))
		for _, source := range row.Sources {
			hash, hashErr := hashFile(filepath.Join(root, filepath.FromSlash(source.Path)))
			if hashErr != nil {
				t.Fatalf("hash provenance source %s: %v", source.Path, hashErr)
			}
			if hash != source.SHA256 {
				t.Fatalf("provenance source %s records %s, disk has %s", source.Path, source.SHA256, hash)
			}
			sources[source.Path] = true
		}
		for _, want := range chatEventServerSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so a
	// re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range chatEventCandidates(t, root) {
		if !chatEventRegisteredCase(base, candidate.Spec.ID) {
			wantCases++
		}
	}
	if len(merged.Cases) != wantCases {
		t.Fatalf("merged carries %d cases, want %d", len(merged.Cases), wantCases)
	}
	for index := 1; index < len(merged.Cases); index++ {
		if merged.Cases[index].ID < merged.Cases[index-1].ID {
			t.Fatalf("merged case list is not sorted by id at %d", index)
		}
	}
}

// TestProtocolChatEventOracleCandidatesExportForReview publishes the reviewed
// candidates and the manifest candidate. An unset export variable publishes
// nothing, so the tracked corpus is never written by this package.
func TestProtocolChatEventOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := chatEventCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), chatEventSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := chatEventCandidatesExport(t, root, candidates, merged)
	if strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv)) == "" {
		if published != "" {
			t.Fatalf("published %s with the export variable unset", published)
		}
		return
	}
	if published == "" {
		t.Fatal("no candidate was published")
	}
	for _, candidate := range candidates {
		for relative := range candidate.Assets {
			path := filepath.Join(published, filepath.FromSlash(relative))
			if _, statErr := os.Lstat(path); statErr != nil {
				t.Fatalf("candidate asset %s is missing: %v", relative, statErr)
			}
		}
	}
	written, err := os.ReadFile(filepath.Join(published, "contracts.json"))
	if err != nil {
		t.Fatalf("read manifest candidate: %v", err)
	}
	encoded, err := encodeInventory(merged)
	if err != nil {
		t.Fatalf("encode merged manifest: %v", err)
	}
	if !bytes.Equal(written, append(encoded, '\n')) {
		t.Fatal("manifest candidate is not the encoded merged manifest")
	}
	reloaded, err := LoadInventory(filepath.Join(published, "contracts.json"))
	if err != nil {
		t.Fatalf("reload manifest candidate: %v", err)
	}
	if err := verifyProtocolManifest(root, loadRealManifest(t, root), reloaded); err != nil {
		t.Fatalf("manifest candidate does not reconcile: %v", err)
	}
}

// TestProtocolChatEventGoEncoderPublishesEveryBranchWire pins that the reviewed
// branch wires are the Go encoder's own bytes rather than a hand-written
// approximation: every legal branch shape is encoded through the production
// encoder and compared with the reviewed literal, and the encoded payload is
// read back through the production decoder before it counts as evidence.
func TestProtocolChatEventGoEncoderPublishesEveryBranchWire(t *testing.T) {
	wireCodec, err := newChatEventCodec()
	if err != nil {
		t.Fatalf("newChatEventCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	for _, branch := range chatEventBranches() {
		reviewed := chatEventWire(branch)
		encodedID, payload, err := wireCodec.EncodeServer(protocol.StatePlay, chatEventDTO(branch))
		if err != nil {
			t.Fatalf("branch %s: EncodeServer: %v", branch.label, err)
		}
		if encodedID != 16 {
			t.Fatalf("branch %s: encoder published packet ID %d, want 16", branch.label, encodedID)
		}
		if !bytes.Equal(payload, reviewed) {
			t.Fatalf("branch %s: encoder published %x, want %x", branch.label, payload, reviewed)
		}
		decoded, err := wireCodec.DecodeServer(protocol.StatePlay, encodedID, payload)
		if err != nil {
			t.Fatalf("branch %s: encoded payload does not decode back: %v", branch.label, err)
		}
		if !reflect.DeepEqual(decoded, chatEventDTO(branch)) {
			t.Fatalf("branch %s: encoded payload decodes to %#v", branch.label, decoded)
		}
	}
}

// TestProtocolChatEventGoDecoderAppliesTheFixedPayloadCeiling pins the one
// structural boundary this family shares with the Rust consumer: the Go decoder
// answers a payload above the fixed ceiling with its fixed-maximum message
// before any field is read, which both sides classify as the capacity category.
//
// The ceiling is reachable on the decode side, because the decoder applies it
// before it parses: a valid branch payload padded beyond the bound is refused by
// the ceiling rather than by the trailing-byte rule.
func TestProtocolChatEventGoDecoderAppliesTheFixedPayloadCeiling(t *testing.T) {
	wireCodec, err := newChatEventCodec()
	if err != nil {
		t.Fatalf("newChatEventCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	reviewed := chatEventAcceptedWire()
	if len(reviewed) >= protocol.ChatEventMaxWireBytes {
		t.Fatalf("the reviewed payload is %d bytes, which is not below the ceiling", len(reviewed))
	}
	above := make([]byte, protocol.ChatEventMaxWireBytes+1)
	copy(above, reviewed)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 16, above); err == nil {
		t.Fatal("the Go decoder accepted a payload above the fixed ceiling")
	} else if !strings.Contains(err.Error(), "payload exceeds fixed maximum") {
		t.Fatalf("the Go decoder answered the over-ceiling payload with %v", err)
	}
	// A payload of exactly the ceiling length is parsed rather than refused by
	// the ceiling, so the guard is a strict comparison.
	atCeiling := make([]byte, protocol.ChatEventMaxWireBytes)
	copy(atCeiling, reviewed)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 16, atCeiling); err == nil {
		t.Fatal("the Go decoder accepted a padded payload at the ceiling length")
	} else if strings.Contains(err.Error(), "payload exceeds fixed maximum") {
		t.Fatalf("the Go decoder answered the at-ceiling payload with %v", err)
	}
}

// TestProtocolChatEventGoEncoderRefusesTheCommandBound pins that the Go server
// encoder runs the outbound validator before it writes a single byte, so a
// command above the byte bound is refused instead of silently truncated.
//
// The Rust encoder refuses the same DTO through its value gate, and both publish
// the `invalid-value` category this case records: the Go side answers from the
// combination validator's accepted-branch message and the Rust side from its own
// value gate, so neither implementation admits the record.
func TestProtocolChatEventGoEncoderRefusesTheCommandBound(t *testing.T) {
	wireCodec, err := newChatEventCodec()
	if err != nil {
		t.Fatalf("newChatEventCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	branch := chatEventBranches()[0]
	branch.command = strings.Repeat("a", 1025)
	_, _, err = wireCodec.EncodeServer(protocol.StatePlay, chatEventDTO(branch))
	if err == nil {
		t.Fatal("the Go encoder published a command above the byte bound")
	}
	// The Go server encoder runs the outbound validator before it writes a
	// single byte, so the command bound is answered by the combination
	// validator's own message rather than by the wire primitive.
	if !strings.Contains(err.Error(), "invalid accepted chat event") {
		t.Fatalf("the Go encoder refused the over-bound command with %v", err)
	}
	// The silent wire the unvalidated Rust encoder would have published before
	// this node: the same record with the command slot's own bytes.
	silent := chatEventWire(branch)
	if len(silent) <= 1025 {
		t.Fatalf("the silent wire is %d bytes", len(silent))
	}
}

// TestProtocolChatEventProducerIDIsAllowlisted pins the exporter identity this
// group publishes under.
func TestProtocolChatEventProducerIDIsAllowlisted(t *testing.T) {
	if chatEventProducerID != "runtime-oracle/protocol-chat-event" {
		t.Fatalf("producer ID = %s, want runtime-oracle/protocol-chat-event", chatEventProducerID)
	}
	if !validProducerIDs[chatEventProducerID] {
		t.Fatalf("producer ID %s is not allowlisted", chatEventProducerID)
	}
}
