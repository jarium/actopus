package actopus

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type behavior func(msg string) error

type Message struct {
	From *Actor
	Data string
}

const (
	stateInitialized = iota
	stateRunning
	stateStopped
	stateHealing
	stateFatal
)

type Actor struct {
	ctx          context.Context
	cancelFn     context.CancelFunc
	mu           sync.RWMutex
	createdAt    time.Time
	behaviorName string

	pid     int
	state   int
	fn      behavior
	inbox   chan Message
	errChan chan error

	belongsTo *Supervisor
	restarts  int
}

func NewActor(pid int, behaviorName string, fn behavior, inboxLimit int) *Actor {
	ctx, cancel := context.WithCancel(context.Background())

	return &Actor{
		ctx:          ctx,
		cancelFn:     cancel,
		createdAt:    time.Now(),
		behaviorName: behaviorName,
		pid:          pid,
		state:        stateInitialized,
		fn:           fn,
		inbox:        make(chan Message, inboxLimit),
		errChan:      make(chan error),
	}
}

// Stop @todo: poison pill instead of this
func (a *Actor) Stop() {
	a.cancelFn()
}

func (a *Actor) Send(to *Actor, data string) {
	to.inbox <- Message{
		From: a,
		Data: data,
	}
}

func (a *Actor) Tell(data string) {
	a.inbox <- Message{
		Data: data,
	}
}

func (a *Actor) PID() int {
	return a.pid
}

func (a *Actor) Context() context.Context {
	return a.ctx
}

func (a *Actor) start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.errChan <- fmt.Errorf("actor panicked, pid: %d, panic: %v", a.pid, r)
				return
			}
		}()

		for {
			select {
			case msg := <-a.inbox:
				if err := a.fn(msg.Data); err != nil {
					a.errChan <- err
					return
				}
			case <-a.ctx.Done():
				// @todo: graceful stop
				return
			}
		}
	}()
}

func (a *Actor) GetState() int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.state
}

func (a *Actor) setState(state int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state = state
}

func (a *Actor) GetRestarts() int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.restarts
}

func (a *Actor) incrementRestart() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.restarts++
}
