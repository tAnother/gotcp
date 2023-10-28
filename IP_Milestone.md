### A few questions to consider

1. What are the different parts of your IP stack and what data structures do they use? How do these parts interact (API functions, channels, shared data, etc.)?

2. What fields in the IP packet are read to determine how to forward a packet?\
```Destination IP, TTL, ProtocolNum and subnet mask for rip```

3. What will you do with a packet destined for local delivery (ie, destination IP == your node’s IP)?\
` If the dest IP belongs to one of the node's interfaces, no need to forward/send, simply print out the recv msg? `
or 
`Send & recv normally?`

4. What structures will you use to store routing/forwarding information?\
`a hashmap. Key is the prefix; value is the routing entry (next hop and cost`

5. What happens when a link is disabled? (ie, how is forwarding affected)?\
`First, the forwarding table will get updated about the expired link (discard it). We can simply drop the packet if no other paths are available, or forward it via another available path.`

## Distance Vector Routing

If a link is down, the update from the neighbor will time out. Remove the corresponding entry from the routing table.

Router R receive an update $<D,C_D>$ from Neighbor N:\
R can reach D via N with Cost $C = C_D + C_N$

Table has format $<dest, cost, next\_hop>$:
1. If D isn't in the table, add it $<D,C, N>$
2. If D is in the table $<D, C_{old}, M>$\
2.1 If $c < c_{old}$ &rarr; update $<D,C,N>$\
2.2 If $c >= c_{old}$ and $N == M$ &rarr;
    Topology changed, route has a higher cost &rarr;update table $<D,C,N>$\
2.3 If $c >= c_{old}$ and $N != M$ &rarr; Ignore\
2.4 If $c == c_{old}$ and $N == M$ &rarr; refresh timeout


<br>


## Directory Structure
```
├── cmd                         -- source code for executables
│   ├── vhost
│   ├── vrouter
├── pkg
│   ├── proto
│   │   ├── ip.go
│   │   ├── rip.go
│   ├── ipnode
│   │   ├── node.go             -- shared structs & functions among routers & hosts
│   │   ├── router.go           -- router specific functions
│   │   ├── host.go
│   ├── repl
│   │   ├── repl.go
│   ├── lnxconfig               -- parser for .lnx file
├── util
│   ├── rip-dissector           -- for wireshark to decode messages in RIP protocols
├── reference                   -- reference programs
├── net                         -- network topologies
└── Makefile
```

## cmd: the control flow
0. parse command line args, get IPConfig

1. init from IPConfig (the decoded .lnx info), determine whether it is a router or host, init repl config 
    - fill in the node structure
    - create a handler for each REPL command

2. spawn several go routines to listen on each interface
    - upon receiving test packet, print it out
    - upon receiving RIP msg, call its handler

3. a go routine to send out RIP to neighbors every 5 sec

4. a go routine to update the routing table every 12 sec

5. main routine runs the REPL (`li, ln, lr, up, down, send`) and handle API calls (RIP & TCP)


## pkg: structs & methods

### node.go
```go
type Node struct {
    NodeType        NodeType     // is it a router or host
    Interfaces      map[string]*Interface   // interface name -> interface instance
    IFNeighbors     map[string][]*Neighbor  // interface name -> a list of neighbors on that interface
    RoutingTable    map[netip.Prefix]*RoutingEntry // aka forwarding table
    RoutingTableMu  *sync.mutex

    RecvHandlers    map[int]HandlerFunc
    RecvHandlersMu  *sync.mutex
    IFNeighborsMu *sync.RWMutex
    InterfacesMu  *sync.RWMutex
}

```
The routing table of a host should look like:

| Prefix | Next Hop | Cost | RouteType|
| -- | -- | -- |--|
| (subnet this node belongs to, e.g. 10.0.1.0/24)  | IF0 | 0 | Local|
| 0.0.0.0/0 | (VIP of IF0) | 0 | Static|


