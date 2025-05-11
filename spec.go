package actopus

import (
	"context"
	"sync"
	"time"
)

const (
	specActor      = "actor"
	specSupervisor = "supervisor"
)

const (
	RestartValuePermanent = iota
	RestartValueTransient
	RestartValueTemporary
)

const (
	stateInitialized = iota
	stateRunning
	stateStopping
	stateStopped
	stateRestarting
)

var stateMap = map[int]string{
	stateInitialized: "Initialized",
	stateRunning:     "Running",
	stateStopping:    "Stopping",
	stateStopped:     "Stopped",
	stateRestarting:  "Restarting",
}

type restartEvent struct {
	wrapper *specWrapper
	err     error
}

type specWrapper struct {
	sp               spec
	siblings         []*specWrapper
	shutdownInterval time.Duration
	ctx              context.Context
	cancel           context.CancelFunc
	restartChan      chan restartEvent
	done             chan struct{}
}

func newSpecWrapper(ctx context.Context, sp spec, restartChan chan restartEvent) *specWrapper {
	ctx, cancel := context.WithCancel(ctx)
	return &specWrapper{
		sp:               sp,
		ctx:              ctx,
		cancel:           cancel,
		restartChan:      restartChan,
		shutdownInterval: sp.getShutdownInterval(),
		done:             make(chan struct{}),
	}
}

func (sw *specWrapper) stop() {
	sw.sp.setState(stateStopping)
	time.Sleep(sw.shutdownInterval)
	sw.cancel()
	<-sw.done
}

type spec interface {
	getId() string
	run(ctx context.Context) error
	setState(state int)
	incrementRestart()
	setLastError(err error)
	getShutdownInterval() time.Duration
	getName() string
	getIndex() int
	getRestartValue() int
}

type baseSpec struct {
	id               string
	index            int
	createdAt        time.Time
	updatedAt        *time.Time
	restarts         int
	restartValue     int
	shutdownInterval time.Duration
	state            int
	lastError        error
	mu               sync.RWMutex
}

func newBaseSpec(id string, index int, restartValue int, shutdownInterval time.Duration) *baseSpec {
	return &baseSpec{
		id:               id,
		index:            index,
		createdAt:        time.Now(),
		restartValue:     restartValue,
		shutdownInterval: shutdownInterval,
		state:            stateInitialized,
	}
}

func (b *baseSpec) setState(state int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.state = state
}

func (b *baseSpec) getId() string {
	return b.id
}

func (b *baseSpec) incrementRestart() {
	b.mu.Lock()
	defer b.mu.Unlock()

	t := time.Now()
	b.updatedAt = &t
	b.restarts++
}

func (b *baseSpec) setLastError(last error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	t := time.Now()
	b.updatedAt = &t
	b.lastError = last
}

func (b *baseSpec) getShutdownInterval() time.Duration {
	return b.shutdownInterval
}

func (b *baseSpec) getIndex() int {
	return b.index
}

func (b *baseSpec) getRestartValue() int {
	return b.restartValue
}
