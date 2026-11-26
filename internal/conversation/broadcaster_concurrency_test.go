// ABOUTME: Regression tests for publishing while subscriber channels are closed.
// ABOUTME: Covers explicit shutdown, context cancellation, and slow drop logging.

package conversation

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/store"
	"github.com/stretchr/testify/require"
)

type blockingDropHandler struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (*blockingDropHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *blockingDropHandler) Handle(_ context.Context, record slog.Record) error {
	if record.Message == "dropped event for slow subscriber" {
		h.once.Do(func() {
			close(h.entered)
			<-h.release
		})
	}
	return nil
}

func (h *blockingDropHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *blockingDropHandler) WithGroup(string) slog.Handler      { return h }

func shutdownBroadcaster(b *EventBroadcaster, mode string, ids [2]string, cancel context.CancelFunc) {
	defer cancel()
	switch mode {
	case "unsubscribe":
		for _, id := range ids {
			b.Unsubscribe("agent-1", id)
		}
	case "close":
		b.Close()
	case "cancel":
		cancel()
	}
}

func TestBroadcaster_ShutdownWhileLoggingDrop(t *testing.T) {
	for _, mode := range []string{"unsubscribe", "close", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			handler := &blockingDropHandler{entered: make(chan struct{}), release: make(chan struct{})}
			b := NewEventBroadcaster(slog.New(handler))
			defer b.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(handler.release) }) }
			defer release()

			ch1, id1 := b.Subscribe(ctx, "agent-1")
			ch2, id2 := b.Subscribe(ctx, "agent-1")
			event := makeEvent("buffered", "agent-1")
			for range cap(ch1) {
				b.Publish("agent-1", event, "")
			}

			published := make(chan any, 1)
			go func() {
				defer func() { published <- recover() }()
				b.Publish("agent-1", makeEvent("overflow", "agent-1"), "")
			}()
			select {
			case <-handler.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("publisher did not reach drop logging")
			}

			closed := make(chan struct{})
			go func() {
				shutdownBroadcaster(b, mode, [2]string{id1, id2}, cancel)
				for _, ch := range []<-chan *store.LedgerEvent{ch1, ch2} {
					for range ch {
					}
				}
				close(closed)
			}()
			select {
			case <-closed:
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown blocked while the drop logger was running")
			}

			release()
			select {
			case panicValue := <-published:
				require.Nil(t, panicValue, "publishing after concurrent shutdown must not panic")
			case <-time.After(5 * time.Second):
				t.Fatal("publisher did not finish after releasing the logger")
			}
		})
	}
}

func TestBroadcaster_ConcurrentPublishShutdown(t *testing.T) {
	for _, mode := range []string{"unsubscribe", "close", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			b := NewEventBroadcaster(nil)
			defer b.Close()
			event := makeEvent("concurrent", "agent-1")
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			resetTimeout := func() {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(5 * time.Second)
			}
			for range 100 {
				ctx, cancel := context.WithCancel(t.Context())
				ch1, id1 := b.Subscribe(ctx, "agent-1")
				ch2, id2 := b.Subscribe(ctx, "agent-1")
				start := make(chan struct{})
				published := make(chan any, 1)
				go func() {
					defer func() { published <- recover() }()
					<-start
					for range 128 {
						b.Publish("agent-1", event, "")
					}
				}()
				closed := make(chan struct{})
				go func() {
					<-start
					shutdownBroadcaster(b, mode, [2]string{id1, id2}, cancel)
					for _, ch := range []<-chan *store.LedgerEvent{ch1, ch2} {
						for range ch {
						}
					}
					close(closed)
				}()
				close(start)
				resetTimeout()
				select {
				case panicValue := <-published:
					require.Nil(t, panicValue, "concurrent publishing and shutdown must not panic")
				case <-timer.C:
					cancel()
					t.Fatal("concurrent publisher did not finish")
				}
				resetTimeout()
				select {
				case <-closed:
				case <-timer.C:
					cancel()
					t.Fatal("concurrent shutdown did not close both channels")
				}
			}
		})
	}
}
