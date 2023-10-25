# TCP_Milstone

## Milestone I

- Establishing new connections by properly following the TCP state diagram under ideal conditions. Connection teardown is NOT required for this milestone.

- When creating new connections, you should allocate a data structure pertaining to the socket—be prepared to discuss what you need to include in this data structure for the rest of your implementation

- To test establishing new connections, you should implement the a, c, and (partially) ls commands in your TCP driver to listen for, create, and list connections, respectively. For the ls command, you need not list the window sizes for the milestone.

- You should be able to view your TCP traffic in Wireshark to confirm you are using the packet header correctly. However, you do not need to compute the TCP checksum yet.

- In addition, try to consider how you will tackle these problems, which we will discuss:
    - What does a SYN packet or a FIN packet do to the receiving socket (in general)?
    - What data structures/state variables would you need to represent each TCP socket?
    - How will you map incoming packets to sockets?
    - What types of events do you need to consider that would affect each socket?
    - How will you implement retransmissions?
    - In what circumstances would a socket allocation be deleted? What could be hindering when doing so? Note that the state CLOSED would not be equivalent as being deleted.

## Milestone II

For this meeting, students should have the send and receive commands working over non-lossy links. That is, send and receive should each be utilizing the sliding window and ACKing the data received to progress the window. To do this, you should have a working implementation for your send and receive buffers, including handling sequence numbers, circular buffers, etc.

Retransmission, connection teardown, packet logging and handling out-of-order packets is not required yet.

## Design

### TCP Commands (hosts)

- **Basic commands** are small wrappers around Socket API functions so that we can test them. These commands don’t do much more than call their respective API function:
    - `a`: Listen and accept on a port (like a “server”)
    - `c`: Connect to a socket (like a “client”)
    - `s`: Send data using a socket
    - `r`: Receive data on a socket
    - `cl`: Close a socket
- **Other commands** are a bit more involved:
    - `ls`: List all sockets
    - `sf`: Use your Socket API to send a file
    - `rf`: Use your Socket API to receive a file