```go
type Neighbor struct {
    VIP         netip.Addr    // virtual ip addr
    UDPAddr     netip.AddrPort 
    IFName      string
}

type Interface struct {
    Name            string
    AssignedIP      netip.Addr
    AssignedPrefix  netip.Prefix
    SubnetMask      uint32
    UDPAddr         netip.AddrPort
    IsDown          bool
    Conn           *net.UDPConn
}

type RoutingEntry struct{
    RouteType       uint16
    NextHop         netip.Addr // nil if local
    LocalNextHop    string     // interface name. nil if not local
    Cost            uint16
}


// ------------ IP API ------------

// init a node instance & register handlers
func Init(config lnxconfig.IPConfig) (*Node, error)   


// listen on designated interface, handle receiving & unmarshalling packet
func (n *Node) ListenOn(udpPort uint16) 



type RecvHandlerFunc func([]byte)  error   // params TBD
func (n *Node) RegisterRecvHandler(protoNum uint16, callbackFunc HandlerFunc)
// register(0, testPacketHandler) -- testPacketHandler should be shared, but prob don't put it in node.go
func testPacketHandler([]byte) error
// register (200, ripHandler)
func ripHandler([]byte) error

// send:
// 1. Marshal data
// 2. Recursively find next hop in the routing table
// (2.5)  (prob need a way to find the corresponding udp port/interface)
// 3. Write to the dest UDP port 
func (n *Node) Send(destIP netip.Addr, data []byte, protoNum uint16)
func findNextHop(destIP netip.Addr)
```

**A bit about finding the next hop:**

Let's say host 1 is trying to send to 10.0.0.2

| Prefix | Next Hop | Cost | RouteType|
| -- | -- | -- |--|
| 10.0.0.0/24  | IF0 | 0 | Local|
| 0.0.0.0/0 | 10.0.0.1 (VIP of R1) | 0 | Static|

next hop = IF0. From there, look up 10.0.0.2 in IF0's neighbor list.

If we're sending to 10.1.0.3, we need to instead look up **10.0.0.1** in IF0's neighbor list.


```go
// Set interface state should:
// 1. update routing table 
// 2. notify rip neighboring interfaces (updateRoutingtable)
func (n *Node) SetInterfaceIsDown(state)
```

### host.go

```Go
type Host struct {
    Node      Node
    MsgChan   chan string
}
```

### router.go

```Go
type Router struct {
    Node            Node
    RoutingType     uint8    // might make it an enum val
    MsgChan         chan string
}



// Routing Info related stuff
func SendUpdate() // send update to the neighbors every 5 sec
func UpdateTable()

// and some function to convert proto.RIPMsg into RoutingEntry's...
```

### ip.go

We can use <github.com/brown-csci1680/iptcp-headers/blob/main/ipv4header.go>

```Go
constant (
    MTU uint16 = 1400  // maximum-transmission-unit, default 1400 bytes
    RIPProtoNum int = 200
    TestProtoNum int = 0
)

// type IPv4Header struct {
//     Version  int         // protocol version
//     Len      int         // header length
//     TOS      int         // type-of-service
//     TotalLen int         // packet total length
//     ID       int         // identification
//     Flags    HeaderFlags // flags
//     FragOff  int         // fragment offset
//     TTL      int         // time-to-live
//     Protocol int         // next protocol
//     Checksum int         // checksum
//     Src      netip.Addr  // source address
//     Dst      netip.Addr  // destination address
//     Options  []byte      // options, extension headers
// }

func ValidateCheckSum(headerBytes []byte, checksumFromHeader uint16) uint16
func ValidateTTL(TTL int)
//func ComputeCheckSum(b []byte) uint16
//func (h *IPv4Header) Marshal() ([]byte, error)
//func (h *IPv4Header) UnMarshal(data []byte) error
```

### rip.go
```go
type RoutingCmdType uint16
const (
    RoutingRequest RoutingCmdType = 1   // for request of routing info
    RoutingResponse RoutingCmdType = 2
)

type ripEntry struct{
    Cost    uint32      // <= 16. we define INFINITY to be 16
    Address uint32      // IP addr
    Mask    uint32      // subnet mask
}

type RIPMsg struct {
    Command     RoutingCmdType
    NumEntries  uint16 // <= 64 (and must be 0 for a request command)
    Entries     []*ripEntry
}

func (m *RIPMsg) Marshal() ([]byte, error)
func (m *RIPMsg) UnMarshal(data []byte) error
```

