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
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// This file is the Go producer for the domain identity and text family. It
// executes every committed corpus case through the current Go identity and text
// validators, so a recorded outcome is what the checked-in authority decides
// about the value rather than a copy of a Rust result or a hand-written
// expectation. The producer only reads immutable corpus inputs and calls
// in-process functions; no world authority, listener, model call or native ABI
// is involved, and the frozen corpus files it materializes are read-only inputs
// for later consumers.

const (
	// domainIdentityFamily is the corpus family this package executes. Its
	// eventual owner is the Rust domain crate, which owns the identity, text
	// and scalar value rules this family pins.
	domainIdentityFamily = "domain.identity_values"
	// domainIdentityVersion labels the family as the current value rules rather
	// than a numbered schema: the identity and text rules have no version
	// history, so a version number would imply a migration path that does not
	// exist.
	domainIdentityVersion = "current"
	// domainIdentityOperation is the manifest operation name for an identity or
	// text admission case.
	domainIdentityOperation = "admit"
	// domainIdentityConsumer is the manifest consumer the change pins for this
	// family: the Rust crate that owns these rules.
	domainIdentityConsumer = "mornlea_domain"
	// domainIdentityCorpusRelDir is the repository-relative directory holding
	// the frozen identity and text corpus cases.
	domainIdentityCorpusRelDir = "testdata/runtime-migration/cases/domain/identity_values"
	// domainIdentityProducerTestRelPath and the two producer test names locate
	// the package-local producer that executes the core validators. The topic
	// name is the entry point the domain plan's filter requires; a corpus whose
	// producer test is missing has no independently executed evidence at all.
	domainIdentityProducerTestRelPath = "packages/tools/cmd/runtime-oracle/domain_identity_values_test.go"
	domainIdentityProducerExecuteName = "TestDomainIdentityValuesOracleExecutesEveryCase"
	domainIdentityProducerTopicName   = "TestDomainOracle_identity_values"
	// domainIdentityCorpusReportName is the published report file name for the
	// executed identity and text evidence.
	domainIdentityCorpusReportName = "runtime-corpus-domain-identity-values.json"
	// domainIdentitySource is the primary provenance source of the identity and
	// display-name rules.
	domainIdentitySource = "packages/shared/core/player_id.go"
	// domainIdentityNumericSemantics records the numeric contract this family
	// pins, in the same shape the other families use.
	domainIdentityNumericSemantics = "UUIDv4 identity; canonical display and companion names; bounded command and speech text; reject zero, wrong version, wrong variant, out-of-range and untrimmed values"
)

// domainIdentityFamilySources is the merged provenance set the family records.
// Every entry is a file the producer's rules are read from, so a change to any
// of them is a change to the recorded evidence.
var domainIdentityFamilySources = []string{
	"packages/shared/core/player_id.go",
	"packages/shared/companion/identity.go",
	"packages/shared/network/protocol/message_companion.go",
}

