package tcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/channing771/mornlea/internal/network"
)

type streamConn interface {
	io.ReadWriteCloser
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

type ownerGate struct {
	init  sync.Once
	token chan struct{}
}

func (gate *ownerGate) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	gate.init.Do(func() {
		gate.token = make(chan struct{}, 1)
		gate.token <- struct{}{}
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-gate.token:
	}
	if err := ctx.Err(); err != nil {
		gate.release()
		return err
	}
	return nil
}

func (gate *ownerGate) release() {
	gate.token <- struct{}{}
}

type tcpStream struct {
	conn  streamConn
	codec *network.Codec

	readOwner  ownerGate
	writeOwner ownerGate

	closeOnce sync.Once
	closeErr  error
	closed    atomic.Bool
}

type tcpClientStream struct {
	stream *tcpStream
}

type tcpServerStream struct {
	stream *tcpStream
	peer   string
}

func (client *tcpClientStream) Send(ctx context.Context, state network.State, packet network.ClientPacket) error {
	return client.stream.send(ctx, func() (uint32, []byte, error) {
		return client.stream.codec.EncodeClient(state, packet)
	})
}

func (client *tcpClientStream) Recv(ctx context.Context, state network.State) (network.ServerPacket, error) {
	return receivePacket(ctx, client.stream, func(id uint32, payload []byte) (network.ServerPacket, error) {
		return client.stream.codec.DecodeServer(state, id, payload)
	})
}

func (client *tcpClientStream) Close() error {
	return client.stream.Close()
}

func (server *tcpServerStream) Send(ctx context.Context, state network.State, packet network.ServerPacket) error {
	return server.stream.send(ctx, func() (uint32, []byte, error) {
		return server.stream.codec.EncodeServer(state, packet)
	})
}

func (server *tcpServerStream) Recv(ctx context.Context, state network.State) (network.ClientPacket, error) {
	return receivePacket(ctx, server.stream, func(id uint32, payload []byte) (network.ClientPacket, error) {
		return server.stream.codec.DecodeClient(state, id, payload)
	})
}

func (server *tcpServerStream) Peer() string {
	return server.peer
}

func (server *tcpServerStream) Close() error {
	return server.stream.Close()
}

func (stream *tcpStream) send(
	ctx context.Context,
	encode func() (uint32, []byte, error),
) error {
	if err := stream.writeOwner.acquire(ctx); err != nil {
		return err
	}
	defer stream.writeOwner.release()

	if err := ctx.Err(); err != nil {
		return err
	}
	if stream.closed.Load() {
		return network.ErrClosed
	}
	cleanup := installWriteContext(ctx, stream.conn)
	defer cleanup()

	id, payload, err := encode()
	if err != nil {
		if stream.closed.Load() {
			return network.ErrClosed
		}
		_ = stream.Close()
		return err
	}
	if err := network.WriteFrame(stream.conn, id, payload); err != nil {
		return stream.failIO(ctx, "write frame", err)
	}
	return nil
}

func receivePacket[T any](
	ctx context.Context,
	stream *tcpStream,
	decode func(uint32, []byte) (T, error),
) (T, error) {
	var zero T
	if err := stream.readOwner.acquire(ctx); err != nil {
		return zero, err
	}
	defer stream.readOwner.release()

	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if stream.closed.Load() {
		return zero, network.ErrClosed
	}
	cleanup := installReadContext(ctx, stream.conn)
	defer cleanup()

	id, payload, err := network.ReadFrame(stream.conn)
	if err != nil {
		return zero, stream.failIO(ctx, "read frame", err)
	}
	packet, err := decode(id, payload)
	if err != nil {
		if stream.closed.Load() {
			return zero, network.ErrClosed
		}
		_ = stream.Close()
		return zero, err
	}
	return packet, nil
}

func (stream *tcpStream) failIO(ctx context.Context, operation string, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.ErrClosedPipe) || isTCPStreamClosedError(err) {
		_ = stream.Close()
		return network.ErrClosed
	}
	_ = stream.Close()
	return fmt.Errorf("network: %s: %w", operation, err)
}

func (stream *tcpStream) Close() error {
	if stream == nil {
		return nil
	}
	stream.closeOnce.Do(func() {
		stream.closed.Store(true)
		connErr := stream.conn.Close()
		if isTCPStreamClosedError(connErr) {
			connErr = nil
		}
		codecErr := stream.codec.Close()
		stream.closeErr = errors.Join(connErr, codecErr)
	})
	return stream.closeErr
}

func installReadContext(ctx context.Context, conn streamConn) func() {
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
	}
	callbackDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = conn.SetReadDeadline(time.Now())
		close(callbackDone)
	})
	return func() {
		if !stop() {
			<-callbackDone
		}
		_ = conn.SetReadDeadline(time.Time{})
	}
}

func installWriteContext(ctx context.Context, conn streamConn) func() {
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(deadline)
	}
	callbackDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = conn.SetWriteDeadline(time.Now())
		close(callbackDone)
	})
	return func() {
		if !stop() {
			<-callbackDone
		}
		_ = conn.SetWriteDeadline(time.Time{})
	}
}

var _ network.ClientPacketStream = (*tcpClientStream)(nil)
var _ network.ServerPacketStream = (*tcpServerStream)(nil)
