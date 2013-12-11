// Package dnode implements a message processor for communication
// via dnode protocol. See the following URL for details:
// https://github.com/substack/dnode-protocol/blob/master/doc/protocol.markdown
package dnode

import (
	"io/ioutil"
	"log"
	"reflect"
)

var l *log.Logger = log.New(ioutil.Discard, "", log.Lshortfile)

// Uncomment following to see log messages.
// var l *log.Logger = log.New(os.Stderr, "", log.Lshortfile)

type Dnode struct {
	// Registered methods are saved in this map.
	handlers map[string]reflect.Value

	// Reference to sent callbacks are saved in this map.
	callbacks map[uint64]reflect.Value

	// Next callback number.
	// Incremented atomically by registerCallback().
	seq uint64

	// For sending and receiving messages
	transport Transport

	// Argument wrappers to be called when sending/receiving.
	WrapMethodArgs   Wrapper
	WrapCallbackArgs Wrapper

	// Dnode message processors.
	RunMethod   Runner
	RunCallback Runner
}

type Wrapper func(args interface{}, tr Transport) []interface{}
type Runner func(method string, handlerFunc reflect.Value, args *Partial, tr Transport)

// Transport is an interface for sending and receiving data on network.
// Each Transport must be unique for each Client.
type Transport interface {
	// Address of the connected client
	RemoteAddr() string

	// Send single message
	Send(msg []byte) error

	// Receive single message
	Receive() ([]byte, error)

	// A place to save/read extra information about the client
	Properties() map[string]interface{}
}

// Message is the JSON object to call a method at the other side.
type Message struct {
	// Method can be an integer or string.
	Method interface{} `json:"method"`

	// Array of arguments
	Arguments *Partial `json:"arguments"`

	// Integer map of callback paths in arguments
	Callbacks map[string]Path `json:"callbacks"`

	// Links are not used for now.
	Links []interface{} `json:"links"`
}

// New returns a pointer to a new Dnode.
func New(transport Transport) *Dnode {
	return &Dnode{
		handlers:  make(map[string]reflect.Value),
		callbacks: make(map[uint64]reflect.Value),
		transport: transport,
	}
}

// Copy returns a pointer to a new Dnode with the same handlers as d but empty callbacks.
func (d *Dnode) Copy(transport Transport) *Dnode {
	return &Dnode{
		handlers:         d.handlers,
		callbacks:        make(map[uint64]reflect.Value),
		transport:        transport,
		WrapMethodArgs:   d.WrapMethodArgs,
		WrapCallbackArgs: d.WrapCallbackArgs,
		RunMethod:        d.RunMethod,
		RunCallback:      d.RunCallback,
	}
}

// HandleFunc registers the handler for the given method.
// If a handler already exists for method, HandleFunc panics.
func (d *Dnode) HandleFunc(method string, handler interface{}) {
	if method == "" {
		panic("dnode: invalid method " + method)
	}
	if handler == nil {
		panic("dnode: nil handler")
	}
	if _, ok := d.handlers[method]; ok {
		panic("dnode: handler already exists for method")
	}
	val := reflect.ValueOf(handler)
	if val.Kind() != reflect.Func {
		panic("dnode: handler must be a func")
	}

	d.handlers[method] = val
}

// Run processes incoming messages. Blocking.
func (d *Dnode) Run() error {
	for {
		msg, err := d.transport.Receive()
		if err != nil {
			return err
		}

		// Do not run this function in a separate goroutine,
		// otherwise the order of the messages may be mixed.
		// If the order of the messages is not important for the user of
		// this package he can choose to start separate gorouitens in his own
		// handler. However, if we make this decision here and start a goroutine
		// for each message the user cannot change this behavior in his handler.
		// This is very important in Kites such as Terminal because the order
		// of the key presses must be preserved.
		d.processMessage(msg)
	}
}

// RemoveCallback removes the callback with id from callbacks.
// Can be used to remove unused callbacks to free memory.
func (d *Dnode) RemoveCallback(id uint64) {
	delete(d.callbacks, id)
}
