package actor

import (
    "fmt"
    "time"
    "net/http"
    "io"
    "encoding/base64"
)

// bridge.go implements the Go functions called by JS polyfills.

func (a *Actor) registerBridge() error {
    var err error

    // Timer Bridge
    err = a.vm.RegisterFunc("_go_setTimeout", func(id int, delay int) {
        a.timersLock.Lock()
        defer a.timersLock.Unlock()

        timer := time.AfterFunc(time.Duration(delay)*time.Millisecond, func() {
            a.Send(TimerEvent{Id: id, IsInterval: false})
        })
        a.timers[id] = timer
    }, false)
    if err != nil { return err }

    err = a.vm.RegisterFunc("_go_clearTimeout", func(id int) {
        a.timersLock.Lock()
        defer a.timersLock.Unlock()
        if t, ok := a.timers[id]; ok {
             if timer, ok := t.(*time.Timer); ok {
                 timer.Stop()
             }
            delete(a.timers, id)
        }
    }, false)
    if err != nil { return err }

    err = a.vm.RegisterFunc("_go_setInterval", func(id int, delay int) {
        a.timersLock.Lock()
        defer a.timersLock.Unlock()

        // Using Ticker
        ticker := time.NewTicker(time.Duration(delay)*time.Millisecond)
        go func() {
            for range ticker.C {
                a.Send(TimerEvent{Id: id, IsInterval: true})
            }
        }()

        a.timers[id] = ticker
    }, false)
    if err != nil { return err }

     err = a.vm.RegisterFunc("_go_clearInterval", func(id int) {
        a.timersLock.Lock()
        defer a.timersLock.Unlock()
        if t, ok := a.timers[id]; ok {
            if ticker, ok := t.(*time.Ticker); ok {
                ticker.Stop()
            }
            delete(a.timers, id)
        }
    }, false)
    if err != nil { return err }

    // Fetch Bridge
    err = a.vm.RegisterFunc("_go_fetch", func(url string, optsStr string, resolveId, rejectId int) {
        // Fetch is globally allowed in this simplified architecture for now
        // In real policy, we would check capabilities[].

        go func() {
            // Basic HTTP Get
            resp, err := http.Get(url)
            if err != nil {
                a.Send(CallbackEvent{Id: rejectId, Args: []any{err.Error()}})
                return
            }
            defer resp.Body.Close()

            body, _ := io.ReadAll(resp.Body)
            a.Send(CallbackEvent{Id: resolveId, Args: []any{string(body)}})
        }()
    }, false)
    if err != nil { return err }

    // Console
    err = a.vm.RegisterFunc("_go_print", func(level string, args ...any) {
        // Format args
        fmt.Printf("[%s] [%s] %v\n", a.Id, level, args)
    }, false)
    if err != nil { return err }

    // Base64
    err = a.vm.RegisterFunc("_go_btoa", func(str string) string {
        return base64.StdEncoding.EncodeToString([]byte(str))
    }, false)
    if err != nil { return err }

    err = a.vm.RegisterFunc("_go_atob", func(str string) (string, error) {
        b, err := base64.StdEncoding.DecodeString(str)
        if err != nil { return "", err }
        return string(b), nil
    }, false)
    if err != nil { return err }

    return nil
}
