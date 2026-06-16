# Tasks

- [x] Phase 1: Consolidate Transport on NATS Request-Reply (Removing gRPC)
  - [x] Implement NATS Request-Reply handlers for Catalog compilation on the Master
  - [x] Implement NATS Request-Reply handlers for PKI / Bootstrap enrollment on the Master (HTTPS CA fallback)
  - [x] Update the Agent's catalog retrieval logic to use NATS Request-Reply instead of gRPC client calls
  - [x] Update the Agent's PKI bootstrap logic to enroll over HTTPS
  - [x] Remove gRPC dependency and ports from Master and Agent config/launch code
  - [x] Verify Phase 1 compiles and tests pass
- [-] Phase 2: Kubernetes Resource Model (KRM) & Status Reconciliation (Deferred)
  - [ ] Adapt schemas to support granular resource status blocks
  - [ ] Implement NATS KV store abstractions for individual resource state synchronization
  - [ ] Build Agent controllers to reconcile resources reacting to KV updates
  - [ ] Verify Phase 2 compiles and tests pass
- [x] Phase 3: Concurrent DAG Execution Scheduler
  - [x] Implement thread-safe scheduler logic with dependency satisfaction checking in `scheduler.go`
  - [x] Integrate parallel execution pool in `executor.go`
  - [x] Write unit tests for parallel and failure skip propagation
  - [x] Verify Phase 3 compiles and tests pass
