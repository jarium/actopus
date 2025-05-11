package actopus

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type ActorConfig struct {
	Name             string //must be unique or will be overridden by the next name
	RestartValue     int
	InboxLimit       int
	ShutdownInterval time.Duration
	Behavior         func(msg Message) error
}

var (
	poisonPill  = Message{Data: "<actopus-poison-pill>"}
	errPoisoned = errors.New("context poisoned by poisonPill")
)

type Message struct {
	From *Actor
	Data string
}

type ActorInfo struct {
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	State        string `json:"state"`
	Restarts     int    `json:"restarts"`
	MessageCount int    `json:"messageCount"`
	MaxMessages  int    `json:"maxMessages"`
	Behavior     string `json:"behavior"`
	LastError    string `json:"lastError"`
}

type Actor struct {
	config ActorConfig
	do     func() error
	inbox  chan Message
	*baseSpec
}

func newActor(id string, index int, c ActorConfig) *Actor {
	a := &Actor{
		config:   c,
		inbox:    make(chan Message, c.InboxLimit),
		baseSpec: newBaseSpec(id, index, c.RestartValue, c.ShutdownInterval),
	}

	a.do = func() (runErr error) {
		defer func() {
			if r := recover(); r != nil {
				runErr = fmt.Errorf("panic: %v", r)
				return
			}
		}()

		select {
		case msg := <-a.inbox:
			if msg == poisonPill {
				return errPoisoned
			}

			return a.config.Behavior(msg)
		default:
			return
		}
	}

	return a
}

func (a *Actor) run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if err := a.do(); err != nil {
				if errors.Is(err, errPoisoned) {
					return nil
				}
				return err
			}
		}
	}
}

func (a *Actor) getName() string {
	return specActor
}

func (a *Actor) Tell(msg Message) {
	a.inbox <- msg
}

// GetInfo about this Actor
func (a *Actor) GetInfo() ActorInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var lastError string
	if a.lastError != nil {
		lastError = a.lastError.Error()
	}

	var updatedAt string
	if a.updatedAt != nil {
		updatedAt = a.updatedAt.Format(time.DateTime)
	}

	return ActorInfo{
		CreatedAt:    a.createdAt.Format(time.DateTime),
		UpdatedAt:    updatedAt,
		State:        stateMap[a.state],
		Restarts:     a.restarts,
		MessageCount: len(a.inbox),
		MaxMessages:  a.config.InboxLimit,
		Behavior:     a.config.Name,
		LastError:    lastError,
	}
}
