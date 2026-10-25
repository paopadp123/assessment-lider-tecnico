package app

import (
	"context"
	"testing"
)

func outboxMessages(ids ...string) []OutboxMessage {
	msgs := make([]OutboxMessage, len(ids))
	for i, id := range ids {
		msgs[i] = OutboxMessage{ID: int64(i + 1), MessageID: id, Type: CommandRequestPayment}
	}
	return msgs
}

func TestDispatcher_PublishesPendingMessages(t *testing.T) {
	store := &fakeOutbox{pending: outboxMessages("m1", "m2")}
	pub := &fakePublisher{}

	n, err := NewDispatcher(store, pub, discardLogger()).DispatchOnce(context.Background())

	if err != nil || n != 2 || len(store.pending) != 0 || len(pub.sent) != 2 {
		t.Fatalf("n=%d err=%v pendientes=%d enviados=%v", n, err, len(store.pending), pub.sent)
	}
}

// Regla crítica: si el broker no confirma, el mensaje no se pierde; queda pendiente para el siguiente ciclo.
func TestDispatcher_UnconfirmedMessageStaysPendingAndIsRetried(t *testing.T) {
	store := &fakeOutbox{pending: outboxMessages("m1", "m2")}
	pub := &fakePublisher{failFor: map[string]bool{"m1": true}}
	d := NewDispatcher(store, pub, discardLogger())

	n, _ := d.DispatchOnce(context.Background())

	if n != 1 || len(store.pending) != 1 || store.pending[0].MessageID != "m1" || store.pending[0].Attempts != 1 {
		t.Fatalf("n=%d pendientes=%+v", n, store.pending)
	}

	pub.failFor = nil
	if n, _ := d.DispatchOnce(context.Background()); n != 1 || len(store.pending) != 0 {
		t.Fatalf("segundo ciclo: n=%d pendientes=%d", n, len(store.pending))
	}
}

func TestDispatcher_RunStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := NewDispatcher(&fakeOutbox{}, &fakePublisher{}, discardLogger()).Run(ctx); err != nil {
		t.Fatalf("Run con contexto cancelado debe terminar limpio: %v", err)
	}
}
