package actopus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

var (
	OptionWithLogger = func(logger io.Writer) func(e *Engine) {
		return func(e *Engine) {
			e.logger = logger
		}
	}
	OptionWithRoot = func(rc RootConfig) func(e *Engine) {
		return func(e *Engine) {
			e.rootConstructor = func(logger io.Writer) *supervisor {
				return newSupervisor("root", 0, rc.ToSupervisorConfig(), logger)
			}
		}
	}
	OptionSupervisors = func(supervisorConfigs ...SupervisorConfig) func(e *Engine) {
		return func(e *Engine) {
			for i, c := range supervisorConfigs {
				e.supervisorConstructors = append(e.supervisorConstructors, func(logger io.Writer) *supervisor {
					return newSupervisor(strconv.Itoa(i), i, c, e.logger)
				})
			}
		}
	}
)

type Option func(e *Engine)

var defaultRootConfig = RootConfig{
	MaxRestarts:      0,
	RestartDelay:     0,
	ShutdownInterval: 0,
	RestartStrategy:  OneForOneStrategy,
	RestartValue:     RestartValueTemporary,
}

type RootConfig struct {
	MaxRestarts      int
	RestartDelay     time.Duration
	ShutdownInterval time.Duration
	RestartStrategy  restartStrategy
	RestartValue     int
}

func (r RootConfig) ToSupervisorConfig() SupervisorConfig {
	return SupervisorConfig{
		MaxRestarts:      r.MaxRestarts,
		RestartDelay:     r.RestartDelay,
		ShutdownInterval: r.ShutdownInterval,
		RestartStrategy:  r.RestartStrategy,
		RestartValue:     r.RestartValue,
	}
}

type Engine struct {
	logger                 io.Writer
	root                   *supervisor
	rootConstructor        func(logger io.Writer) *supervisor
	supervisorConstructors []func(logger io.Writer) *supervisor
	registry               map[string]*Actor
	ctx                    context.Context
	cancel                 context.CancelFunc
}

func NewEngine(opts ...Option) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	e := &Engine{
		supervisorConstructors: []func(logger io.Writer) *supervisor{},
		registry:               map[string]*Actor{},
		ctx:                    ctx,
		cancel:                 cancel,
	}

	for _, o := range opts {
		o(e)
	}

	if e.logger == nil {
		e.logger = os.Stdout
	}
	if e.rootConstructor == nil {
		e.rootConstructor = func(logger io.Writer) *supervisor {
			return newSupervisor("root", 0, defaultRootConfig.ToSupervisorConfig(), e.logger)
		}
	}
	e.root = e.rootConstructor(e.logger)

	var supervisors []spec
	for _, sc := range e.supervisorConstructors {
		supervisors = append(supervisors, sc(e.logger))
	}
	e.root.children = append(e.root.children, supervisors...)

	for _, s := range supervisors {
		for _, c := range s.(*supervisor).children {
			if c.getName() == specActor {
				actor := c.(*Actor)
				e.registry[actor.config.Name] = actor
			}
		}
	}

	return e
}

func (e *Engine) Start() context.Context {
	go func() {
		if err := e.root.run(e.ctx); err != nil {
			e.logger.Write([]byte(fmt.Sprintf("root error: %s\n", err.Error())))
		}
	}()

	return e.ctx
}

func (e *Engine) Stop() {
	e.cancel()
}

// GetActor returns defined Actor or nil if it not exists
func (e *Engine) GetActor(name string) *Actor {
	return e.registry[name]
}

// Tree returns json representation of current supervision tree
func (e *Engine) Tree() string {
	m, _ := json.Marshal(e.root.getInfo())
	return string(m)
}