## repl.go
```Go

type replHandlerFunc func(string, *REPLConfig) error

type REPL struct{
    commands map[string]replHandlerFunc
    help     map[string]string
}  

type REPLConfig struct{
    writer  io.Writer
    node    *ipnode.Node
}

func (r *REPL) RegisterREPLHandler(command string, handler replHandlerFunc, helpstring string)

func (r *REPL) Start(n *Node)


// some shared handler
func lnHandler(input string, replConfig *REPLConfig)
func liHandler(input string, replConfig *REPLConfig)
func lrHandler(input string, replConfig *REPLConfig)
func upHandler(input string, replConfig *REPLConfig)
func downHandler(input string, replConfig *REPLConfig)
func sendHandler(input string, replConfig *REPLConfig)
```

## Development Phase

### Phase 1: Test Packet

1. Initizalition of the network\
1.1 Parse program arguments
    - `vhost --config <lnx_file>` : initialize host nodes &rarr; `Init(lnxFile)`
    - `vrouter --config <lnx_file>`: initialize router nodes &rarr; `Init(lnxFile)`

    1.2 Generate and register handlers for each REPL and Packet &rarr; `REPLHandlerFunc, RecvHandlerFunc, Node.RegisterREPLHandler(replType, callbackFunc), Node.RegisterRecvHandler(protocolNum, callbackFunc)`

2. REPL Parse command line\
2.1 `li`: list all of the interfaces related to this node &rarr; `lnHandler(n, replConfig) -> n.Interfaces()`\
2.2 `ln`: list all of the neighbors related to this node &rarr; `liHandler(n, replConfig) -> n.Neighbors()`\
2.3 `lr`: list the routing table of this node &rarr; `lrHandler(n, replConfig) -> n.RoutingTable()`\
2.4 `up <interface_name>`: sets the interface to up &rarr; `upHandler(n, replConfig) -> n.SetInterfaceIsDown(0)`\
2.5 `down <interface_name>`: sets the interface to down &rarr; `downHandler(n, replConfig) -> n.SetInterfaceIsDown(1)`\
2.6 `send <dest_ip> <message>`: sends the test packet to the dest ip &rarr; `n.Send(destIP, data, 0)`
    - If is test packet: (test agaist **linear-r1h2** and **linear-r1h4**)
        - Marshal data
        - Consult `node.RoutingTable` to find the matched `prefix` (either exactly matched or longest matched) of `destIP`
            - If local, fetch the corresponding `RoutingEntry` and get the `routingEntry.localNextHop`
            - If not local, fetch the corresponding `RoutingEntry`. Consult the `RoutingTable` again by `routingEntry.NextHop`. By this time, the `routingEntry.localNextHop` should have a local interface name.
        - Get the `interface` from `node.interfaces` by `routingEntry.localNextHop`.
        - Lookup the udp port from `interface.Neighbors` where `destIP == neighbor.VIP`.
       
    - If is RIP:
        - handle this case in Phase 2
    
3. Receive Packet
    - If is test packet:
        - Fetch `func testPacketHandler` from `n.RecvHandlers`
        - Trigger `testPacketHandler()`
            - Unmarshall
            - Validation
            - Write received message to stdout: `Received test packet: Src: {src}, Dst: {dest}, TTL:{TTL}, Data:{msg}`
    - If is RIP:
        - handle this case in Phase 2

### Phase 2: RIP

