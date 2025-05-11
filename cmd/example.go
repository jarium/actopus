package main

import (
	"errors"
	"fmt"
	"github.com/jarium/actopus"
	"time"
)

func main() {
	printer := actopus.ActorConfig{
		Name:             "printer",
		RestartValue:     actopus.RestartValuePermanent,
		InboxLimit:       100,
		ShutdownInterval: time.Second / 2,
		Behavior: func(msg actopus.Message) error {
			fmt.Println(msg.Data)
			return nil
		},
	}

	errorer := actopus.ActorConfig{
		Name:             "errorer",
		RestartValue:     actopus.RestartValuePermanent,
		InboxLimit:       100,
		ShutdownInterval: time.Second / 2,
		Behavior: func(msg actopus.Message) error {
			return errors.New("there was an error")
		},
	}

	panicker := actopus.ActorConfig{
		Name:             "panicker",
		RestartValue:     actopus.RestartValuePermanent,
		InboxLimit:       100,
		ShutdownInterval: time.Second / 2,
		Behavior: func(msg actopus.Message) error {
			panic("whoops")
		},
	}

	supervisor := actopus.SupervisorConfig{
		MaxRestarts:      1,
		RestartDelay:     time.Second / 2,
		ShutdownInterval: time.Second / 2,
		RestartStrategy:  actopus.OneForOneStrategy,
		RestartValue:     actopus.RestartValuePermanent,
		ActorConfigs: []actopus.ActorConfig{
			printer,
			errorer,
			panicker,
		},
		SupervisorConfigs: nil,
	}

	engine := actopus.NewEngine(actopus.OptionSupervisors(supervisor))
	ctx := engine.Start()
	go func() {
		time.Sleep(time.Second * 30)
		engine.Stop()
	}()

	actors := []*actopus.Actor{
		engine.GetActor("printer"),
		engine.GetActor("errorer"),
		engine.GetActor("panicker"),
	}

	for _, a := range actors {
		a.Tell(actopus.Message{
			Data: "hi actors!",
		})
	}

	<-ctx.Done()
	fmt.Println(engine.Tree())
}
