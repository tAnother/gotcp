# TCP Testing

## Handshake under ideal conditions [ALL PASS]

1. Listen on a non-existing port: `a 9999`
2. Listen on an existing port should error: `a 9999`
3. Connect to an open port should create sockets on both sender and receiver: `c 10.0.0.1 9999`
4. Connect to a non-existing port should error: `c 10.0.0.1 8888`
5. Successful handshake
![Alt text](./md_images/tcp/ideal_handshake.png)

## Send and Recv CLI over non-lossy links

### Recv Test Case

```
s 0 aabbccdd
r 1 4       ==> aabb
s 0 eeff
r 1 12      ==> ccddeeff
r 1 6       ==> block
s 0 aabb    ==> aabb
r 1 2       ==> block
s 0 aabb    ==> aa
s 0 eeffgghh 
r 1 4       ==> bbee
```

**Expected:**
![Alt text](./md_images/tcp/terminal-read.png)
![Alt text](./md_images/tcp/expected-non-lossy-read.png)

## Retransmission

## Connection teardown

### Active Close

#### Listener

- Established: 

    `cl 0` : should not be able to connect to the listen port. Retransmit the SYN packet till failure.
    ![Alt text](./md_images/tcp/listener_close.png)

#### Normal socket

 - Established: 
    On h2, `cl 0`; then on h1, `cl 1`.
    ![Alt text](./md_images/tcp/normal_close.png)
    ![Alt text](./md_images/tcp/image.png)
    ![Alt text](./md_images/tcp/normal_close_wireshark.png)

## Out-of-order packets