See this [post](https://edstem.org/us/courses/45889/discussion/3629449) and [this](https://edstem.org/us/courses/45889/discussion/3629059) for encapsulating RIP msg.

1. Send Update Spec

    See this [post](https://edstem.org/us/courses/45889/discussion/3630914) for details.

```Go
// Sends periodic RIP updates to the neighbors every 5 sec
func (n *Node) SendPeriodicUpdate(){ //should have a thread for each node
   // 1. sendRipReqeust
   node.senRipRequest()
   // 2. sendUpdateToNeighbors on startup
   node.sendUpdateToNeighbors()
   // 3. Periodic sendUpdateToNeighbors
   ticker := time.NewTicker(util.RIP_COOLDOWN)
   defer ticker.Stop()
   for <- ticker.C:{
        node.sendUpdateToNeighbors()
    }
} 

func (n *Node) sendRipRequest(){
   // for each interface [i] of the router
        // generate RIP msg according to the routing table
        ripMsg := n.routingTableToRipMsg(RoutingCmdTypeRequest)
        // Marshal RIP msg to []bytes
        bytesToSend, err := ripMsg.Marshal()
        // Send the packet to the interface's neighbors [n]
            n.Send(destIP, msg, 200) // we might need to change msg type from string to byte
}

func (n *Node) sendUpdateToNeighbors(){
    // 1. get ripmsg from the node
    ripMsg := n.routingTableToRipMsg(RoutingCmdTypeResponse)
        // 2. for each rip entry, update the cost using split horizon and poison reverse
            // if rt.NextHop is one of the neighbors and the cost is not 0
                // It means this node does not have better knowledge about the destatination than the neighboring node. So we should set the corresponding ripEntry's cost to infinity 
        // 3. Marshal the ripMsg
        // 4. Wrap it to the packet
        // 5. Send the packet to the neighbor interface
}

func (n *Node) routingTableToRipMsg(commandType RoutingCmdType) *RIPMsg{}

//should be triggered whenever a interface's state is chanegd
func (n *Node) SendTriggeredUpdate(updatedEntry []ripEntry){} //should be similar to sendUpdateToNeighbors. 
```

2. Expire routing entry

    We should another field for `RoutingEntry`  (maybe `LastRefreshTime time.Time`). 
    
    Check out this [post](https://edstem.org/us/courses/45889/discussion/3634129) for details.

    It's sufficient to have a thread that checks the whole routing table every N seconds (where N < 12), looks for entries that have expired, and then removes the routes and sends a triggered update if necessary.
    
    If it expires, the routing entry should be deleted from the routing table, and we need to call `SendTriggeredUpdate`.

    ```Go
    func (n *Node) CheckExpired(){
        //for each routing entry in the routing table
            //check if CurrentTime - LastRefreshTime >= 12
            //if yes, remove the routing entry from the table and generate ripEntry with Cost = Infinity
            // send triggered update
    }
    ```

<!-- 3. Send RIP packet 

    If the packet is not for this node: `packet.Header.Dst != interface.AssignedIP`
    -  Decrement TTL
    - If TTL  == 0, drop the packet
    - Recompute the checksum
    - Forward the packet to the `routingEntry.nextHop` according to `n.Routingtable`
     -->

4. Receive RIP packet (RIP handler)

    - If Request Command: 

        a. The node should respond with a RIP response that contains the full routing table (just like a standard periodic update). \
        b. The node should add `R` type routing entry to the table. This is learned from received RIPMsg. \
            Consider doc_example, R1 has interface `if1: 10.1.0.1`. It receives a RIP request packet sent from R2 with `Src: 10.1.0.2`. \
            This ripMsg should contain the following entries: `{0, 10.2.0.0, 24}` and `{0, 10.1.0.0, 24}`. \
            Since `{0, 10.2.0.0, 24}` is not within the same subnet as `if1: 10.1.0.1`. We need to add this entry to the node's routing table as
            `{R 10.2.0.0/24 1}`.

    - If Response Command: update the routing table using Distance Vector Routing

    ```Go
    func updateRoutingtable(ripEntries []ripEntry, n *Neighbor) { // params TBD
        //For each ripEntry
        <D, C_n, N> = <ripEntry.Address, ripEntry.Cost, n>
            // 1. Convert addr and mask to netip.Addr and netip.Prefix 
            netIp := util.Uint32ToIp(ripEntry.Address)
            prefix := netip.PrefixFrom(netIp, mask)
            // let c = ripEntry.cost + 1

            // 2. If the corresponding routingEntry not in the routing table and c < inifinity, add it to the table
            // 3. If we have an existing RoutingEntry
                // refer to Distance Vector Routing
    }
    ```