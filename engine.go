package actopus

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync"
	"time"
)

var (
	ErrBehaviorExist = errors.New("actor with this name already exists")
)

type EngineConfig struct {
	Logger           io.Writer
	ActorInboxSize   int
	SupervisorConfig SupervisorConfig
}

// Engine wraps root supervisor process
type Engine struct {
	config   EngineConfig
	logger   io.Writer
	root     *Supervisor
	registry map[string]*Actor
	mu       sync.RWMutex
}

func NewEngine(config EngineConfig) *Engine {
	return &Engine{
		config:   config,
		logger:   config.Logger,
		registry: make(map[string]*Actor),
		root:     NewSupervisor(1, config.Logger, config.SupervisorConfig, nil),
	}
}

// Spawn an actor under available Supervisor.
func (e *Engine) Spawn(b behavior, name string) (*Actor, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.registry[name]; ok {
		return nil, ErrBehaviorExist
	}

	actor := NewActor(len(e.registry)+1, name, b, e.config.ActorInboxSize)
	last := e.getLastSupervisor()
	last.Run(actor)

	e.registry[name] = actor
	return actor, nil
}

// Tree returns json representation of current supervision tree, useful for monitoring
func (e *Engine) Tree() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	type ActorData struct {
		CreatedAt    string `json:"createdAt"`
		State        string `json:"state"`
		Restarts     int    `json:"restarts"`
		MessageCount int    `json:"messageCount"`
		Behavior     string `json:"behavior"`
		//@todo: add last error
	}
	type SupervisorData struct {
		ID        string               `json:"id"`
		CreatedAt string               `json:"createdAt"`
		Actors    map[string]ActorData `json:"actors"`
		Child     *SupervisorData      `json:"child"`
	}

	stateMap := map[int]string{
		stateInitialized: "Initialized",
		stateRunning:     "Running",
		stateStopped:     "Stopped",
		stateHealing:     "Healing",
		stateFatal:       "Fatal",
	}

	var data *SupervisorData

	for current := e.root; current != nil; current = current.child {
		currentData := &SupervisorData{
			ID:        "supervisor_" + strconv.Itoa(current.ID),
			Actors:    make(map[string]ActorData),
			CreatedAt: current.createdAt.Format(time.DateTime),
		}

		for _, actor := range current.actors {
			actorKey := "actor_" + strconv.Itoa(actor.pid)
			currentData.Actors[actorKey] = ActorData{
				CreatedAt:    actor.createdAt.Format(time.DateTime),
				State:        stateMap[actor.GetState()],
				Restarts:     actor.GetRestarts(),
				MessageCount: len(actor.inbox),
				Behavior:     actor.behaviorName,
			}
		}

		if data == nil {
			data = currentData
			continue
		}

		var lastChildData *SupervisorData
		for childData := data.Child; childData != nil; childData = childData.Child {
			lastChildData = childData
		}

		if lastChildData == nil {
			data.Child = currentData
			continue
		}

		lastChildData.Child = currentData
	}

	m, _ := json.Marshal(data)
	return string(m)
}

func (e *Engine) getLastSupervisor() *Supervisor {
	current := e.root
	for child := current.child; child != nil; child = child.child {
		current = child
	}

	return current
}
