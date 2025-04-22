package main

import (
	"errors"
	"fmt"
	"github.com/jarium/actopus"

	"os"
	"time"
)

// Example
func main() {
	conf := actopus.EngineConfig{
		Logger:         os.Stdout,
		ActorInboxSize: 1000,
		SupervisorConfig: actopus.SupervisorConfig{
			MaxRestarts:  1,
			RestartDelay: time.Second * 5,
			MaxActors:    4,
		},
	}

	eng := actopus.NewEngine(conf)

	//helloer
	helloer := func(msg string) error {
		fmt.Printf("hello message: %s\n", msg)
		return nil
	}
	actor1, _ := eng.Spawn(helloer, "helloer")
	actor1.Tell("message_1")
	actor1.Tell("message_2")

	//errorer
	errorer := func(msg string) error {
		return errors.New("i return error")
	}
	actor2, _ := eng.Spawn(errorer, "errorer")
	actor2.Tell("message_1")
	actor2.Tell("message_2")

	//panicker
	panicker := func(msg string) error {
		panic("i panic")
	}
	actor3, _ := eng.Spawn(panicker, "panicker")
	actor3.Tell("message_1")
	actor3.Tell("message_2")

	time.Sleep(time.Second * 10)
	fmt.Println(eng.Tree())
}
