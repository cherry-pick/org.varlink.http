package varlink

import (
	"bufio"
	"encoding/json"
	"net"
	"strings"
)

// Message flags for Send(). More indicates that the client accepts more than one method
// reply to this call. Oneway requests, that the service must not send a method reply to
// this call. Continues indicates that the service will send more than one reply.
const (
	More      = 1 << iota
	Oneway    = 1 << iota
	Continues = 1 << iota
)

// Error is a varlink error returned from a method call.
type Error struct {
	Name       string
	Parameters interface{}
}

// Error returns the fully-qualified varlink error name.
func (e *Error) Error() string {
	return e.Name
}

// Connection is a connection from a client to a service.
type Connection struct {
	address string
	conn    net.Conn
	reader  *bufio.Reader
	writer  *bufio.Writer
}

// Send sends a method call. It returns a receive() function which is called to retrieve the method reply.
// If Send() is called with the `More`flag and the receive() function carries the `Continues` flag, receive()
// can be called multiple times to retrieve multiple replies.
func (c *Connection) Send(method string, parameters interface{}, flags uint64) (func(interface{}) (uint64, error), error) {
	type call struct {
		Method     string      `json:"method"`
		Parameters interface{} `json:"parameters,omitempty"`
		More       bool        `json:"more,omitempty"`
		Oneway     bool        `json:"oneway,omitempty"`
	}

	if (flags&More != 0) && (flags&Oneway != 0) {
		return nil, &Error{
			Name:       "org.varlink.InvalidParameter",
			Parameters: "oneway",
		}
	}

	m := call{
		Method:     method,
		Parameters: parameters,
		More:       flags&More != 0,
		Oneway:     flags&Oneway != 0,
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	b = append(b, 0)
	_, err = c.writer.Write(b)
	if err != nil {
		return nil, err
	}

	err = c.writer.Flush()
	if err != nil {
		return nil, err
	}

	receive := func(out_parameters interface{}) (uint64, error) {
		type reply struct {
			Parameters *json.RawMessage `json:"parameters"`
			Continues  bool             `json:"continues"`
			Error      string           `json:"error"`
		}

		out, err := c.reader.ReadBytes('\x00')
		if err != nil {
			return 0, err
		}

		var m reply
		err = json.Unmarshal(out[:len(out)-1], &m)
		if err != nil {
			return 0, err
		}

		if m.Error != "" {
			err = &Error{
				Name:       m.Error,
				Parameters: m.Parameters,
			}
			return 0, err
		}

		if m.Parameters != nil {
			json.Unmarshal(*m.Parameters, out_parameters)
		}

		if m.Continues {
			return Continues, nil
		}

		return 0, nil
	}

	return receive, nil
}

// Call sends a method call and returns the method reply.
func (c *Connection) Call(method string, parameters interface{}, out_parameters interface{}) error {
	receive, err := c.Send(method, &parameters, 0)
	if err != nil {
		return err
	}

	_, err = receive(out_parameters)
	return err
}

// GetInterfaceDescription requests the interface description string from the service.
func (c *Connection) GetInterfaceDescription(name string) (string, error) {
	type request struct {
		Interface string `json:"interface"`
	}
	type reply struct {
		Description string `json:"description"`
	}

	var r reply
	err := c.Call("org.varlink.service.GetInterfaceDescription", request{Interface: name}, &r)
	if err != nil {
		return "", err
	}

	return r.Description, nil
}

// GetInfo requests information about the service.
func (c *Connection) GetInfo(vendor *string, product *string, version *string, url *string, interfaces *[]string) error {
	type reply struct {
		Vendor     string   `json:"vendor"`
		Product    string   `json:"product"`
		Version    string   `json:"version"`
		URL        string   `json:"url"`
		Interfaces []string `json:"interfaces"`
	}

	var r reply
	err := c.Call("org.varlink.service.GetInfo", nil, &r)
	if err != nil {
		return err
	}

	if vendor != nil {
		*vendor = r.Vendor
	}
	if product != nil {
		*product = r.Product
	}
	if version != nil {
		*version = r.Version
	}
	if url != nil {
		*url = r.URL
	}
	if interfaces != nil {
		*interfaces = r.Interfaces
	}

	return nil
}

// Close terminates the connection.
func (c *Connection) Close() error {
	return c.conn.Close()
}

// NewConnection returns a new connection to the given address.
func NewConnection(address string) (*Connection, error) {
	var err error

	words := strings.SplitN(address, ":", 2)
	protocol := words[0]
	addr := words[1]

	// Ignore parameters after ';'
	words = strings.SplitN(addr, ";", 2)
	if words != nil {
		addr = words[0]
	}

	switch protocol {
	case "unix":
		break

	case "tcp":
		break
	}

	c := Connection{}
	c.conn, err = net.Dial(protocol, addr)
	if err != nil {
		return nil, err
	}

	c.address = address
	c.reader = bufio.NewReader(c.conn)
	c.writer = bufio.NewWriter(c.conn)

	return &c, nil
}
