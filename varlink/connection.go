package varlink

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strings"
)

const ResolverAddress = "unix:/run/org.varlink.resolver"

type Connection interface {
	SendMessage(message interface{}) error
	ReceiveMessage(reply interface{}) error
	Call(method string, args interface{}, reply interface{}) error
	GetInterface(name string) (*Interface, error)
	Close() error
}

type connection struct {
	conn net.Conn
}

type CallArgs struct {
	Method     string      `json:"method"`
	Parameters interface{} `json:"parameters,omitempty"`
	More       bool        `json:"more,omitempty"`
}

type CallReply struct {
	Parameters *json.RawMessage `json:"parameters,omitempty"`
	Error      *string          `json:"error,omitempty"`
	Continues  bool             `json:"continues,omitempty"`
}

type Error struct {
	Name       string
}

func (e *Error) Error() string {
	return e.Name
}

func (c *connection) SendMessage(message interface{}) error {
	err := json.NewEncoder(c.conn).Encode(message)
	if err != nil {
		return err
	}

	_, err = c.conn.Write([]byte{0})
	return err
}

func (c *connection) ReceiveMessage(reply interface{}) error {
	out, err := bufio.NewReader(c.conn).ReadBytes(0)
	if err != nil {
		return errors.New("error reading from connection: " + err.Error())
	}

	return json.Unmarshal(out[:len(out)-1], reply)
}

func (c *connection) Call(method string, parameters, reply_parameters interface{}) error {
	if parameters == nil {
		var empty struct {}
		parameters = empty
	}

	call := CallArgs{
		Method:     method,
		Parameters: parameters,
	}

	err := c.SendMessage(&call)
	if err != nil {
		return err
	}

	var r CallReply
	err = c.ReceiveMessage(&r)
	if err != nil {
		return err
	}

	if r.Error != nil {
		return &Error{*r.Error}
	}

	err = json.Unmarshal(*r.Parameters, reply_parameters)
	if err != nil {
		return err
	}

	return nil
}

func (c *connection) GetInfo() (*Service, error) {
	var service Service
	err := c.Call("org.varlink.service.GetInfo", nil, &service)
	if err != nil {
		return nil, err
	}

	return &service, nil
}

func (c *connection) GetInterface(name string) (*Interface, error) {
	type GetInterfaceArgs struct {
		Name string `json:"name"`
	}
	type GetInterfaceReply struct {
		InterfaceString string `json:"interfacestring"`
	}

	var reply GetInterfaceReply
	err := c.Call("org.varlink.service.GetInterface", GetInterfaceArgs{name}, &reply)
	if err != nil {
		return nil, err
	}

	iface := NewInterface(reply.InterfaceString)
	if iface == nil {
		return nil, errors.New("Received invalid interface")
	}

	return iface, nil
}

func (c *connection) Close() error {
	return c.conn.Close()
}

func Dial(address string) (Connection, error) {
	var err error

	parts := strings.Split(address, ":")
	if len(parts) != 2 {
		return nil, errors.New("invalid address: " + address)
	}

	transport := parts[0]
	path := parts[1]

	if transport != "unix" {
		return nil, errors.New("unsupported transport: " + transport)
	}

	c := &connection{}

	c.conn, err = net.Dial(transport, path)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func DialInterface(iface string) (Connection, error) {
	address, err := Resolve(iface)
	if err != nil {
		return nil, err
	}

	return Dial(address)
}

func Resolve(iface string) (string, error) {
	type ResolveArgs struct {
		Interface string `json:"interface"`
	}
	type ResolveReplyArgs struct {
		Address string `json:"address"`
	}

	/* don't ask the resolver for itself */
	if iface == "org.varlink.resolver" {
		return ResolverAddress, nil
	}

	connection, err := Dial(ResolverAddress)
	if err != nil {
		return "", err
	}
	defer connection.Close()

	var reply ResolveReplyArgs
	err = connection.Call("org.varlink.resolver.Resolve", &ResolveArgs{iface}, &reply)
	if err != nil {
		return "", err
	}

	return reply.Address, nil
}

func Call(method string, parameters, reply_parameters interface{}) error {
	parts := strings.Split(method, ".")
	iface := strings.TrimSuffix(method, "."+parts[len(parts)-1])
	address, err := Resolve(iface)
	if err != nil {
		return err
	}

	connection, err := Dial(address)
	if err != nil {
		return err
	}
	defer connection.Close()

	return connection.Call(method, parameters, reply_parameters)
}
