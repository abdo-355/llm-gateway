package services

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	gatewayerrors "github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type blockingStreamBody struct {
	data      []byte
	closed    chan struct{}
	readBlock chan struct{}
	readDone  chan struct{}
	closeOnce sync.Once
	blockOnce sync.Once
	doneOnce  sync.Once
}

func newBlockingStreamBody(data string) *blockingStreamBody {
	return &blockingStreamBody{
		data:      []byte(data),
		closed:    make(chan struct{}),
		readBlock: make(chan struct{}),
		readDone:  make(chan struct{}),
	}
}

func (b *blockingStreamBody) Read(p []byte) (int, error) {
	if len(b.data) > 0 {
		n := copy(p, b.data)
		b.data = b.data[n:]
		return n, nil
	}

	b.blockOnce.Do(func() { close(b.readBlock) })
	<-b.closed
	b.doneOnce.Do(func() { close(b.readDone) })
	return 0, io.ErrClosedPipe
}

func (b *blockingStreamBody) Close() error {
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

func waitForParserResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("parser did not return")
		return nil
	}
}

func requireBodyClosed(t *testing.T, body *blockingStreamBody) {
	t.Helper()
	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("parser did not close stream body")
	}
}

func requireReadFinished(t *testing.T, body *blockingStreamBody) {
	t.Helper()
	select {
	case <-body.readDone:
	case <-time.After(time.Second):
		t.Fatal("parser returned before the blocked scan finished")
	}
}

func TestParseSSEStreamChannelDoneClosesBlockingBody(t *testing.T) {
	body := newBlockingStreamBody("data: [DONE]\n\n")
	result := make(chan error, 1)

	go func() {
		result <- newProviderService().parseSSEStreamChannel(
			context.Background(), body, make(chan *types.SSEChunk), "openai", "model", "request",
		)
	}()

	require.NoError(t, waitForParserResult(t, result))
	requireBodyClosed(t, body)
}

func TestParseSSEStreamChannelProviderErrorClosesBlockingBody(t *testing.T) {
	body := newBlockingStreamBody("data: {\"error\":{\"message\":\"stream failed\"}}\n\n")
	result := make(chan error, 1)

	go func() {
		result <- newProviderService().parseSSEStreamChannel(
			context.Background(), body, make(chan *types.SSEChunk), "openai", "model", "request",
		)
	}()

	var providerErr *gatewayerrors.ProviderError
	require.ErrorAs(t, waitForParserResult(t, result), &providerErr)
	requireBodyClosed(t, body)
}

func TestParseSSEStreamChannelCancellationClosesBlockingScan(t *testing.T) {
	body := newBlockingStreamBody("")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)

	go func() {
		result <- newProviderService().parseSSEStreamChannel(
			ctx, body, make(chan *types.SSEChunk), "openai", "model", "request",
		)
	}()

	<-body.readBlock
	cancel()
	require.ErrorIs(t, waitForParserResult(t, result), context.Canceled)
	requireBodyClosed(t, body)
	requireReadFinished(t, body)
}

func TestParseSSEStreamChannelDeadlineClosesBlockingScan(t *testing.T) {
	body := newBlockingStreamBody("")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)

	go func() {
		result <- newProviderService().parseSSEStreamChannel(
			ctx, body, make(chan *types.SSEChunk), "openai", "model", "request",
		)
	}()

	<-body.readBlock
	require.ErrorIs(t, waitForParserResult(t, result), context.DeadlineExceeded)
	requireBodyClosed(t, body)
	requireReadFinished(t, body)
}

func TestParseOllamaSSEStreamDoneClosesBlockingBody(t *testing.T) {
	body := newBlockingStreamBody("{\"model\":\"llama\",\"message\":{\"role\":\"assistant\",\"content\":\"done\"},\"done\":true}\n\n")
	chunks := make(chan *types.SSEChunk, 1)
	result := make(chan error, 1)

	go func() {
		result <- newProviderService().parseOllamaSSEStream(
			context.Background(), body, chunks, "llama", "request",
		)
	}()

	require.NoError(t, waitForParserResult(t, result))
	requireBodyClosed(t, body)
	chunk := <-chunks
	require.NotNil(t, chunk.Usage)
	assert.Equal(t, "done", *chunk.Choices[0].Delta.Content)
}

func TestCohereStreamCancellationUnblocksChunkSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan bool, 1)
	go func() {
		result <- sendCohereStreamChunk(ctx, make(chan *types.SSEChunk), &types.SSEChunk{})
	}()
	cancel()

	select {
	case sent := <-result:
		assert.False(t, sent)
	case <-time.After(time.Second):
		t.Fatal("Cohere stream remained blocked sending a chunk")
	}
}

func TestCohereStreamCancellationUnblocksErrorSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sendCohereStreamError(ctx, make(chan *types.GatewayError), &types.GatewayError{})
		close(done)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Cohere stream remained blocked sending an error")
	}
}
