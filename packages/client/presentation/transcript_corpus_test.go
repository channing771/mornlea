package presentation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/network"
)

const (
	transcriptCorpusSchemaVersion = 1
	maxTranscriptFiles            = 16
	maxTranscriptFileBytes        = 256 << 10
	maxTranscriptFrames           = 64
	maxTranscriptPayloadBytes     = network.MaxCompressedSnapshot + 8
)

type transcriptCorpus struct {
	SchemaVersion int               `json:"schema_version"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	SummarySHA256 string            `json:"summary_sha256"`
	Frames        []transcriptFrame `json:"frames"`
}

type transcriptFrame struct {
	Label      string                     `json:"label"`
	Direction  string                     `json:"direction"`
	State      string                     `json:"state"`
	PacketID   uint32                     `json:"packet_id"`
	PayloadHex string                     `json:"payload_hex"`
	Expect     transcriptFrameExpectation `json:"expect"`
}

type transcriptFrameExpectation struct {
	Result        string `json:"result"`
	PacketType    string `json:"packet_type,omitempty"`
	Summary       string `json:"summary,omitempty"`
	RejectBy      string `json:"reject_by,omitempty"`
	ErrorContains string `json:"error_contains,omitempty"`
}

func TestTranscriptCorpus(t *testing.T) {
	directory := filepath.Join("..", "..", "..", "testdata", "godot-pilot", "transcripts")
	paths, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	wantFiles := []string{
		"invalid-enum.json",
		"overflow.json",
		"truncated.json",
		"valid-session.json",
		"version-error.json",
	}
	if len(paths) != len(wantFiles) || len(paths) > maxTranscriptFiles {
		t.Fatalf("transcript files=%v, want exactly %v within limit %d", baseNames(paths), wantFiles, maxTranscriptFiles)
	}
	for index, path := range paths {
		if filepath.Base(path) != wantFiles[index] {
			t.Fatalf("transcript file %d=%q, want %q", index, filepath.Base(path), wantFiles[index])
		}
	}

	codec, err := network.NewCodec()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := codec.Close(); err != nil {
			t.Errorf("close codec: %v", err)
		}
	})

	coveredLabels := make(map[string]struct{})
	for _, path := range paths {
		path := path
		t.Run(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(data) > maxTranscriptFileBytes {
				t.Fatalf("file has %d bytes, limit %d", len(data), maxTranscriptFileBytes)
			}
			corpus, err := decodeTranscriptCorpus(data)
			if err != nil {
				t.Fatal(err)
			}
			if corpus.SchemaVersion != transcriptCorpusSchemaVersion {
				t.Fatalf("schema_version=%d, want %d", corpus.SchemaVersion, transcriptCorpusSchemaVersion)
			}
			if corpus.Name != strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) {
				t.Fatalf("name=%q does not match filename", corpus.Name)
			}
			if corpus.Description == "" {
				t.Fatal("description is empty")
			}
			if len(corpus.Frames) == 0 || len(corpus.Frames) > maxTranscriptFrames {
				t.Fatalf("frames=%d, want 1..%d", len(corpus.Frames), maxTranscriptFrames)
			}

			labels := make(map[string]struct{}, len(corpus.Frames))
			summaryLines := make([]string, 0, len(corpus.Frames))
			totalPayloadBytes := 0
			for frameIndex, frame := range corpus.Frames {
				if frame.Label == "" {
					t.Fatalf("frame %d has empty label", frameIndex)
				}
				if _, duplicate := labels[frame.Label]; duplicate {
					t.Fatalf("duplicate frame label %q", frame.Label)
				}
				labels[frame.Label] = struct{}{}
				coveredLabels[frame.Label] = struct{}{}
				payload, err := decodeCanonicalHex(frame.PayloadHex)
				if err != nil {
					t.Fatalf("frame %q payload: %v", frame.Label, err)
				}
				if len(payload) > maxTranscriptPayloadBytes {
					t.Fatalf("frame %q payload has %d bytes, limit %d", frame.Label, len(payload), maxTranscriptPayloadBytes)
				}
				totalPayloadBytes += len(payload)
				if totalPayloadBytes > maxTranscriptFileBytes {
					t.Fatalf("decoded payload total exceeds %d bytes", maxTranscriptFileBytes)
				}
				state, err := transcriptState(frame.State)
				if err != nil {
					t.Fatalf("frame %q: %v", frame.Label, err)
				}
				line := verifyTranscriptFrame(t, codec, frame, state, payload)
				summaryLines = append(summaryLines, line)
			}
			gotDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(summaryLines, "\n"))))
			if corpus.SummarySHA256 != gotDigest {
				t.Fatalf("summary_sha256=%q, want %q for:\n%s", corpus.SummarySHA256, gotDigest, strings.Join(summaryLines, "\n"))
			}
		})
	}
	for _, required := range []string{
		"client-hello",
		"server-hello",
		"login-start",
		"login-success",
		"initial-snapshot",
		"movement-input",
		"authoritative-correction-weather-hud",
		"chunk-update",
		"remote-player-spawn",
		"remote-player-state",
		"remote-player-despawn",
		"disconnect",
		"unsupported-client-version",
		"truncated-login-success",
		"invalid-player-weather",
		"remote-player-count-overflow",
	} {
		if _, ok := coveredLabels[required]; !ok {
			t.Errorf("required transcript frame %q is missing", required)
		}
	}
}

func TestTranscriptCorpusSchemaRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	valid := []byte(`{"schema_version":1,"name":"strict","description":"strict fixture","summary_sha256":"digest","frames":[]}`)
	for _, test := range []struct {
		name string
		data []byte
	}{
		{"unknown top-level field", bytes.Replace(valid, []byte(`"frames":[]`), []byte(`"unknown":true,"frames":[]`), 1)},
		{"unknown nested field", []byte(`{"schema_version":1,"name":"strict","description":"strict fixture","summary_sha256":"digest","frames":[{"label":"x","direction":"client_to_server","state":"play","packet_id":0,"payload_hex":"","unexpected":true,"expect":{"result":"error","error_contains":"x"}}]}`)},
		{"trailing JSON", append(append([]byte(nil), valid...), []byte(` {}`)...)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeTranscriptCorpus(test.data); err == nil {
				t.Fatal("malformed corpus was accepted")
			}
		})
	}
}

func decodeTranscriptCorpus(data []byte) (transcriptCorpus, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var corpus transcriptCorpus
	if err := decoder.Decode(&corpus); err != nil {
		return transcriptCorpus{}, fmt.Errorf("decode transcript corpus: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("trailing JSON value")
		}
		return transcriptCorpus{}, fmt.Errorf("decode transcript corpus: %w", err)
	}
	return corpus, nil
}

func verifyTranscriptFrame(t *testing.T, codec *network.Codec, frame transcriptFrame, state network.State, payload []byte) string {
	t.Helper()
	var (
		packet any
		err    error
	)
	switch frame.Direction {
	case "client_to_server":
		packet, err = codec.DecodeClient(state, frame.PacketID, payload)
	case "server_to_client":
		packet, err = codec.DecodeServer(state, frame.PacketID, payload)
	default:
		t.Fatalf("frame %q has unknown direction %q", frame.Label, frame.Direction)
	}

	switch frame.Expect.Result {
	case "valid":
		if frame.Expect.PacketType == "" || frame.Expect.Summary == "" || frame.Expect.RejectBy != "" || frame.Expect.ErrorContains != "" {
			t.Fatalf("frame %q has invalid valid expectation", frame.Label)
		}
		if err != nil {
			t.Fatalf("frame %q decode: %v", frame.Label, err)
		}
		packetType := fmt.Sprintf("%T", packet)
		if packetType != frame.Expect.PacketType {
			t.Fatalf("frame %q packet type=%q, want %q", frame.Label, packetType, frame.Expect.PacketType)
		}
		summary := transcriptPacketSummary(packet)
		if summary != frame.Expect.Summary {
			t.Fatalf("frame %q summary=%q, want %q", frame.Label, summary, frame.Expect.Summary)
		}
		var encodedID uint32
		var encoded []byte
		switch packet := packet.(type) {
		case network.ClientPacket:
			encodedID, encoded, err = codec.EncodeClient(state, packet)
		case network.ServerPacket:
			encodedID, encoded, err = codec.EncodeServer(state, packet)
		default:
			t.Fatalf("frame %q decoded unsupported packet type %T", frame.Label, packet)
		}
		if err != nil {
			t.Fatalf("frame %q canonical encode: %v", frame.Label, err)
		}
		if encodedID != frame.PacketID || !bytes.Equal(encoded, payload) {
			t.Fatalf("frame %q is not canonical: id=%d payload=%x", frame.Label, encodedID, encoded)
		}
		return fmt.Sprintf("%s|%s|%s|%d|%s|%s", frame.Label, frame.Direction, frame.State, frame.PacketID, packetType, summary)
	case "error":
		if frame.Expect.PacketType != "" || frame.Expect.Summary != "" || frame.Expect.RejectBy == "" || frame.Expect.ErrorContains == "" {
			t.Fatalf("frame %q has invalid error expectation", frame.Label)
		}
		switch frame.Expect.RejectBy {
		case "codec":
			if err == nil {
				t.Fatalf("frame %q decoded successfully as %T, want codec error containing %q", frame.Label, packet, frame.Expect.ErrorContains)
			}
		case "protocol":
			if err != nil {
				t.Fatalf("frame %q codec decode failed before protocol validation: %v", frame.Label, err)
			}
			switch packet := packet.(type) {
			case network.ClientPacket:
				err = network.ValidateClientPacket(state, packet)
			case network.ServerPacket:
				err = network.ValidateServerPacket(state, packet)
			default:
				t.Fatalf("frame %q decoded unsupported packet type %T", frame.Label, packet)
			}
			if err == nil {
				t.Fatalf("frame %q passed protocol validation as %T, want error containing %q", frame.Label, packet, frame.Expect.ErrorContains)
			}
		default:
			t.Fatalf("frame %q has unknown reject_by %q", frame.Label, frame.Expect.RejectBy)
		}
		if !strings.Contains(err.Error(), frame.Expect.ErrorContains) {
			t.Fatalf("frame %q error=%q, want substring %q", frame.Label, err, frame.Expect.ErrorContains)
		}
		return fmt.Sprintf("%s|%s|%s|%d|error:%s|%s", frame.Label, frame.Direction, frame.State, frame.PacketID, frame.Expect.RejectBy, frame.Expect.ErrorContains)
	default:
		t.Fatalf("frame %q has unknown result %q", frame.Label, frame.Expect.Result)
		return ""
	}
}

func transcriptPacketSummary(packet any) string {
	switch packet := packet.(type) {
	case network.ClientHello:
		return fmt.Sprintf("protocol=%d", packet.ProtocolVersion)
	case network.ServerHello:
		return fmt.Sprintf("protocol=%d", packet.ProtocolVersion)
	case network.LoginStart:
		return fmt.Sprintf("player=%s,name=%q,view=%d", packet.PlayerID, packet.DisplayName, packet.ViewDistance)
	case network.LoginSuccess:
		return fmt.Sprintf("player=%s,world_seed=%d", packet.PlayerID, packet.WorldSeed)
	case network.PlayerInput:
		return fmt.Sprintf("sequence=%d,move=%d/%d,jump=%t,yaw=%g,pitch=%g,mining=%t,eating=%t,sprinting=%t,sneaking=%t", packet.Sequence, packet.MoveX, packet.MoveZ, packet.Jump, packet.Yaw, packet.Pitch, packet.Mining, packet.Eating, packet.Sprinting, packet.Sneaking)
	case network.ChunkSnapshot:
		return fmt.Sprintf("dimension=%d,chunk=%d/%d,revision=%d,sections=%d,payload_bytes=%d", packet.Dimension, packet.Chunk.X, packet.Chunk.Z, packet.Revision, len(packet.Sections), packet.PayloadBytes())
	case network.BlockChanges:
		return fmt.Sprintf("dimension=%d,chunk=%d/%d,revision=%d->%d,changes=%d", packet.Dimension, packet.Chunk.X, packet.Chunk.Z, packet.BaseRevision, packet.NewRevision, len(packet.Changes))
	case network.PlayerState:
		return fmt.Sprintf("tick=%d,ack=%d,dimension=%d,position=%g/%g/%g,velocity=%g/%g/%g,yaw=%g,pitch=%g,ready=%t,reset=%t,health=%d,hunger=%d,oxygen=%d,time=%d,weather=%d,season=%d,season_progress=%d,temperature=%d,armor=%d", packet.ServerTick, packet.LastInputSequence, packet.Dimension, packet.Position[0], packet.Position[1], packet.Position[2], packet.Velocity[0], packet.Velocity[1], packet.Velocity[2], packet.Yaw, packet.Pitch, packet.Ready, packet.Reset, packet.Health, packet.Hunger, packet.Oxygen, packet.WorldTimeTicks, packet.WeatherKind, packet.Season, packet.SeasonProgress, packet.Temperature, packet.ArmorPoints)
	case network.RemotePlayerSpawn:
		return fmt.Sprintf("player=%s,name=%q,tick=%d,dimension=%d,position=%g/%g/%g,yaw=%g,pitch=%g", packet.PlayerID, packet.DisplayName, packet.ServerTick, packet.Dimension, packet.Position[0], packet.Position[1], packet.Position[2], packet.Yaw, packet.Pitch)
	case network.RemotePlayerStates:
		return fmt.Sprintf("tick=%d,players=%d,first=%s", packet.ServerTick, len(packet.Players), packet.Players[0].PlayerID)
	case network.RemotePlayerDespawn:
		return fmt.Sprintf("player=%s", packet.PlayerID)
	case network.Disconnect:
		return fmt.Sprintf("code=%d,message=%q", packet.Code, packet.Message)
	default:
		return fmt.Sprintf("unsupported:%T", packet)
	}
}

func transcriptState(value string) (network.State, error) {
	switch value {
	case "handshake":
		return network.StateHandshake, nil
	case "login":
		return network.StateLogin, nil
	case "play":
		return network.StatePlay, nil
	default:
		return 0, fmt.Errorf("unknown state %q", value)
	}
}

func decodeCanonicalHex(value string) ([]byte, error) {
	if value != strings.ToLower(value) || len(value)%2 != 0 {
		return nil, fmt.Errorf("payload_hex is not canonical lowercase even-length hex")
	}
	payload, err := hex.DecodeString(value)
	if err != nil {
		return nil, err
	}
	if hex.EncodeToString(payload) != value {
		return nil, fmt.Errorf("payload_hex is not canonical")
	}
	return payload, nil
}

func baseNames(paths []string) []string {
	names := make([]string, len(paths))
	for index, path := range paths {
		names[index] = filepath.Base(path)
	}
	return names
}
