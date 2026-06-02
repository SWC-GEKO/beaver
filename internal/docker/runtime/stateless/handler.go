package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	beaver "github.com/SWC-GEKO/beaver/sdk"
	"github.com/SWC-GEKO/beaver/spec/api"
	"github.com/nats-io/nats.go"
)

type Config struct {
	Name     string
	NATSAddr string

	SubTopics       []string
	PubTopic        string
	DeadLetterTopic string
}

type Handler struct {
	Registry *beaver.Registry
	Nats     *nats.Conn
	Config
}

func main() {

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("error occurred loading config from env: %s", err)
	}

	h, err := New(*cfg)
	if err != nil {
		log.Fatalf("not able to create function handler, aborting: %s", err)
	}

	var subs []*nats.Subscription
	for _, t := range h.SubTopics {
		s, err := h.Nats.SubscribeSync(t)
		if err != nil {
			log.Fatalln(err)
		}
		subs = append(subs, s)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)

	serverErrChan := make(chan error)

	go func() {
		if err = http.ListenAndServe(":8080", mux); err != nil {
			serverErrChan <- err
		}
	}()

	var wg sync.WaitGroup

	for _, s := range subs {
		f := h.Registry.Get().Stateless

		wg.Add(1)
		go func(sub *nats.Subscription, fn api.StatelessFunction) {
			defer wg.Done()

			h.EventLoop(ctx, sub, fn)
		}(s, f)
	}

	select {
	case <-ctx.Done():
		log.Println("shutdown requested")
	case err = <-serverErrChan:
		log.Fatalf("http server failed: %v", err)
	}

	wg.Wait()

	if err = h.Nats.Drain(); err != nil {
		log.Printf("drain failed: %v", err)
	}
}

func New(cfg Config) (*Handler, error) {
	registry := beaver.Default()

	nc, err := nats.Connect(cfg.NATSAddr)
	if err != nil {
		return nil, err
	}

	return &Handler{
		Registry: registry,
		Nats:     nc,
		Config:   cfg,
	}, nil
}

// EventLoop is the core loop of the function, as it handles the nats-Subscription and the routing layer
func (h *Handler) EventLoop(ctx context.Context, sub *nats.Subscription, fn api.StatelessFunction) {
	defer sub.Unsubscribe()

	for msg := range sub.Msgs() {
		select {
		case <-ctx.Done():
			log.Printf("shutting event loop for topic: %s down", sub.Subject)
			return
		default:
		}

		e, err := parseMsgToEvent(msg)
		if err != nil {
			log.Printf("parsing of message failed with err: %v", err)
			// TODO: maybe add the error anywhere in the errors so that the user can trace errors better
			if err = h.Nats.Publish(h.DeadLetterTopic, msg.Data); err != nil {
				log.Println("publishing to dlq failed with err: ", err)
			}
			continue
		}

		resp, err := fn.Exec(ctx, e)
		if err != nil {
			log.Println("processing of event failed with err: ", err)
			// TODO: maybe add the error anywhere in the errors so that the user can trace errors better
			if err = h.Nats.Publish(h.DeadLetterTopic, msg.Data); err != nil {
				log.Println("publishing to dlq failed with err: ", err)
			}
			continue
		}

		bytes, err := parseEventToByteSlice(resp)
		if err != nil {
			log.Println("parsing of function response to byte slice failed with err: ", err)
			// TODO: maybe add the error anywhere in the errors so that the user can trace errors better
			if err = h.Nats.Publish(h.DeadLetterTopic, msg.Data); err != nil {
				log.Println("publishing to dlq failed with err: ", err)
			}
			continue
		}

		if err = h.Nats.Publish(h.PubTopic, bytes); err != nil {
			log.Println("publishing failed with error: ", err)
		}
	}
}

func LoadConfigFromEnv() (*Config, error) {

	name := os.Getenv("NAME")
	if name == "" {
		return nil, errors.New("name must be set")
	}

	natsAddr := os.Getenv("NATS_ADDR")
	if natsAddr == "" {
		return nil, errors.New("nats-addr must be set")
	}

	subTopicsStr := os.Getenv("SUB_TOPICS")
	var subTopics []string
	for _, s := range strings.Split(subTopicsStr, ",") {
		subTopics = append(subTopics, s)
	}

	if len(subTopics) == 0 {
		return nil, errors.New("must set at least one topic to consume from")
	}

	pubTopic := os.Getenv("PUB_TOPIC")
	if pubTopic == "" {
		return nil, errors.New("must set a topic to publish events to")
	}

	dlqTopic := os.Getenv("DLQ_TOPIC")
	if dlqTopic == "" {
		return nil, errors.New("must set a dlq-topic")
	}

	return &Config{
		Name:            name,
		NATSAddr:        natsAddr,
		SubTopics:       subTopics,
		PubTopic:        pubTopic,
		DeadLetterTopic: dlqTopic,
	}, nil
}

func parseMsgToEvent(msg *nats.Msg) (*api.Event, error) {
	// TODO: implement correct functionality!
	return &api.Event{
		Body: msg.Data,
	}, nil
}

func parseEventToByteSlice(e *api.Event) ([]byte, error) {
	// TODO: implement correct functionality!
	return json.Marshal(e)
}

func health(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(http.StatusOK)
}
