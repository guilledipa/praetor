# Tasks

- [x] Phase 1: Bazel Build System Integration
  - [x] Install `bazelisk` via Homebrew
  - [x] Initialize `MODULE.bazel` with `rules_go` and `gazelle`
  - [x] Generate `BUILD.bazel` files using Gazelle
  - [x] Compile and test with `bazel test //...`
- [x] Phase 2: Production MVP Hardening
  - [x] Robust catalog compiler error propagation
  - [x] Master-side catalog resource schema validation
  - [x] Agent catalog application mutex
  - [x] Graceful shutdown for Master & Agent
  - [x] Global verification and test pass
- [x] Phase 3: Agent Dry-Run (Simulation) Mode
  - [x] Add `DryRun` field to agent `Config` struct and command line options
  - [x] Implement dry-run logic inside the agent `Executor`
  - [x] Write unit tests for dry-run behavior
  - [x] Verify using Bazel test suite
