package actopus

import (
	"context"
	"fmt"
	"slices"
	"sort"
)

type restartStrategy func(ctx context.Context, s *supervisor, e restartEvent)

var (
	OneForOneStrategy = func(ctx context.Context, s *supervisor, e restartEvent) {
		e.wrapper.stop()
		s.logger.Write([]byte(fmt.Sprintf("supervisor: %s, stopped %s, id: %s\n", s.id, e.wrapper.sp.getName(), e.wrapper.sp.getId())))

		newWrapper := newSpecWrapper(ctx, e.wrapper.sp, e.wrapper.restartChan)
		e.wrapper.siblings[e.wrapper.sp.getIndex()] = newWrapper

		newWrapper.siblings = e.wrapper.siblings
		s.logger.Write([]byte(fmt.Sprintf("supervisor: %s, running %s, id: %s\n", s.id, e.wrapper.sp.getName(), e.wrapper.sp.getId())))
		go func(w *specWrapper) {
			defer close(w.done)
			s.startSpec(w)
		}(newWrapper)
	}
	OneForAllStrategy = func(ctx context.Context, s *supervisor, e restartEvent) {
		for i := len(e.wrapper.siblings) - 1; i >= 0; i-- {
			wrapper := e.wrapper.siblings[i]
			wrapper.stop()
			s.logger.Write([]byte(fmt.Sprintf("supervisor: %s, stopped %s, id: %s\n", s.id, wrapper.sp.getName(), wrapper.sp.getId())))
		}

		for len(e.wrapper.restartChan) > 0 {
			<-e.wrapper.restartChan //drain channel in case multiple specs failed at the same time
		}

		s.startChildren(ctx, e.wrapper.restartChan)
	}
	RestForOneStrategy = func(ctx context.Context, s *supervisor, e restartEvent) {
		events := []restartEvent{e}
		for len(e.wrapper.restartChan) > 0 {
			events = append(events, <-e.wrapper.restartChan) //drain channel into slice
		}

		sort.Slice(events, func(i, j int) bool {
			return events[i].wrapper.sp.getIndex() < events[j].wrapper.sp.getIndex()
		})

		//restart from the lowest index in a case of multiple specs failed at the same time
		lowestIndexedEvent := events[0]
		childrenToRestart := e.wrapper.siblings[lowestIndexedEvent.wrapper.sp.getIndex():]

		stoppedIds := map[string]bool{}
		for i := len(childrenToRestart) - 1; i >= 0; i-- {
			wrapper := childrenToRestart[i]
			wrapper.stop()
			s.logger.Write([]byte(fmt.Sprintf("supervisor: %s, stopped %s, id: %s\n", s.id, wrapper.sp.getName(), wrapper.sp.getId())))
			stoppedIds[wrapper.sp.getId()] = true
		}

		//if we get events due to RestartValuePermanent after stopping wrappers, ignore them
		var eventsAfterStop []restartEvent
		for len(e.wrapper.restartChan) > 0 {
			eventsAfterStop = append(eventsAfterStop, <-e.wrapper.restartChan)
		}
		slices.DeleteFunc(eventsAfterStop, func(event restartEvent) bool {
			return stoppedIds[event.wrapper.sp.getId()]
		})
		//put back different events that remained after filtering
		for _, e := range eventsAfterStop {
			e.wrapper.restartChan <- e
		}

		var wrappers []*specWrapper
		for _, sw := range childrenToRestart {
			wrapper := newSpecWrapper(ctx, sw.sp, sw.restartChan)
			wrappers = append(wrappers, wrapper)
			e.wrapper.siblings[wrapper.sp.getIndex()] = wrapper
		}

		for _, w := range wrappers {
			w.siblings = e.wrapper.siblings
			s.logger.Write([]byte(fmt.Sprintf("supervisor: %s, running %s, id: %s\n", s.id, w.sp.getName(), w.sp.getId())))
			w.sp.setState(stateRunning)
			go func(w *specWrapper) {
				defer close(w.done)
				s.startSpec(w)
			}(w)
		}
	}
)
