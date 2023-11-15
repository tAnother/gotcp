# TCP

- **Major design decisions**
- **TCP traffic**, as follows: \
    measure your implementation’s performance relative to the reference node and comment on it in your README.
    To get a baseline for performance, run two reference nodes connected directly to each other with no packet loss and compare the time to send a file of a few megabytes in size (you can also directly measure the throughput in Wireshark). Your implementation should have performance on the same order of magnitude as the reference under the same test conditions.
- **Packet capture**: 
    - Requirements: a **1 megabyte** file transmission between two of your nodes. To do this, run two of your nodes in the ABC network with the lossy node in the middle, configured with a **2%** drop rate.
        - The 3-way handshake
        - One example segment sent and acknowledged
        - One segment that is retransmitted
        - Connection teardown

## Design 

### Directory Structure

```
├── cmd                         -- source code for executables
│   ├── vhost
│   ├── vrouter
├── pkg
│   ├── proto
│   │   ├── ippacket.go
│   │   ├── tcppacket.go
│   │   ├── rip.go
│   ├── ipnode
│   │   ├── node.go             -- shared structs & functions among routers & hosts
│   │   ├── node_print.go
│   │   ├── node_subr.go
│   │   ├── ip_repl.go
│   ├── tcpstack
│   │   ├── tcpstack.go         -- shared structs & functions for tcpstack
│   │   ├── conn.go             -- normal socket related functions
│   │   ├── listener.go         -- listener socket related functions
│   │   ├── state.go            -- state machine
│   │   ├── tcb.go              -- tcb info
│   │   ├── tcp_repl.go         
│   │   ├── tcp_print.go  
│   │   ├── tcp_internal.go 
│   │   ├── cirbuff.go      
│   │   ├── utils.go  
│   ├── repl
│   │   ├── repl.go
│   ├── vhost
│   │   ├── vhost.go
│   ├── vrouter
│   │   ├── vrouter.go
│   │   ├── utils.go
│   ├── lnxconfig               -- parser for .lnx file
├── util
│   ├── rip-dissector           -- for wireshark to decode messages in RIP protocols
├── reference                   -- reference programs
├── net                         -- network topologies
└── Makefile
```

### pkg: tcpstack

#### tcpstack.go

It contains how structs are defined under tcp stack and APIs.

```Go

type TCPEndpointID struct{
    localAddr  netip.Addr
    localPort  uint16
    remoteAddr netip.Addr
    remotePort uint16
}


type TCPGlobalInfo struct {
	IP        *ipstack.IPGlobalInfo
	localAddr netip.Addr

	listenerTable map[uint16]*VTCPListener // key: port num
	connTable     map[TCPEndpointID]*VTCPConn
	tableMu       sync.RWMutex // TODO: make it finer grained?

	socketNum int32 // a counter that keeps track of the most recent socket id

	// TODO: add extra maps for faster querying by socket id?
}

type VTCPListener struct {
	t *TCPGlobalInfo // a pointer to the tcp global state struct

	socketId       int32
	port           uint16
	pendingSocketC chan struct {
		*proto.TCPPacket
		netip.Addr
	}
}

type VTCPConn struct { // represents a TCP socket
	t *TCPGlobalInfo // a pointer to the tcp global state struct

	TCPEndpointID
	socketId int32

	state   State
	stateMu sync.RWMutex

	srtt             *SRTT
	localInitSeqNum  uint32         // ISS (unsafe - should stay unchanged once initialized)
	remoteInitSeqNum uint32         // IRS (unsafe - should stay unchanged once the connection is established)
	seqNum           *atomic.Uint32 // SND.NXT - next seq num to use
	largestAck       *atomic.Uint32 // SND.UNA - the largest ACK we received
	expectedSeqNum   *atomic.Uint32 // the ACK num we should send to the other side
	windowSize       *atomic.Int32  // RCV.WND - range 0 ~ 65535

	sendBuf *sendBuf
	recvBuf *CircBuff

	recvChan chan *proto.TCPPacket // for receiving tcp packets dispatched to this connection
	closeC   chan struct{}         // for closing // TODO: or also for other user input...?
}


func Init(addr netip.Addr, host *vhost.VHost) 
func VListen(port uint16) (*VTCPListener, error)
func VConnect(addr netip.Addr, port int16) (VTCPConn, error)
func tcpRecvHandler(packet *proto.Packet, node *ipnode.Node)
```

#### listener.go

```Go
func NewListenerSocket(port uint16, addr netip.Addr) *VTCPListener 
func (*VTCPListener) VAccept() (*VTCPConn, error)
func (*VTCPListener) VClose() error
```

#### conn.go

```Go
func NewSocket(state State, endpoint TCPEndpointID, remoteInitSeqNum uint32) *VTCPConn

func (*VTCPConn) VRead(buf []byte) (int, error) // not for milestone I
func (*VTCPConn) VWrite(data []byte) (int, error) // not for milestone I
func (*VTCPConn) VClose() error // not for milestone I
```

#### state.go

It contains state machine which handles states under different situations.

```Go

func handleSynRecvd(conn *VTCPConn, endPoint TCPEndpointID) error
func handleSynSent(conn *VTCPConn, endpoint TCPEndpointID) error
//...etc
```

#### tcp_repl.go
 
This contains all cli handlers for tcp stack.


#### circBuff.go

```Go
type CircBuff struct {
	buff     []byte
	capacity uint32
	lbr      uint32
	nxt      uint32
	isFull   bool
	canRead  chan bool
	lock     sync.Mutex
}

func (cb *CircBuff) Read(buf []byte) (bytesRead uint32, err error) 
func (cb *CircBuff) Write(buf []byte) (bytesWritten uint32, err error)
```

### proto

#### tcppacket.go

```Go
type TCPPacket struct {
	TcpHeader *header.TCPFields
	Payload   []byte
}
```

## TCP Traffic

## Packet Capture