package network

import (
	"context"
	"errors"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

var ErrClosed = errors.New("network: transport closed")

type ClientEndpoint interface {
	Send(context.Context, protocol.ClientMessage) error
	Recv(context.Context) (protocol.ServerMessage, error)
	Close() error
}

type ServerEndpoint interface {
	Send(context.Context, protocol.ServerMessage) error
	Recv(context.Context) (protocol.ClientMessage, error)
	Close() error
}

type clientPlayEndpoint struct {
	stream ClientPacketStream
}

func newClientPlayEndpoint(stream ClientPacketStream) ClientEndpoint {
	return &clientPlayEndpoint{stream: stream}
}

func (endpoint *clientPlayEndpoint) Send(ctx context.Context, message protocol.ClientMessage) error {
	packet, ok := message.(protocol.ClientPacket)
	if !ok {
		return errors.New("network: client message is not a packet")
	}
	return endpoint.stream.Send(ctx, protocol.StatePlay, packet)
}

func (endpoint *clientPlayEndpoint) Recv(ctx context.Context) (protocol.ServerMessage, error) {
	for {
		packet, err := endpoint.stream.Recv(ctx, protocol.StatePlay)
		if err != nil {
			return nil, err
		}
		switch packet := packet.(type) {
		case protocol.KeepAlive:
			if err := endpoint.stream.Send(ctx, protocol.StatePlay, protocol.KeepAliveReply{Token: packet.Token}); err != nil {
				return nil, err
			}
		case protocol.Disconnect:
			_ = endpoint.stream.Close()
			return nil, &RemoteError{State: protocol.StatePlay, Code: uint8(packet.Code), Message: packet.Message}
		default:
			message, ok := packet.(protocol.ServerMessage)
			if !ok {
				_ = endpoint.stream.Close()
				return nil, protocolViolation(errors.New("unexpected server play packet"))
			}
			return message, nil
		}
	}
}

func (endpoint *clientPlayEndpoint) Close() error {
	return endpoint.stream.Close()
}

type serverPlayEndpoint struct {
	stream ServerPacketStream
}

func newServerPlayEndpoint(stream ServerPacketStream) ServerEndpoint {
	return &serverPlayEndpoint{stream: stream}
}

func (endpoint *serverPlayEndpoint) Send(ctx context.Context, message protocol.ServerMessage) error {
	packet, ok := message.(protocol.ServerPacket)
	if !ok {
		return errors.New("network: server message is not a packet")
	}
	return endpoint.stream.Send(ctx, protocol.StatePlay, packet)
}

func (endpoint *serverPlayEndpoint) Recv(ctx context.Context) (protocol.ClientMessage, error) {
	packet, err := endpoint.stream.Recv(ctx, protocol.StatePlay)
	if err != nil {
		return nil, err
	}
	message, ok := packet.(protocol.ClientMessage)
	if !ok {
		_ = endpoint.stream.Close()
		return nil, protocolViolation(errors.New("unexpected client play packet"))
	}
	return message, nil
}

func (endpoint *serverPlayEndpoint) Close() error {
	return endpoint.stream.Close()
}
