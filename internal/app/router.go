package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"runtime/debug"
)

// Router is the one service the desktop shell binds. Every frontend call to
// an App method arrives as Router.Call with the method's name, and goes to
// the App of the window that made the call, never to another window's.
//
// It keeps the behaviour of binding *App directly: the same argument checks
// and messages, App errors returned unchanged, results returned as Wails
// would, and panics turned into errors.
type Router struct {
	windows *Windows
	// window names the window that made a call, from its context; "" when
	// the call came from no window.
	window func(ctx context.Context) string
}

// NewRouter routes calls to the Apps of windows. window returns the name of
// the window that made a call; the shell reads it from the toolkit's call
// context.
func NewRouter(windows *Windows, window func(ctx context.Context) string) *Router {
	return &Router{windows: windows, window: window}
}

// Call runs App method on the calling window's App with the JSON-encoded
// args. It is the only method the frontend can call.
func (r *Router) Call(ctx context.Context, method string, args []json.RawMessage) (any, error) {
	m, ok := appMethods[method]
	if !ok {
		return nil, fmt.Errorf("unknown App method %q", method)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	name := ""
	if r.window != nil {
		name = r.window(ctx)
	}
	if name == "" {
		return nil, fmt.Errorf("App.%s: the call came from no window", method)
	}
	a, err := r.windows.app(ctx, name)
	if err != nil {
		return nil, err
	}
	return m.call(ctx, reflect.ValueOf(a), args)
}

// routedMethod is an exported *App method the frontend may call.
type routedMethod struct {
	name         string
	fn           reflect.Value // the method expression: receiver first
	in           []reflect.Type
	needsContext bool
	variadic     bool
}

// routerSkips are the methods Wails never binds (its internalServiceMethods).
var routerSkips = map[string]bool{
	"ServiceName":     true,
	"ServiceStartup":  true,
	"ServiceShutdown": true,
	"ServeHTTP":       true,
}

var (
	contextType = reflect.TypeFor[context.Context]()
	errorType   = reflect.TypeFor[error]()
)

// appMethods are the *App methods the frontend may call, by name.
var appMethods = routedMethods(reflect.TypeFor[*App]())

// routedMethods are the methods of typ that Wails would bind, by name.
func routedMethods(typ reflect.Type) map[string]*routedMethod {
	out := map[string]*routedMethod{}
	for i := range typ.NumMethod() {
		def := typ.Method(i)
		if routerSkips[def.Name] {
			continue
		}
		ft := def.Type
		m := &routedMethod{name: def.Name, fn: def.Func, variadic: ft.IsVariadic()}
		for j := 1; j < ft.NumIn(); j++ {
			in := ft.In(j)
			if j == 1 && in.AssignableTo(contextType) {
				m.needsContext = true
			}
			m.in = append(m.in, in)
		}
		out[def.Name] = m
	}
	return out
}

// call runs the method on recv. It matches Wails' BoundMethod.Call so the
// frontend sees the same results and errors as when *App was bound.
func (m *routedMethod) call(ctx context.Context, recv reflect.Value, args []json.RawMessage) (result any, err error) {
	defer func() {
		p := recover()
		if p == nil {
			return
		}
		perr, ok := p.(error)
		if !ok {
			perr = fmt.Errorf("%v", p)
		}
		log.Printf("agent-sessions: panic in App.%s: %v\n%s", m.name, perr, debug.Stack())
		result = nil
		err = fmt.Errorf("App.%s: panic: %s", m.name, perr)
	}()

	argCount := len(args)
	if m.needsContext {
		argCount++
	}
	if argCount != len(m.in) {
		return nil, fmt.Errorf("App.%s expects %d arguments, got %d", m.name, len(m.in), argCount)
	}

	callArgs := make([]reflect.Value, 0, 1+argCount)
	callArgs = append(callArgs, recv)
	base := 0
	if m.needsContext {
		callArgs = append(callArgs, reflect.ValueOf(ctx))
		base = 1
	}
	for i, arg := range args {
		v := reflect.New(m.in[base+i])
		if err := json.Unmarshal(arg, v.Interface()); err != nil {
			return nil, fmt.Errorf("could not parse argument #%d: %s", i, err)
		}
		callArgs = append(callArgs, v.Elem())
	}

	var out []reflect.Value
	if m.variadic {
		out = m.fn.CallSlice(callArgs)
	} else {
		out = m.fn.Call(callArgs)
	}

	var values []any
	var errs []error
	for _, v := range out {
		if v.Type() == errorType {
			if !v.IsNil() {
				errs = append(errs, v.Interface().(error))
			}
			continue
		}
		values = append(values, v.Interface())
	}
	switch {
	case len(errs) == 1:
		return nil, errs[0]
	case len(errs) > 1:
		return nil, errors.Join(errs...)
	case len(values) == 1:
		return values[0], nil
	case len(values) > 1:
		return values, nil
	}
	return nil, nil
}
