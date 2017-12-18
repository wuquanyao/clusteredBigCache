package comms

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

var (
	errConnectionUnusable = errors.New("connection is not usable")
	errTimeout            = errors.New("i/o timeout")
)

//defines a connection to a remote peer
type Connection struct {
	Remote      string
	Uid         string
	conn        *net.TCPConn
	buffReader  *bufio.Reader
	readTimeout time.Duration
	Usable      bool
	writeLock   sync.Mutex
}

//Create a new tcp connection .
//Connect to the remote entity
func NewConnection(endpoint string, connectionTimeout time.Duration) (*Connection, error) {

	c := &Connection{}
	conn, err := net.DialTimeout("tcp", endpoint, connectionTimeout)
	if err != nil {
		return nil, err
	}

	c.conn = conn.(*net.TCPConn)

	c.Uid = c.conn.LocalAddr().String()
	c.Remote = endpoint
	c.conn.SetKeepAlive(true)
	c.buffReader = bufio.NewReader(c)
	c.Usable = true
	c.writeLock = sync.Mutex{}

	return c, nil
}

func WrapConnection(conn *net.TCPConn) *Connection {

	c := &Connection{}
	c.conn = conn
	c.Uid = c.conn.LocalAddr().String()
	c.Remote = conn.RemoteAddr().String()
	c.conn.SetKeepAlive(true)
	c.buffReader = bufio.NewReader(c)
	c.Usable = true
	c.writeLock = sync.Mutex{}

	return c
}

func (c *Connection) SetReadTimeout(timeout time.Duration) {
	c.readTimeout = timeout
}

func (c *Connection) Read(p []byte) (int, error) {
	if !c.Usable {
		return 0, errConnectionUnusable
	}

	if c.readTimeout != 0 {
		c.conn.SetReadDeadline(time.Now().Add(c.readTimeout))
	}

	n, err := c.conn.Read(p)
	if err != nil {
		if err == io.EOF {
			c.Usable = false
		} else {
			if strings.Contains(err.Error(), "timeout") {
				err = errors.New("i/o timeout")
			}
		}
	}

	return n, err
}

func (c *Connection) Write(data []byte) (int, error)  {
	if !c.Usable {
		return 0, errConnectionUnusable
	}

	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	count := 0
	size := len(data)
	for count < size {
		n, err := c.conn.Write(data[count:])
		if err != nil {
			if err == io.EOF {
				c.Usable = false
			}
			return count, err
		}

		count += n
	}

	return count, nil
}

//Send a []byte over the network
func (c *Connection) SendData(data []byte) error {

	if !c.Usable {
		return errConnectionUnusable
	}

	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	count := 0
	size := len(data)
	for count < size {
		n, err := c.conn.Write(data[count:])
		if err != nil {
			if err == io.EOF {
				c.Usable = false
			}
			return err
		}

		count += n
	}

	return nil
}

//Read size byte of data and return is to the caller
func (c *Connection) ReadData(size uint, timeout time.Duration) ([]byte, error) {

	ret := make([]byte, size)
	var err error

	tmp := c.readTimeout
	c.SetReadTimeout(timeout)
	defer c.SetReadTimeout(tmp)

	if 0 != timeout {
		done := make(chan bool)
		defer close(done)

		go func() {
			_, err = io.ReadFull(c.buffReader, ret)
			done <- true
		}()

		select {
		case <-done:
			return ret, err
		case <-time.After(timeout):
			return nil, errTimeout
		}
	} else {
		_, err = io.ReadFull(c.buffReader, ret)
		return ret, err
	}
}

func (c *Connection) Close() {
	c.Shutdown()
}

func (c *Connection) Shutdown() {
	c.conn.Close()
	c.buffReader = nil
	c.Usable = false
}
