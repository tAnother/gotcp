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

#### 2.3.1 Node

#### 2.3.2 Packet

### 2.4 Thread Model

### 2.5 Steps to Process IP Packets

#### 2.5.1 Send

##### 2.5.1.1 Test

##### 2.5.1.2 RIP

#### 2.5.2 Receive

##### 2.5.2.1 Test

##### 2.5.2.2 RIP

### 2.6 Known Bugs