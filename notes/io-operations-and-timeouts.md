# IO Operations and Timeouts

## Communication between Worker and Manager
- Worker talks to manager over http
- Default http client has an indefinite timeout
- If current manager becomes unavailable, worker's go routines for heartbeating and checkpointing are blocked on IO

## Communication between Manager and KVS 

- During checkpoint, manager communicates with etcd
- etcd client binds io operation to the context provided when creating `Txn`
- Presentely this context is `context.Background()`
- If etcd cluster becomes unavailable but the manager node survices, calls to checkpoint blocks


