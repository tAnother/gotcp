# Testing

## Test packet [ALL PASS]

1. Send the packet to the neighbors: TTL should be 31;
2. Send the packet to other available nodes: TTL should decrement according to the number of links it passed;
3. Send the packet to the node itself: TTL should be 31;
4. Host sends the packet to a non-existing ip: should only print the sent message;
5. Router sends the packet to a non-existing ip: should errors no such a node with this ip exists;
6. If an interface is down/up, should update the interfaces map `li` accordingly;
7. `ln`, `lr` should be able to print the correct information.

## RIP packet [IN PHASE]

1. **[PASS]** Should have all of the test packet capabilities;
2. **[PASS]** Should be able to update `R` routing entry at the beginning immediately;

    > r2h2 reference\
    <img src="md_images/ip/lr_start.png" alt="drawing" width="500"/>

3. **[PASS]** If an interface is down, the neighbors should delete the corresponding RIP routing entry if no other routes avaiable;
    > r2h2 reference: if0 of r2 is down\
    <img src="md_images/ip/link_down.png" alt="drawing" width="500"/>


4. **[PASS]** For each `R` routing entry, it should always has the lowest-cost path:
     > loop reference\
    <img src="md_images/ip/lowest_cost.png" alt="drawing" width="500"/>

    **Clarification**
    For r5, the next hop of 10.1.0.0 is 10.4.0.1 with cost 2 in the ref prog, but ours has 10.5.0.2 with cost 2. We do not need to worry about it. See this [post](https://edstem.org/us/courses/45889/discussion/3641481).


5. **[PASS]**  If an interface is down, the neighbors should update the routing table if there are other available routes:
    > loop reference: if0 of r2 is down\
    <img src="md_images/ip/loop_down.png" alt="drawing" width="500"/>

    **[FIXED] BUG**\
    r2 does not update the routing table accordingly.
    | Prefix         | Expected Next Hop    | Expected Cost |  Actual Next Hop    | Actual Cost |
    |--------------|------------|-------------| -------------| -------------|
    | 10.3.0.0/24 | 10.2.0.2  | 3   | 10.1.0.1 | 1 |
    | 10.0.0.0/24 | 10.2.0.2  | 4   | 10.1.0.1 | 1 |
    | 10.4.0.0/24 | 10.2.0.2  | 2   | 10.1.0.1 | 2 |

6. **[PASS]** If a host is not reachable, all of the routers need to delete that entry and should not be able to send message to that host.

    > loop reference: if1, if2 of r1 is down. h1 is not reachable\
    <img src="md_images/ip/host_non_reachable.png" alt="drawing" width="600"/>

    **[FIXED] BUG** Concurrent error:
     > loop: if1, if2 of r1 is down. \
    <img src="md_images/ip/bug/concurrent_bug.png" alt="drawing" width="600"/>

7.  **[PASS]** If an interface is back online, routing table should update accordingly.

## RIP debug changelog

10.15 Fix nil pointer issue in RipMsg.UnMarshal

10.15 Fix updateRoutingTable: skip any updates for local or static routes; costs should choose the minimum value.

10.15 Fix racing condition on `node.ListenOn()`: disassemble `ListenOn(interface)` to `BindUDP()` which binds all of the interfaces to the designated UDP port and then starts listening on each port.

10.15 Get the latest commit. Fix update rip nbhrs when an interface is down. Error still persists. For example, down r2 if0 does not update r2 routing table. and it affects r3 routing entry for 10.1.0.0 subnet.

10.16 Fixed. The error was due to old entries marked as updated when they were in fact not. 

10.16 Fixed concurrent map read in 6. Added read lock in `node.getAllEntries()`.