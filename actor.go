package actopus

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var discardOnAction = func() error { return nil }

type ActorConfig struct {
	Name             string //must be unique or will be overridden by the next name
	RestartValue     int
	InboxLimit       int
	ShutdownInterval time.Duration
	OnStart          func() error
	OnStop           func() error
	OnError          func() error
	OnPanic          func() error
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
	if c.OnStart == nil {
		c.OnStart = discardOnAction
	}
	if c.OnStop == nil {
		c.OnStop = discardOnAction
	}
	if c.OnError == nil {
		c.OnError = discardOnAction
	}
	if c.OnPanic == nil {
		c.OnPanic = discardOnAction
	}

	a := &Actor{
		config:   c,
		inbox:    make(chan Message, c.InboxLimit),
		baseSpec: newBaseSpec(id, index, c.RestartValue, c.ShutdownInterval),
	}

	a.do = func() (runErr error) {
		defer func() {
			if r := recover(); r != nil {
				runErr = fmt.Errorf("panic: %v", r)
				if onPanicErr := c.OnPanic(); onPanicErr != nil {
					runErr = fmt.Errorf("runErr: %w, onPanicErr: %w", runErr, onPanicErr)
				}
				return
			}
		}()

		select {
		case msg := <-a.inbox:
			if msg == poisonPill {
				return errPoisoned
			}

			if err := a.config.Behavior(msg); err != nil {
				runErr = err
				if onErrorErr := a.config.OnError(); onErrorErr != nil {
					runErr = fmt.Errorf("runErr: %w, onErrorErr: %w", runErr, onErrorErr)
				}
			}
			return
		default:
			return
		}
	}

	return a
}

func (a *Actor) run(ctx context.Context) (err error) {
	if onStartErr := a.config.OnStart(); onStartErr != nil {
		return fmt.Errorf("onStartErr: %w", onStartErr)
	}

	defer func() {
		if err == nil {
			if onStopErr := a.config.OnStop(); onStopErr != nil {
				err = fmt.Errorf("onStopErr: %w", onStopErr)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if doErr := a.do(); doErr != nil {
				if errors.Is(doErr, errPoisoned) {
					return nil
				}
				err = doErr
				return
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
