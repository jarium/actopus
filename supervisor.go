package actopus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
)

type SupervisorConfig struct {
	MaxRestarts       int
	RestartDelay      time.Duration
	ShutdownInterval  time.Duration
	RestartStrategy   restartStrategy
	RestartValue      int
	ActorConfigs      []ActorConfig
	SupervisorConfigs []SupervisorConfig
}

type SupervisorInfo struct {
	ID                 string                    `json:"id"`
	CreatedAt          string                    `json:"createdAt"`
	UpdatedAt          string                    `json:"updatedAt"`
	State              string                    `json:"state"`
	Restarts           int                       `json:"restarts"`
	RestartedSpecCount int                       `json:"restartedSpecCount"`
	Actors             map[string]ActorInfo      `json:"actors,omitempty"`
	Children           map[string]SupervisorInfo `json:"children"`
	LastError          string                    `json:"lastError"`
}

type supervisor struct {
	config             SupervisorConfig
	children           []spec
	restartedSpecCount int
	logger             io.Writer
	*baseSpec
}

func newSupervisor(id string, index int, config SupervisorConfig, logger io.Writer) *supervisor {
	s := &supervisor{
		config:   config,
		logger:   logger,
		children: []spec{},
		baseSpec: newBaseSpec(id, index, config.RestartValue, config.ShutdownInterval),
	}

	for _, ac := range config.ActorConfigs {
		actorId := id + "_" + strconv.Itoa(len(s.children))
		s.children = append(s.children, newActor(actorId, len(s.children), ac))
	}

	for _, sc := range config.SupervisorConfigs {
		supervisorId := id + "_" + strconv.Itoa(len(s.children))
		s.children = append(s.children, newSupervisor(supervisorId, len(s.children), sc, logger))
	}

	return s
}

func (s *supervisor) run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	restartEvents := make(chan restartEvent, len(s.children))
	restarts := 0

	s.startChildren(ctx, restartEvents)

	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-restartEvents:
			s.logger.Write([]byte(fmt.Sprintf("supervisor: %s, restart event for %s, id: %s, err: %s\n", s.id, e.wrapper.sp.getName(), e.wrapper.sp.getId(), e.err.Error())))
			if s.config.MaxRestarts != 0 && restarts >= s.config.MaxRestarts {
				return errors.New("max restarts have been exceeded")
			}
			s.restart(ctx, e)
			restarts++
		}
	}
}

func (s *supervisor) getName() string {
	return specSupervisor
}

func (s *supervisor) restart(ctx context.Context, e restartEvent) {
	e.wrapper.sp.setState(stateRestarting)
	time.Sleep(s.config.RestartDelay)
	s.config.RestartStrategy(ctx, s, e)
	s.incrementRestartedSpecCount()
	e.wrapper.sp.incrementRestart()
	s.logger.Write([]byte(fmt.Sprintf("supervisor: %s, successful restart event by %s, id: %s\n", s.id, e.wrapper.sp.getName(), e.wrapper.sp.getId())))
}

func (s *supervisor) startSpec(wrapper *specWrapper) {
	s.logger.Write([]byte(fmt.Sprintf("supervisor: %s, spec %s starting, id: %s\n", s.id, wrapper.sp.getName(), wrapper.sp.getId())))
	wrapper.sp.setState(stateRunning)

	err := wrapper.sp.run(wrapper.ctx) //will block until stop

	wrapper.sp.setState(stateStopped)
	s.logger.Write([]byte(fmt.Sprintf("supervisor: %s, spec %s stopped, id: %s\n", s.id, wrapper.sp.getName(), wrapper.sp.getId())))
	if err != nil {
		s.logger.Write([]byte(fmt.Sprintf("supervisor: %s, spec %s got error, id: %s, err: %s\n", s.id, wrapper.sp.getName(), wrapper.sp.getId(), err.Error())))
		wrapper.sp.setLastError(err)
		if wrapper.sp.getRestartValue() == RestartValueTemporary {
			return
		}
		wrapper.restartChan <- restartEvent{
			wrapper: wrapper,
			err:     err,
		}
		return
	}

	if wrapper.sp.getRestartValue() == RestartValuePermanent {
		wrapper.restartChan <- restartEvent{
			wrapper: wrapper,
			err:     errors.New("restart value is permanent"),
		}
	}
}

func (s *supervisor) startChildren(ctx context.Context, restartChan chan restartEvent) {
	var wrappers []*specWrapper

	for _, sp := range s.children {
		wrapper := newSpecWrapper(ctx, sp, restartChan)
		wrappers = append(wrappers, wrapper)
	}

	for _, w := range wrappers {
		w.siblings = wrappers
		go func(w *specWrapper) {
			defer close(w.done)
			s.startSpec(w)
		}(w)
	}
}

func (s *supervisor) incrementRestartedSpecCount() {
	s.mu.Lock()
	defer s.mu.Unlock()

	t := time.Now()
	s.updatedAt = &t
	s.restartedSpecCount++
}

func (s *supervisor) getInfo() SupervisorInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	actorInfos := map[string]ActorInfo{}
	supervisorInfos := map[string]SupervisorInfo{}
	for _, c := range s.children {
		if c.getName() == specActor {
			actorInfos["actor_"+c.getId()] = c.(*Actor).GetInfo()
			continue
		}

		supervisorInfos["supervisor_"+c.getId()] = c.(*supervisor).getInfo()
	}

	var lastError string
	if s.lastError != nil {
		lastError = s.lastError.Error()
	}

	var updatedAt string
	if s.updatedAt != nil {
		updatedAt = s.updatedAt.Format(time.DateTime)
	}

	return SupervisorInfo{
		ID:                 "supervisor_" + s.getId(),
		CreatedAt:          s.createdAt.Format(time.DateTime),
		UpdatedAt:          updatedAt,
		State:              stateMap[s.state],
		Actors:             actorInfos,
		Children:           supervisorInfos,
		Restarts:           s.restarts,
		RestartedSpecCount: s.restartedSpecCount,
		LastError:          lastError,
	}
}
