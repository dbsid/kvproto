# Table Group Protocol Contract

## Ownership

`proto/table_grouppb.proto` defines the shared wire contract. PD is the sole future writer of authoritative `TableGroup` snapshots. CSE persists and reports an applied mirror through `metapb.Region.table_group`. SQL catalog and physical key encoding are consumers, not authorities.

This initial contract is limited to new clusters or explicitly enabled new keyspaces. It does not define conversion or downgrade of an existing keyspace.

## Ordinary Region Compatibility

`metapb.Region.table_group` is field 9 and is optional. When it is absent, the Region is an ordinary Region and existing split behavior applies. An ordinary Region therefore gains no serialized heartbeat bytes.

Presence activates validation, not permissive behavior. A consumer must fail closed when any of these conditions holds:

- `table_group_id` is zero;
- `applied_metadata_version` is zero;
- identity conflicts with the authoritative record;
- applied version regresses, skips an unavailable snapshot, or exceeds authority;
- full Table Group metadata is unavailable;
- state, split policy, capacity state, or another required enum is unspecified or unknown.

`keyspace_id` zero is a valid identity value and must not be used alone as an absence sentinel.

## Identity And Version Rules

- Complete identity is `(keyspace_id, table_group_id)`.
- `table_group_id` is non-zero and never reused, including after tombstone.
- Region mirror keyspace/group identity is immutable after attachment.
- Authoritative `metadata_version` starts at one and advances on identity, lifecycle, membership, binding-spec, split-policy, capacity-budget, or placement mutation.
- Independent `status_version` starts at one and advances when observed Region applied version or capacity status changes.
- Region `applied_metadata_version` is non-zero and monotonically increasing.
- Equal metadata-version equal-spec replay is idempotent; equal metadata-version unequal spec is a conflict.
- Status-only changes never advance or invalidate metadata compare-and-swap operations.
- Lower versions are stale. Consumers do not silently accept a skipped version.

## Region Binding

One `TableGroupRegionBinding` contains exactly one Region ID, one Region epoch, one CSE Shard ID, and the observed applied metadata version. Region ID also identifies the Raft Group. No list or secondary binding exists in this version.

Region ID, epoch, and Shard ID are metadata-versioned binding spec. `applied_metadata_version` is status-versioned observation. This separation prevents authority acknowledgement from creating a new metadata version that the Region must immediately apply again.

Before the first mirror attachment, CSE cannot know the allocated Table Group ID from Region metadata. `GetTableGroupByRegion` therefore resolves an existing PD authority snapshot by `(keyspace_id, region_id)`. It is a read-only recovery/discovery operation over the same authoritative record and Region index; it does not create a second metadata source. The explicit keyspace ID prevents a caller from using a Region lookup to cross a tenant boundary.

## Membership

`active_membership` and `prepared_membership` are complete snapshots, not deltas. A `TableGroupMember` includes one table or partition's row/primary-key data implicitly and carries the complete secondary-index ID set explicitly.

Canonical form is required:

- members are unique and ordered by `(table_id, partition_id)`;
- a non-partitioned table uses partition ID zero;
- index IDs are unique and ascending;
- prepared membership version is active membership version plus one;
- only active membership is visible to query/catalog consumers.

Milestone-1 validation limits are 1,024 members and 4,096 total index IDs.

## Lifecycle

| Current state | Operation | Result |
|---|---|---|
| absent | create | `CREATING` with stable identity and complete binding/policies |
| `CREATING` | exact create replay | same group; no second allocation |
| `CREATING` | authority observes matching applied Region version | `ACTIVE` |
| `ACTIVE` | prepare membership | `UPDATING_MEMBERSHIP`; active snapshot unchanged |
| `UPDATING_MEMBERSHIP` | commit matching token/version | `ACTIVE`; prepared atomically becomes active |
| `UPDATING_MEMBERSHIP` | abort matching token/version | `ACTIVE`; prepared discarded, active unchanged |
| any | mismatched token/version | no mutation and structured error |

`DELETING` and `TOMBSTONE` reserve stable wire values. Milestone-1 exposes no delete RPC and never reuses tombstoned identity.

## Operation Tokens

Mutation tokens are opaque, non-empty byte strings up to 64 bytes. The same token, operation kind, base/target versions, and payload identify an exact replay. Reusing a token with different content returns `TABLE_GROUP_ERROR_CODE_OPERATION_CONFLICT`.

Tokens are idempotency identifiers, not authentication credentials.

## Split Policy

Active Milestone-1 groups require `SPLIT_POLICY_MODE_FORBID`. Every denied split reports a bounded `SplitSource` and `SplitRejectionReason`. Callers and metrics use enum values rather than parsing diagnostic text.

The source enum covers automatic size, automatic key count, automatic load, manual request, administrative command, and recovery replay paths. No capacity condition changes split policy; exhausted groups require rejection, throttling, or explicit future migration.

## Capacity

Every active group has finite non-zero budgets for data bytes, write bytes per second, requests per second, Raft log bytes, snapshot bytes, and recovery seconds. Limit action is reject or throttle.

