package actopus

import (
	"fmt"
	"io"
	"time"
)

type SupervisorConfig struct {
	MaxRestarts  int
	RestartDelay time.Duration
	MaxActors    int
}

type Supervisor struct {
	ID        int
	config    SupervisorConfig
	actors    map[int]*Actor
	createdAt time.Time

	logger io.Writer

	parent *Supervisor
	child  *Supervisor
}

func NewSupervisor(id int, logger io.Writer, config SupervisorConfig, parent *Supervisor) *Supervisor {
	return &Supervisor{
		ID:        id,
		createdAt: time.Now(),
		config:    config,
		actors:    make(map[int]*Actor),
		logger:    logger,
		child:     nil,
		parent:    parent,
	}
}

func (s *Supervisor) Run(a *Actor) {
	if len(s.actors) < s.config.MaxActors {
		// if enough room, assign to this supervisor.
		s.actors[a.pid] = a
		a.belongsTo = s
		s.supervise(a)
		a.start()
		a.setState(stateRunning)
		s.logger.Write([]byte(fmt.Sprintf("actor started under supervisor id: %d, pid: %d\n", s.ID, a.pid)))
		return
	}

	// if not enough room, create a new supervisor under this.
	child := NewSupervisor(s.ID+1, s.logger, s.config, s)
	s.child = child

	// assign to child
	child.Run(a)
}

func (s *Supervisor) supervise(a *Actor) {
	go func() {
		select {
		case <-a.ctx.Done():
			a.setState(stateStopped)
			s.logger.Write([]byte(fmt.Sprintf("actor stopped, pid: %d\n", a.pid)))
			return
		case err := <-a.errChan:
			if a.GetRestarts() < s.config.MaxRestarts {
				// @todo: one to one. support other strategies in the future
				a.setState(stateHealing)
				s.logger.Write([]byte(fmt.Sprintf("actor got error, restarting. err: %s, pid: %d\n", err.Error(), a.pid)))
				time.Sleep(s.config.RestartDelay)

				defer func() {
					a.incrementRestart()
					s.supervise(a)
					a.start()
					a.setState(stateRunning)
				}()
				return
			}

			// @todo: remove actor? restart supervisor?
			a.setState(stateFatal)
			s.logger.Write([]byte(fmt.Sprintf("actor max restarts limit reached. err: %s, pid: %d\n", err.Error(), a.pid)))
			return
		}
	}()
}