// domainIdentityLabelPattern is the shape a corpus label must have: a lowercase
// slug with an optional zero-padded numeric suffix, so a boundary row sorts in
// numeric order under a lexical sort.
var domainIdentityLabelPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(-[0-9]+)?$`)

// updateDomainIdentityCorpus rewrites the frozen identity and text corpus from
// the executed validators. It follows the same discipline as the other fixture
// update flags: an ordinary run only compares, so a frozen artifact is never
// silently regenerated to match an implementation.
var updateDomainIdentityCorpus = flag.Bool(
	"update-domain-identity-corpus",
	false,
	"rewrite testdata/runtime-migration/cases/domain/identity_values from the executed core validators",
)

// The plan names one canonical UUIDv4 vector set for the identity boundary: a
// valid value plus the zero, wrong-version and wrong-variant rejections. These
// are case inputs, never expected outcomes; the producer's answer about each
// one comes from `core.PlayerID.Valid`. The two text forms below are the
// identity fixtures the speech-slot validator needs, because that validator is
// reached through a chat event that must carry a legal identity.
const (
	domainIdentityUUIDZero         = "00000000000000000000000000000000"
	domainIdentityUUIDValid        = "00112233445546778899aabbccddeeff"
	domainIdentityUUIDWrongVersion = "00112233445536778899aabbccddeeff"
	domainIdentityUUIDWrongVariant = "00112233445546770099aabbccddeeff"
	domainIdentityPlayerUUIDText   = "00112233-4455-4677-8899-aabbccddeeff"
	domainIdentityCompanionText    = "22334455-4667-4889-9aab-bccddeeff001"
	domainIdentitySpeechPlayerName = "Alice"
	domainIdentitySpeechCompanion  = "Buddy"
)

// domainIdentityInput is the frozen, self-describing corpus input for one case.
//
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the authority. `Name` and `Text` are pointers so an
// absent field and an explicit empty string stay distinguishable, which matters
// because the empty display name and the empty text slot are both boundary
// values the rule rejects.
type domainIdentityInput struct {
	Consumer string  `json:"consumer"`
	Rule     string  `json:"rule"`
	UUID     string  `json:"uuid,omitempty"`
	Name     *string `json:"name,omitempty"`
	Text     *string `json:"text,omitempty"`
}

// domainIdentityCase is one frozen corpus case: its label and its input, and
// nothing else.
type domainIdentityCase struct {
	label string
	input domainIdentityInput
}

// domainIdentityCases is the ordered case table the producer executes.
//
// The rows are the boundaries the identity and text rules name: the UUIDv4
// version and variant nibbles, the display-name scalar and byte limits, the
// canonical-form requirement that keeps the trim in admission, the companion
// name's embedded-whitespace rejection, and the command and speech byte bounds.
// The length bounds are read from the exported Go constants where they exist so
// a case cannot sit one byte away from a limit the authority does not use.
func domainIdentityCases() []domainIdentityCase {
	players := []domainIdentityCase{
		{
			label: "player-id-zero",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "player-id", UUID: domainIdentityUUIDZero},
		},
		{
			label: "player-id-wrong-version",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "player-id", UUID: domainIdentityUUIDWrongVersion},
		},
		{
			label: "player-id-wrong-variant",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "player-id", UUID: domainIdentityUUIDWrongVariant},
		},
		{
			label: "player-id-valid",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "player-id", UUID: domainIdentityUUIDValid},
		},
	}

	display := []domainIdentityCase{
		{
			label: "display-name-canonical",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "display-name", Name: domainIdentityName("Alice")},
		},
		{
			label: "display-name-interior-space",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "display-name", Name: domainIdentityName("Alice B")},
		},
		{
			label: "display-name-zero-width-space",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "display-name", Name: domainIdentityName("Ali\u200Bce")},
		},
		{
			label: "display-name-32-scalars",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "display-name", Name: domainIdentityName(strings.Repeat("a", 32))},
		},
		{
			label: "display-name-33-scalars",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "display-name", Name: domainIdentityName(strings.Repeat("a", 33))},
		},
		{
			label: "display-name-128-bytes",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "display-name", Name: domainIdentityName(strings.Repeat("\U0001F600", 32))},
		},
		{
			label: "display-name-129-bytes",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "display-name", Name: domainIdentityName(strings.Repeat("\U0001F600", 32) + "a")},
		},
		{
			label: "display-name-empty",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "display-name", Name: domainIdentityName("")},
		},
		{
			label: "display-name-leading-space",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "display-name", Name: domainIdentityName(" Alice")},
		},
		{
			label: "display-name-trailing-space",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "display-name", Name: domainIdentityName("Alice ")},
		},
		{
			label: "display-name-trailing-nel",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "display-name", Name: domainIdentityName("Alice\u0085")},
		},
		{
			label: "display-name-embedded-del",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "display-name", Name: domainIdentityName("Ali\u007Fce")},
		},
	}

	companions := []domainIdentityCase{
		{
			label: "companion-name-canonical",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "companion-name", Name: domainIdentityName(domainIdentitySpeechCompanion)},
		},
		{
			label: "companion-name-interior-space",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "companion-name", Name: domainIdentityName("A B")},
		},
		{
			label: "companion-name-leading-space",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "companion-name", Name: domainIdentityName(" Buddy")},
		},
		{
			label: "companion-name-trailing-nbsp",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "companion-name", Name: domainIdentityName("Buddy\u00A0")},
		},
		{
			label: "companion-name-empty",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "companion-name", Name: domainIdentityName("")},
		},
	}

	commands := []domainIdentityCase{
		{
			label: "command-text-1024-bytes",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "command-text", Text: domainIdentityText(strings.Repeat("x", protocol.ChatCommandTextMaxBytes))},
		},
		{
			label: "command-text-1025-bytes",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "command-text", Text: domainIdentityText(strings.Repeat("x", protocol.ChatCommandTextMaxBytes+1))},
		},
		{
			label: "command-text-empty",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "command-text", Text: domainIdentityText("")},
		},
		{
			label: "command-text-untrimmed",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "command-text", Text: domainIdentityText(" mine ")},
		},
		{
			label: "command-text-control",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "command-text", Text: domainIdentityText("mi\u0001ne")},
		},
	}

	speeches := []domainIdentityCase{
		{
			label: "speech-text-256-bytes",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "speech-text", Text: domainIdentityText(strings.Repeat("y", protocol.ChatSpeechTextMaxBytes))},
		},
		{
			label: "speech-text-257-bytes",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "speech-text", Text: domainIdentityText(strings.Repeat("y", protocol.ChatSpeechTextMaxBytes+1))},
		},
		{
			label: "speech-text-empty",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "speech-text", Text: domainIdentityText("")},
		},
		{
			label: "speech-text-untrimmed",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "speech-text", Text: domainIdentityText(" hello ")},
		},
		{
			label: "speech-text-control",
			input: domainIdentityInput{Consumer: domainIdentityConsumer, Rule: "speech-text", Text: domainIdentityText("hi\u009Fthere")},
		},
	}

	cases := make([]domainIdentityCase, 0, len(players)+len(display)+len(companions)+len(commands)+len(speeches))
	cases = append(cases, players...)
	cases = append(cases, display...)
	cases = append(cases, companions...)
	cases = append(cases, commands...)
	cases = append(cases, speeches...)
	return cases
}

// domainIdentityName renders one name value for a case input.
func domainIdentityName(name string) *string {
	return &name
}

// domainIdentityText renders one text value for a case input.
func domainIdentityText(text string) *string {
	return &text
}

// runDomainIdentityValues executes one corpus case through the current Go
// identity and text validators.
//
// The verdict always comes from the authority itself: `core.PlayerID.Valid`,
// the canonical display-name admission built on `core.NormalizeDisplayName`,
// `companion.ValidateName`, or the protocol text validators. The rule name and
// the rejection category come from the same tables and bounds the authority
// reads, and the two are cross-checked against each other, so a classification
// that disagrees with the authority fails the run instead of publishing a
// plausible-looking rejection.
func runDomainIdentityValues(c CaseSpec, input []byte) (Outcome, []byte, error) {
	spec, err := domainIdentityDecodeInput(input)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: decode input: %w", c.ID, err)
	}
	if spec.Consumer != domainIdentityConsumer {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names consumer %q, want %q", c.ID, spec.Consumer, domainIdentityConsumer)
	}

	switch spec.Rule {
	case "player-id":
		return domainIdentityRunPlayerID(c, spec)
	case "display-name":
		return domainIdentityRunDisplayName(c, spec)
	case "companion-name":
		return domainIdentityRunCompanionName(c, spec)
	case "command-text":
		return domainIdentityRunCommandText(c, spec)
	case "speech-text":
		return domainIdentityRunSpeechText(c, spec)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown rule %q", c.ID, spec.Rule)
	}
}

// domainIdentityRunPlayerID admits one raw identity through
// `core.PlayerID.Valid`. The companion identity is validated by the same Go
// rule, so this one producer covers both identity types.
func domainIdentityRunPlayerID(c CaseSpec, spec domainIdentityInput) (Outcome, []byte, error) {
	id, err := domainIdentityPlayerID(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}

	fields := map[string]any{"uuid": hex.EncodeToString(id[:])}
	category, rule := domainIdentityPlayerIDRule(id)
	if id.Valid() == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with core.PlayerID.Valid=%v", c.ID, rule, id.Valid())
	}
	if id.Valid() {
		return Outcome{Kind: "ok", Category: "player-id", Fields: fields}, nil, nil
	}
	fields["rule"] = rule
	return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
}

// domainIdentityRunDisplayName admits one display name the way the domain
// canonical constructor will.
//
// The plan pins that display-name normalization is not implicit in the
// canonical constructor: admission trims before it. The Go authority for the
// trimmed form is `core.NormalizeDisplayName`; the canonical-form requirement
// is that the admitted value already equals its normalized form, which is the
// same rule the protocol login path applies to a wire name. The producer never
// normalizes a value on the authority's behalf, so an untrimmed name is
// evidence of a rejection rather than something quietly repaired.
func domainIdentityRunDisplayName(c CaseSpec, spec domainIdentityInput) (Outcome, []byte, error) {
	if spec.Name == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: display-name requires a name", c.ID)
	}
	name := *spec.Name

	_, admitted := domainIdentityCanonicalDisplayName(name)
	fields := map[string]any{"name": name}
	category, rule := domainIdentityDisplayNameRule(name)
	if admitted == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with the canonical display-name admission (admitted=%v)", c.ID, rule, admitted)
	}
	if admitted {
		return Outcome{Kind: "ok", Category: "display-name", Fields: fields}, nil, nil
	}
	fields["rule"] = rule
	return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
}

// domainIdentityRunCompanionName admits one companion name through
// `companion.ValidateName`. The companion rule is the display-name rule plus a
// rejection of any Unicode whitespace, so an interior space that a display name
// keeps is evidence here.
func domainIdentityRunCompanionName(c CaseSpec, spec domainIdentityInput) (Outcome, []byte, error) {
	if spec.Name == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: companion-name requires a name", c.ID)
	}
	name := *spec.Name

	fields := map[string]any{"name": name}
	category, rule := domainIdentityCompanionNameRule(name)
	admitted := companion.ValidateName(name) == nil
	if admitted == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with companion.ValidateName (admitted=%v)", c.ID, rule, admitted)
	}
	if admitted {
		return Outcome{Kind: "ok", Category: "companion-name", Fields: fields}, nil, nil
	}
	fields["rule"] = rule
	return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
}

// domainIdentityRunCommandText admits one chat command through the protocol
// command validator, whose byte bound is the planner instruction bound the Rust
// text rule pins.
func domainIdentityRunCommandText(c CaseSpec, spec domainIdentityInput) (Outcome, []byte, error) {
	if spec.Text == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: command-text requires text", c.ID)
	}
	text := *spec.Text

	fields := map[string]any{"text": text}
	category, rule := domainIdentityBoundedTextRule(text, protocol.ChatCommandTextMaxBytes, "command_text")
	admitted := protocol.ChatCommand{Text: text}.Validate() == nil
	if admitted == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with the chat command validator (admitted=%v)", c.ID, rule, admitted)
	}
	if admitted {
		return Outcome{Kind: "ok", Category: "command-text", Fields: fields}, nil, nil
	}
	fields["rule"] = rule
	return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
}

// domainIdentityRunSpeechText admits one companion speech line through the
// protocol speech validator.
//
// The speech slot has no standalone exported validator: the bound lives in
// `protocol.ChatSpeechTextMaxBytes` and the rule is applied by `ChatEvent`
// when the kind is `ChatEventCompanionSpeech`. The producer therefore drives
// the real event validator with one legal identity held constant and varies
// only the speech text, so the verdict is the speech rule's answer about that
// text and nothing else.
func domainIdentityRunSpeechText(c CaseSpec, spec domainIdentityInput) (Outcome, []byte, error) {
	if spec.Text == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: speech-text requires text", c.ID)
	}
	text := *spec.Text

	event, err := domainIdentitySpeechEvent(text)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: %w", c.ID, err)
	}
	fields := map[string]any{"text": text}
	category, rule := domainIdentityBoundedTextRule(text, protocol.ChatSpeechTextMaxBytes, "speech_text")
	admitted := event.Validate() == nil
	if admitted == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with the companion speech validator (admitted=%v)", c.ID, rule, admitted)
	}
	if admitted {
		return Outcome{Kind: "ok", Category: "speech-text", Fields: fields}, nil, nil
	}
	fields["rule"] = rule
	return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
}

// domainIdentityPlayerIDRule names the first rule a raw identity breaks, read
// from the same version and variant nibbles `core.PlayerID.Valid` reads. An
// empty result means the identity is admitted.
func domainIdentityPlayerIDRule(id core.PlayerID) (string, string) {
	if id == (core.PlayerID{}) {
		return "invalid-identity", "player_id.nonzero"
	}
	if id[6]>>4 != 4 {
		return "invalid-identity", "player_id.version_v4"
	}
	if id[8]&0xc0 != 0x80 {
		return "invalid-identity", "player_id.variant_rfc4122"
	}
	return "", ""
}

// domainIdentityDisplayNameRule names the first rule a display name breaks,
// read from the same trim, length and control checks
// `core.NormalizeDisplayName` applies, plus the canonical-form requirement.
//
// The scalar and byte limits are restated rather than read from constants
// because the Go rule keeps them as literals; the boundary rows in the case
// table are what makes a drift detectable, because a limit that moved turns one
// of those rows into a disagreement with the authority.
func domainIdentityDisplayNameRule(name string) (string, string) {
	if !utf8.ValidString(name) {
		return "invalid-value", "display_name.utf8"
	}
	trimmed := strings.TrimSpace(name)
	if utf8.RuneCountInString(trimmed) < 1 || utf8.RuneCountInString(trimmed) > 32 || len(trimmed) > 128 {
		return "invalid-value", "display_name.length_range"
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return "invalid-value", "display_name.control"
		}
	}
	if trimmed != name {
		return "invalid-value", "display_name.canonical_trim"
	}
	return "", ""
}

// domainIdentityCompanionNameRule names the first rule a companion name breaks:
// the display-name rule first, then the embedded-whitespace rejection that
// makes a companion name a tighter rule than a display name.
func domainIdentityCompanionNameRule(name string) (string, string) {
	if category, rule := domainIdentityDisplayNameRule(name); rule != "" {
		return category, rule
	}
	for _, r := range name {
		if unicode.IsSpace(r) {
			return "invalid-value", "companion_name.embedded_whitespace"
		}
	}
	return "", ""
}

// domainIdentityBoundedTextRule names the first rule one bounded text slot
// breaks, read from the same exported byte bound and the same trim, UTF-8 and
// control checks the protocol validators apply. The bound comes from the
// caller, so the command and speech slots cannot drift apart.
func domainIdentityBoundedTextRule(text string, limit int, subject string) (string, string) {
	if len(text) < 1 || len(text) > limit {
		return "invalid-value", subject + ".byte_range"
	}
	if !utf8.ValidString(text) {
		return "invalid-value", subject + ".utf8"
	}
	if strings.TrimSpace(text) != text {
		return "invalid-value", subject + ".untrimmed"
	}
	for _, r := range text {
		if r == 0 || unicode.IsControl(r) {
			return "invalid-value", subject + ".control"
		}
	}
	return "", ""
}

// domainIdentityCanonicalDisplayName reports whether one display name is
// already canonical: `core.NormalizeDisplayName` accepts it and its normalized
// form is the value itself. The normalized form is returned for callers that
// need to show what admission would have produced.
func domainIdentityCanonicalDisplayName(name string) (string, bool) {
	canonical, err := core.NormalizeDisplayName(name)
	if err != nil {
		return "", false
	}
	return canonical, canonical == name
}

// domainIdentitySpeechEvent assembles the chat event that reaches the speech
// validator. The identity fields are the plan's canonical vectors and the
// names are legal, so only the speech text can decide the verdict.
func domainIdentitySpeechEvent(text string) (protocol.ChatEvent, error) {
	player, err := core.ParsePlayerID(domainIdentityPlayerUUIDText)
	if err != nil {
		return protocol.ChatEvent{}, fmt.Errorf("speech fixture player identity: %w", err)
	}
	companionID, err := companion.ParseID(domainIdentityCompanionText)
	if err != nil {
		return protocol.ChatEvent{}, fmt.Errorf("speech fixture companion identity: %w", err)
	}
	return protocol.ChatEvent{
		EventID:       1,
		PlayerID:      player,
		PlayerName:    domainIdentitySpeechPlayerName,
		CompanionID:   companionID,
		CompanionName: domainIdentitySpeechCompanion,
		Kind:          protocol.ChatEventCompanionSpeech,
		RejectReason:  protocol.ChatRejectNone,
		Speech:        text,
	}, nil
}

// domainIdentityPlayerID resolves the identity field one case names. The value
// is the 16 raw bytes in lowercase hex, so a wrong version or variant nibble is
// expressible as an input instead of being unreachable through a text parser
// that would reject it first.
func domainIdentityPlayerID(c CaseSpec, spec domainIdentityInput) (core.PlayerID, error) {
	if len(spec.UUID) != 32 {
		return core.PlayerID{}, fmt.Errorf("runtime-oracle: case %s names identity %q, which is not 32 hex digits", c.ID, spec.UUID)
	}
	var id core.PlayerID
	decoded, err := hex.Decode(id[:], []byte(spec.UUID))
	if err != nil || decoded != len(id) {
		return core.PlayerID{}, fmt.Errorf("runtime-oracle: case %s names identity %q, which is not hexadecimal", c.ID, spec.UUID)
	}
	if strings.ToLower(spec.UUID) != spec.UUID {
		return core.PlayerID{}, fmt.Errorf("runtime-oracle: case %s names identity %q, which is not lowercase", c.ID, spec.UUID)
	}
	return id, nil
}

// domainIdentityDecodeInput reads one frozen corpus input and rejects trailing
// content, so a producer never executes bytes the case did not name.
func domainIdentityDecodeInput(data []byte) (domainIdentityInput, error) {
	var spec domainIdentityInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&spec); err != nil {
		return domainIdentityInput{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainIdentityInput{}, fmt.Errorf("trailing content after the JSON value")
	}
	return spec, nil
}

// domainIdentityRecord pairs one executed case with the input and outcome the
// producer produced for it.
type domainIdentityRecord struct {
	label   string
	input   domainIdentityInput
	outcome Outcome
}

// domainIdentityExecute runs the whole case table through the producer and
// returns one record per case in table order.
func domainIdentityExecute(t *testing.T) []domainIdentityRecord {
	t.Helper()

	cases := domainIdentityCases()
	records := make([]domainIdentityRecord, 0, len(cases))
	for _, entry := range cases {
		input, err := json.MarshalIndent(entry.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", entry.label, err)
		}
		input = append(input, '\n')
		outcome, _, err := runDomainIdentityValues(CaseSpec{ID: domainIdentityCaseID(entry.label)}, input)
		if err != nil {
			t.Fatalf("execute case %s: %v", entry.label, err)
		}
		records = append(records, domainIdentityRecord{label: entry.label, input: entry.input, outcome: outcome})
	}
	return records
}

// domainIdentitySyncCorpus writes or verifies the frozen corpus files.
//
// An ordinary run is read-only: it proves the committed files still match what
// the validators produce now, so a drifted artifact fails instead of being
// regenerated. The explicit update flag rewrites them, which is the only way a
// frozen artifact changes.
func domainIdentitySyncCorpus(t *testing.T, records []domainIdentityRecord) {
	t.Helper()

	root := mustRepoRoot(t)
	corpusDir := filepath.Join(root, filepath.FromSlash(domainIdentityCorpusRelDir))
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

	if *updateDomainIdentityCorpus {
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
			t.Fatalf("read frozen corpus case %s: %v (rerun with -update-domain-identity-corpus after reviewing the change)", relative, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("frozen corpus case %s drifted from the executed validator (rerun with -update-domain-identity-corpus after reviewing the change)", relative)
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

// domainIdentityCaseID renders the manifest case identity one corpus label
// carries.
func domainIdentityCaseID(label string) string {
	return domainIdentityFamily + "/" + domainIdentityVersion + "/" + label
}

// domainIdentityWorkingManifest assembles the manifest this node executes
// inside a harness-owned temporary directory.
//
// The frozen manifest does not carry this family, because the controller merges
// manifest fragments after acceptance. The working manifest is therefore a
// family-scoped selection: `Cases` holds exactly the `domainIdentityFamily`
// cases the committed corpus registers, every other family's case list is
// cleared because `Reconcile` requires each family's list to match the cases
// this selection registers for it, and the family itself is appended with the
// provenance the producer reads. Registering the family in the live registry is
// deliberately left to the manifest merge: adding it here would make the frozen
// corpus fail reconciliation as an uncovered family before the fragment lands.
func domainIdentityWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	cases := domainIdentityCorpusCases(t, root)

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
	sources := make([]SourceSpec, 0, len(domainIdentityFamilySources))
	for _, relative := range domainIdentityFamilySources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", relative, err)
		}
		sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
	}
	cloned.Families = append(cloned.Families, Family{
		ID:                domainIdentityFamily,
		Kind:              "domain",
		Role:              "input",
		CurrentVersion:    domainIdentityVersion,
		SupportedVersions: []string{domainIdentityVersion},
		Source:            domainIdentitySource,
		EventualOwner:     ownerDomain,
		NumericSemantics:  domainIdentityNumericSemantics,
		Sources:           sources,
		Cases:             caseIDs,
	})
	for index := range cloned.Families {
		// A family this selection does not execute registers no case, so its
		// list must be empty for the manifest to describe itself.
		if cloned.Families[index].ID != domainIdentityFamily {
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

// domainIdentityCorpusCases reads the frozen corpus and registers one case per
// committed input, with digests proven against the files on disk.
func domainIdentityCorpusCases(t *testing.T, root string) []CaseSpec {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(domainIdentityCorpusRelDir))
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
		t.Fatalf("walk identity values corpus directory: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no identity value case under %s", domainIdentityCorpusRelDir)
	}
	sort.Strings(inputs)

	cases := make([]CaseSpec, 0, len(inputs))
	for _, input := range inputs {
		relative, relErr := filepath.Rel(dir, input)
		if relErr != nil {
			t.Fatalf("relative corpus path: %v", relErr)
		}
		label := strings.TrimSuffix(filepath.ToSlash(relative), ".input.json")
		if label == "" || !domainIdentityLabelPattern.MatchString(label) {
			t.Fatalf("corpus case %s has label %q, which is not a lowercase slug with an optional numeric suffix", filepath.ToSlash(relative), label)
		}
		envelope := domainIdentityReadEnvelope(t, input)
		if envelope.Consumer != domainIdentityConsumer {
			t.Fatalf("corpus case %s names consumer %q, want %q", label, envelope.Consumer, domainIdentityConsumer)
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
			ID:           domainIdentityCaseID(label),
			Family:       domainIdentityFamily,
			Version:      domainIdentityVersion,
			Operation:    domainIdentityOperation,
			Input:        AssetRef{Path: domainIdentityCorpusPath(root, input), SHA256: inputHash},
			InputFormat:  "json",
			Expected:     AssetRef{Path: domainIdentityCorpusPath(root, expectedPath), SHA256: expectedHash},
			Checkpoints:  []string{"0"},
			RustConsumer: domainIdentityConsumer,
		})
	}
	return cases
}

// domainIdentityReadEnvelope reads the provenance envelope of one frozen corpus
// input.
func domainIdentityReadEnvelope(t *testing.T, path string) domainIdentityInput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var envelope domainIdentityInput
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return envelope
}

// domainIdentityCorpusPath renders one absolute corpus path as the
// repository-relative slash path the manifest requires.
func domainIdentityCorpusPath(root, absolute string) string {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return absolute
	}
	return filepath.ToSlash(relative)
}

// TestDomainIdentityValuesOracleExecutesEveryCase runs the whole case table
// through the real Go validators, proves the frozen corpus still matches what
// they produce, and then runs the same cases through the production runner so
// the executed evidence satisfies the completeness rules a published trace
// report does.
func TestDomainIdentityValuesOracleExecutesEveryCase(t *testing.T) {
	records := domainIdentityExecute(t)
	domainIdentitySyncCorpus(t, records)

	root := mustRepoRoot(t)
	manifest := domainIdentityWorkingManifest(t, root)
	if len(manifest.Cases) != len(records) {
		t.Fatalf("working manifest registers %d cases, want %d (one per executed table row)", len(manifest.Cases), len(records))
	}
	domainIdentityAssertCaseSpecs(t, root, manifest)
	domainIdentityAssertOutcomesDistinguishCases(t, records)

	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(observations) != len(manifest.Cases) {
		t.Fatalf("produced %d observations, want %d", len(observations), len(manifest.Cases))
	}
	for _, obs := range observations {
		c := domainIdentityCaseByID(t, manifest, obs.CaseID)
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
		c := domainIdentityCaseByID(t, manifest, obs.CaseID)
		if obs.ExpectedDigest != c.Expected.SHA256 {
			t.Fatalf("observation for %s carries expected digest %s, want %s", obs.CaseID, obs.ExpectedDigest, c.Expected.SHA256)
		}
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed identity value evidence failed trace validation: %v", err)
	}
}

// TestDomainIdentityValuesOracleCaseIdentitiesAreTheRustDomainConsumer pins the
// manifest identity of every identity value case: the operation, the version,
// the family and the consumer the change names for the Rust crate that owns
// these rules.
func TestDomainIdentityValuesOracleCaseIdentitiesAreTheRustDomainConsumer(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainIdentityWorkingManifest(t, root)

	labels := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Operation != domainIdentityOperation {
			t.Fatalf("case %s declares operation %q, want %q", c.ID, c.Operation, domainIdentityOperation)
		}
		if c.Version != domainIdentityVersion {
			t.Fatalf("case %s declares version %q, want %q", c.ID, c.Version, domainIdentityVersion)
		}
		if c.RustConsumer != domainIdentityConsumer {
			t.Fatalf("case %s declares consumer %q, want %q", c.ID, c.RustConsumer, domainIdentityConsumer)
		}
		if c.InputFormat != "json" {
			t.Fatalf("case %s declares input_format %q, want json", c.ID, c.InputFormat)
		}
		prefix := c.Family + "/" + c.Version + "/"
		if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
			t.Fatalf("case %s does not match %s<label>", c.ID, prefix)
		}
		label := strings.TrimPrefix(c.ID, prefix)
		if !domainIdentityLabelPattern.MatchString(label) {
			t.Fatalf("case %s label %q is not a lowercase slug with an optional numeric suffix", c.ID, label)
		}
		if labels[label] {
			t.Fatalf("label %q is registered twice", label)
		}
		labels[label] = true

		envelope := domainIdentityReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		if envelope.Consumer != domainIdentityConsumer {
			t.Fatalf("case %s was produced for consumer %q, want %q", c.ID, envelope.Consumer, domainIdentityConsumer)
		}
	}
}

// TestDomainIdentityValuesOracleWorkingManifestDescribesItself proves the
// working manifest is internally consistent without claiming a registry entry
// that the frozen corpus does not carry yet: every case passes the production
// case validation, the family's provenance hashes match disk, and the family's
// case list matches exactly the cases the selection registers.
func TestDomainIdentityValuesOracleWorkingManifestDescribesItself(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainIdentityWorkingManifest(t, root)
	domainIdentityAssertCaseSpecs(t, root, manifest)

	family, ok := domainIdentityFamilySpec(manifest)
	if !ok {
		t.Fatalf("working manifest has no %s family", domainIdentityFamily)
	}
	if len(family.Sources) != len(domainIdentityFamilySources) {
		t.Fatalf("%s records %d provenance sources, want %d", domainIdentityFamily, len(family.Sources), len(domainIdentityFamilySources))
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
		t.Fatalf("%s lists %d cases, want %d", domainIdentityFamily, len(family.Cases), len(registered))
	}
	for _, id := range family.Cases {
		if !registered[id] {
			t.Fatalf("%s lists case %s which the selection does not register", domainIdentityFamily, id)
		}
	}
}

// TestDomainIdentityValuesOracleOutcomesDistinguishAcceptedAndRejected pins that
// the producer is not returning one constant answer, that every rule records
// both an accepted value and a rejection, and that the boundaries the Rust
// identity test enumerates are each present in the executed evidence.
func TestDomainIdentityValuesOracleOutcomesDistinguishAcceptedAndRejected(t *testing.T) {
	records := domainIdentityExecute(t)
	domainIdentityAssertOutcomesDistinguishCases(t, records)

	tally := make(map[string]*domainIdentityRuleTally)
	for _, record := range records {
		entry, ok := tally[record.input.Rule]
		if !ok {
			entry = &domainIdentityRuleTally{}
			tally[record.input.Rule] = entry
		}
		switch record.outcome.Kind {
		case "ok":
			entry.accepted = append(entry.accepted, domainIdentityRecordValue(record))
		case "error":
			entry.rejected = append(entry.rejected, domainIdentityRecordValue(record))
		}
	}
	for rule, entry := range tally {
		if len(entry.accepted) == 0 || len(entry.rejected) == 0 {
			t.Fatalf("rule %s records %d accepted and %d rejected cases, want both non-zero",
				rule, len(entry.accepted), len(entry.rejected))
		}
	}
	if !domainIdentityValuesContain(tally["display-name"].accepted, " ") {
		t.Fatal("no accepted display name keeps an interior space, which the display rule must allow")
	}
	if !domainIdentityValuesContain(tally["companion-name"].rejected, " ") {
		t.Fatal("no rejected companion name carries an interior space, which the companion rule must reject")
	}
	if !domainIdentityValuesContain(tally["display-name"].accepted, "\u200B") {
		t.Fatal("no accepted display name keeps U+200B, which is neither whitespace nor control")
	}
	if !domainIdentityValuesContain(tally["display-name"].rejected, "\u0085") {
		t.Fatal("no rejected display name carries U+0085, which the Go trim treats as whitespace")
	}
}

// TestDomainIdentityValuesOracleRunnerRejectsUnregisteredFamily pins that a
// manifest naming a family this package cannot execute fails the run.
func TestDomainIdentityValuesOracleRunnerRejectsUnregisteredFamily(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainIdentityWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Family = "domain.unknown"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "family domain.unknown has no registered Go producer") {
		t.Fatalf("expected an unregistered-family failure, got: %v", err)
	}
}

// TestDomainIdentityValuesOracleRunnerRejectsOperationFamilyMismatch pins that a
// case cannot declare one operation and be executed by another.
func TestDomainIdentityValuesOracleRunnerRejectsOperationFamilyMismatch(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainIdentityWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Operation = "decode"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "declares operation") {
		t.Fatalf("expected an operation/family mismatch failure, got: %v", err)
	}
}

// TestDomainIdentityValuesOracleRunnerRejectsTamperedInput pins that a producer
// never executes bytes the manifest does not name.
func TestDomainIdentityValuesOracleRunnerRejectsTamperedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainIdentityWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// TestDomainIdentityValuesOracleReportPublishesAndValidates assembles the
// executed evidence into a report, publishes it through the production atomic
// exporter, reloads it and validates it against the working manifest. This is
// the identity and content check a later Rust acceptance step performs; it does
// not claim any Rust behaviour.
func TestDomainIdentityValuesOracleReportPublishesAndValidates(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainIdentityWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed identity value evidence failed trace validation: %v", err)
	}

	workspace, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	defer cleanup()
	target := filepath.Join(workspace, domainIdentityCorpusReportName)
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

// TestDomainIdentityValuesOracleRejectsMissingProducerTest pins that the
// executed evidence has a real producer behind it, including the topic-named
// entry point the domain plan's filter selects. A corpus whose producer test is
// gone has no independent execution, only frozen files, and a missing topic
// entry point would make the plan filter pass without running anything.
func TestDomainIdentityValuesOracleRejectsMissingProducerTest(t *testing.T) {
	root := mustRepoRoot(t)
	path := filepath.Join(root, filepath.FromSlash(domainIdentityProducerTestRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read producer test %s: %v", domainIdentityProducerTestRelPath, err)
	}
	for _, name := range []string{domainIdentityProducerExecuteName, domainIdentityProducerTopicName} {
		declaration := "func " + name + "(t *testing.T) {"
		if !strings.Contains(string(data), declaration) {
			t.Fatalf("%s does not declare %s", domainIdentityProducerTestRelPath, declaration)
		}
	}
}

// TestDomainOracle_identity_values is the topic-named entry point the domain
// plan names for this node. It delegates to the same executed table, so the two
// filters select one source of expected results rather than two.
func TestDomainOracle_identity_values(t *testing.T) {
	TestDomainIdentityValuesOracleExecutesEveryCase(t)
}

// domainIdentityRuleTally collects the accepted and rejected values one rule
// records, so a boundary assertion can look for the value shape it names.
type domainIdentityRuleTally struct {
	accepted []string
	rejected []string
}

// domainIdentityRecordValue renders the value one record put under test: the
// identity, the name or the text.
func domainIdentityRecordValue(record domainIdentityRecord) string {
	switch {
	case record.input.UUID != "":
		return record.input.UUID
	case record.input.Name != nil:
		return *record.input.Name
	case record.input.Text != nil:
		return *record.input.Text
	default:
		return ""
	}
}

// domainIdentityValuesContain reports whether one recorded value set holds a
// value carrying the substring a boundary assertion names.
func domainIdentityValuesContain(values []string, substring string) bool {
	for _, value := range values {
		if strings.Contains(value, substring) {
			return true
		}
	}
	return false
}

// domainIdentityAssertCaseSpecs runs the production case validation over the
// whole selection, so a case with a bad path, digest, format or checkpoint
// fails here rather than surfacing later as a confusing coverage failure.
func domainIdentityAssertCaseSpecs(t *testing.T, root string, manifest Inventory) {
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

// domainIdentityAssertOutcomesDistinguishCases proves the executed evidence
// separates accepted values from rejections instead of publishing one constant
// answer, and that the rejections name a rule the authority publishes.
func domainIdentityAssertOutcomesDistinguishCases(t *testing.T, records []domainIdentityRecord) {
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

// domainIdentityFamilySpec resolves one family from a manifest selection.
func domainIdentityFamilySpec(manifest Inventory) (Family, bool) {
	for _, family := range manifest.Families {
		if family.ID == domainIdentityFamily {
			return family, true
		}
	}
	return Family{}, false
}

// domainIdentityCaseByID indexes a manifest selection by case identity.
func domainIdentityCaseByID(t *testing.T, manifest Inventory, id string) CaseSpec {
	t.Helper()
	for _, c := range manifest.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %s is missing from the working manifest", id)
	return CaseSpec{}
}