`CAPACITY_STATE_UNKNOWN` is fail-closed and never means healthy. Errors return every limiting capacity dimension. Capacity observations advance `status_version`, not `metadata_version`, so telemetry cannot starve DDL compare-and-swap. U2/U3 implement admission but must use these exact units and semantics.

## Placement

Placement intent is declarative: replica count, location labels, replica constraints, and leader constraints. Current peers and current leader remain in existing Region metadata. Placement updates advance the same Table Group metadata version, avoiding a second independently versioned authority.

Milestone-1 validation limits are 64 total constraints, 16 values per constraint, and 128 bytes per label key/value.

## Error Handling

`ResponseHeader.error` is the domain result. A missing error or `TABLE_GROUP_ERROR_CODE_OK` is success. Callers branch only on `TableGroupErrorCode` and structured context; `message` is diagnostic.

| Error class | Caller behavior |
|---|---|
| not found, already exists, invalid argument, identity/epoch/membership mismatch, split forbidden | terminal for unchanged request state |
| stale metadata version | refresh authority and rebuild; reconcile exact prior token first |
| operation conflict, invalid state | reconcile identity/token; do not blind retry changed content |
| capacity exceeded or unknown | reject/throttle; never auto-split |
| internal | bounded retry with the same operation token |

## Import Direction

`table_grouppb` imports `metapb` for `RegionEpoch`. It deliberately does not import `pdpb` or `schedulingpb`. Those packages may therefore add `table_grouppb.TableGroupError` to existing split responses later without an import cycle.

## Field Number Registry

Existing numbers are immutable. New fields must be additive and must pass `protolock status`.

### metapb

| Message | Field | Number |
|---|---|---|
| `TableGroupRegionMeta` | `keyspace_id` | 1 |
| `TableGroupRegionMeta` | `table_group_id` | 2 |
| `TableGroupRegionMeta` | `applied_metadata_version` | 3 |
| `Region` | `table_group` | 9 |

`Region` fields 1 through 8 retain their previous assignments and semantics.

### Core Table Group Messages

| Message | Assignments |
|---|---|
| `TableGroupIdentity` | `keyspace_id=1`, `table_group_id=2` |
| `TableGroupOperation` | `token=1`, `kind=2`, `base_metadata_version=3`, `target_metadata_version=4` |
| `TableGroupMember` | `table_id=1`, `partition_id=2`, `index_ids=3` |
| `TableGroupMembership` | `version=1`, `members=2` |
| `TableGroupRegionBinding` | `region_id=1`, `region_epoch=2`, `shard_id=3`, `applied_metadata_version=4` |
| `TableGroup` | `identity=1`, `metadata_version=2`, `state=3`, `active_membership=4`, `prepared_membership=5`, `pending_operation=6`, `region_binding=7`, `split_policy=8`, `capacity_budget=9`, `capacity_status=10`, `placement_intent=11`, `status_version=12` |
| `TableGroupError` | `code=1`, `message=2`, `identity=3`, `expected_metadata_version=4`, `actual_metadata_version=5`, `operation_token=6`, `split_source=7`, `split_rejection_reason=8`, `capacity_dimensions=9` |

### Policy Messages

| Message | Assignments |
|---|---|
| `SplitPolicy` | `mode=1` |
| `CapacityBudget` | data/write/request/Raft-log/snapshot/recovery limits `1..6`, `limit_action=7` |
| `CapacityUsage` | data/write/request/Raft-log/snapshot/recovery measurements `1..6` |
| `CapacityStatus` | `state=1`, `usage=2`, `limiting_dimensions=3`, `observed_at_unix_millis=4` |
| `PlacementConstraint` | `key=1`, `operator=2`, `values=3` |
| `PlacementIntent` | `replica_count=1`, `location_labels=2`, `replica_constraints=3`, `leader_constraints=4` |

### Lifecycle Requests

| Message | Assignments |
|---|---|
| `CreateTableGroupRequest` | `header=1`, `keyspace_id=2`, `requested_table_group_id=3`, `region_binding=4`, `split_policy=5`, `capacity_budget=6`, `placement_intent=7`, `operation_token=8` |
| `GetTableGroupRequest` | `header=1`, `identity=2` |
| `GetTableGroupByRegionRequest` | `header=1`, `keyspace_id=2`, `region_id=3` |
| `PrepareMembershipRequest` | `header=1`, `identity=2`, `expected_metadata_version=3`, `proposed_membership=4`, `operation_token=5` |
| `CommitMembershipRequest` | `header=1`, `identity=2`, `expected_metadata_version=3`, `membership_version=4`, `operation_token=5` |
| `AbortMembershipRequest` | `header=1`, `identity=2`, `expected_metadata_version=3`, `membership_version=4`, `operation_token=5` |

All lifecycle responses use `header=1`, `table_group=2`.

## Compatibility Verification

Required U1 evidence:

- new full Table Group producer/consumer round trip;
- ordinary Region round trip with mirror absent;
- new Region decoded by a legacy Region shape with old fields intact;
- unrelated unknown wire field does not populate `Region.table_group`;
- Region field 9 reflection check;
- `protolock` compatibility status;
- Go build/tests;
- Rust builds for protobuf and prost codecs.
