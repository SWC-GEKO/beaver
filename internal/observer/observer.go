package observer

import (
	"context"
	"log"
	"sync"

	"github.com/SWC-GEKO/beaver/internal/composer"
	"github.com/nats-io/nats.go/jetstream"
)

type Observer struct {
	Stream string
	Topics map[string]*Topic
	mtx    sync.RWMutex

	JetStream jetstream.JetStream
	Composer  *composer.Composer
}

func NewObserver(stream string, js jetstream.JetStream, c *composer.Composer) {}

func (o *Observer) Add(ctx context.Context, t *Topic) bool {
	o.mtx.Lock()
	defer o.mtx.Unlock()

	if _, ok := o.Topics[t.name]; ok {
		return false
	}

	o.Topics[t.name] = t
	o.Watch(ctx, t)
	return true
}

func (o *Observer) Watch(ctx context.Context, t *Topic) {
	watchCtx, cancel := context.WithCancel(ctx)
	t.mtx.Lock()
	t.stop = cancel
	t.mtx.Unlock()

	consumer, err := o.JetStream.CreateOrUpdateConsumer(watchCtx, o.Stream, jetstream.ConsumerConfig{
		DeliverPolicy: jetstream.DeliverNewPolicy,
		AckPolicy:     jetstream.AckNonePolicy,
		FilterSubject: t.name,
	})
	if err != nil {
		cancel()
		return
	}

	consumer.Consume(func(msg jetstream.Msg) {
		t.mtx.Lock()
		defer t.mtx.Unlock()

		if t.state != Idle {
			return
		}

		meta, err := msg.Metadata()
		if err != nil {
			return
		}

		t.state = Active

		cancel()

		seq := meta.Sequence.Stream
		go o.startFunction(ctx, t.name, seq)

	}, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		// TODO: do proper error handling
		log.Printf("consuming from topic: %s caused err: %s", t.name, err)
	}))
}

func (o *Observer) startFunction(ctx context.Context, uniqueName string, seq uint64) {
	if err := o.Composer.Up(ctx, uniqueName, seq); err != nil {
		// TODO: implement fallback to check what the issue is
		// TODO: if the issue is -> function is already running -> no problem
		// TODO: if the issue is -> function is stopped and down -> problem
	}
}
