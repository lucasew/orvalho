package runtime

import (
    "fmt"
)

type TimerEvent struct {
    Id         int
    IsInterval bool
}

func (e TimerEvent) Process(a *Actor) error {
    _, err := a.vm.Eval(fmt.Sprintf("__invoke_callback(%d)", e.Id), 0)

    if !e.IsInterval {
         a.vm.Eval(fmt.Sprintf("__unregister_callback(%d)", e.Id), 0)

         a.timersLock.Lock()
         delete(a.timers, e.Id)
         a.timersLock.Unlock()
    }
    return err
}

type CallbackEvent struct {
    Id   int
    Args []any
}

func (e CallbackEvent) Process(a *Actor) error {
    _, err := a.vm.Call("__invoke_callback", append([]any{e.Id}, e.Args...)...)
    return err
}

func (a *Actor) invokeCallback(id int, args ...any) {
    a.Send(CallbackEvent{Id: id, Args: args})
}
