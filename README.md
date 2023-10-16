# README
## Content

- How you abstract your link layer and its interfaces
- The thread model for your RIP implementation
- The steps you will need to process IP packets
- You should also list any known bugs or notable design decisions in your README. If you identify known bugs and include a description of how you have attempted to fix the bug/where you think the bug originates from in your code, we will take off fewer points than if we have to find them ourselves.

- In your README, please also note whether you developed your work using the container environment or on the department machines, and any instructions necessary to build your project.

## 1. Build
The project is developed in the provided container environment.

`make all` will create a folder `exec` and build executables `vhost` and `vrouter` in the folder.

`make clean` will remove `vhost` and `vrouter` in the folder.

`util/vnet_run --bin-dir exec <directory with lnx files>` should start the network as expected.

## 2. Design

### 2.1 Directory Structure

```
├── cmd                         -- source code for executables
│   ├── vhost
│   ├── vrouter
├── pkg
│   ├── proto
│   │   ├── packet.go
│   │   ├── rip.go
│   ├── ipnode
│   │   ├── node.go             -- shared structs & functions among routers & hosts
│   │   ├── router.go           -- router specific functions
│   │   ├── host.go
│   ├── repl                    -- shared repl funcs among nodes
│   │   ├── repl.go
│   ├── lnxconfig               -- parser for .lnx file
│   │   ├── lnxconfig.go
│   ├── util                    -- shared functions among the module
│   │   ├── util.go
├── util
│   ├── rip-dissector           -- for wireshark to decode messages in RIP protocols
├── exec                        -- executables
│   ├── vhost
│   ├── vrouter
├── reference                   -- reference programs
├── net                         -- network topologies
├── README.md
└── Makefile

```

### 2.2 Design Decisions

### 2.3 Abstraction 

#### 2.3.1 ipnode

Our network is representated as a graph of ip nodes. The ipnode package consists of three components: `node` as the top-level abstraction, `host` and `router` as differnent types of ip nodes.

- `Node` contains the basic information of a network node. 

    Since a host has one interface and a router might have multiple interfaces, we chose to use a map to store the interfaces' information as `Interfaces`. 

    Each interface has its own neighboring interfaces. We also want to include this kind of information in the `Node`. Therefore, we chose to use a map `ifNeighbors` for the purpose, where key is the interface's name and value is a list of `Neighbor` instances. The reason why we chose to design another struct `Neighbor` instead of directly using `Interface` is that we only want to know about the interface name, its ip and udp addresses during `send` procedures. The `IFName` will facilitate us in finding the `Interface` instance by looking at `node.Interfaces`.

    For `RoutingTable`, we also chose to use a map where key is the prefix and value is a routing entry that contains information about next hop, cost, and route type. 

    We also include a `ripNeighbors` list to store the rip neighbors of the router node. 

    ```Go
    type Node struct {
        Interfaces     map[string]*Interface          // interface name -> interface instance
        ifNeighbors    map[string][]*Neighbor         // interface name -> a list of neighbors on that interface
        RoutingTable   map[netip.Prefix]*RoutingEntry // aka forwarding table
        RoutingTableMu sync.RWMutex
        routingMode    lnxconfig.RoutingMode
        recvHandlers   map[uint8]RecvHandlerFunc
        // router specific info
        ripNeighbors []netip.Addr
    }
    ```

`node.go` also includes shared structs and functions among routers and hosts: 
    
- `Interface` as the interface of each node. Each `interface.conn` is a type of `UDPConn` which represents the link layer in this project.

    ```Go
    type Interface struct {
        Name           string
        AssignedIP     netip.Addr
        AssignedPrefix netip.Prefix
        UDPAddr        netip.AddrPort
        isDown         bool
        conn           *net.UDPConn
    }
    ```

- `Neighbor` as the neighbor (simplified) interface of each interface.

    ```Go
    type Neighbor struct {
        VIP     netip.Addr
        UDPAddr netip.AddrPort
        IFName  string
    }
    ```

- `RoutingEntry` as the entry of the node's routing table:

    ```Go
    type RoutingEntry struct {
        RouteType    RouteType
        NextHop      netip.Addr
        LocalNextHop string     // interface name. "" if not local
        Cost         int16
    }   
    ```

#### 2.3.2 proto

proto package is composed of `packet` and `rip`. 

- `Packet` represents a packet in the computer networks:

    ```GO
    type Packet struct {
        Header  *ipv4header.IPv4Header
        Payload []byte
    }
    ```

- `RipMsg` is a type of message that encompasses routing related information. 
    ```Go
    type RipMsg struct {
        Command    RoutingCmdType
        NumEntries uint16 // <= 64 (and must be 0 for a request command)
        Entries    []*RipEntry
    }

    type RipEntry struct {
        Cost    uint32 // <= 16. we define INFINITY to be 16
        Address uint32 // IP addr
        Mask    uint32 // subnet mask
    }

    type RoutingCmdType uint16

    const (
        RoutingCmdTypeRequest  RoutingCmdType = 1 // for request of routing info
        RoutingCmdTypeResponse RoutingCmdType = 2
    )
    ```

### 2.4 Thread Model

### 2.5 Steps to Process IP Packets

#### 2.5.1 Send

##### 2.5.1.1 Test

##### 2.5.1.2 RIP

#### 2.5.2 Receive

##### 2.5.2.1 Test

##### 2.5.2.2 RIP

## 3. Known Bugs