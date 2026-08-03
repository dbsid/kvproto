# Versioned Transaction Participant Protocol

The CSE transaction participant RPC is a fail-closed envelope around the
existing TiKV transactional request/response semantics. It exists so a CSE
coordinator can combine an in-process local fast path with remote participants
without allowing an old store to ignore new version or idempotency fields and
still execute a mutation.

## Compatibility Rules

- `TxnProtocolNegotiate` must succeed before any `TxnParticipant` mutation is
  sent to a store process.
- A pre-protocol store returns gRPC `UNIMPLEMENTED`; this is a definitive
  pre-mutation rejection.
- Unknown major versions and unknown required capabilities are rejected.
  Optional capabilities may be omitted only during negotiation and never after
  a mutation has started.
- Negotiation is bound to keyspace, tenant/authorization digests, store ID,
  process ID, and compatibility baseline. A process restart forces
  renegotiation.
- Request, coordinator, negotiation, and process IDs are exactly 16 opaque
  bytes. Canonical semantic digests are exactly 32 bytes.
- An outer `CommandKind` selects exactly one existing serialized `kvrpcpb`
  request payload. The participant checks version, identity, aggregate length,
  context, and payload digest before decoding that payload.
- Retries reuse request ID, sequence, context digest, payload digest, and
  mutation digest. Reusing an ID with different bytes is rejected.
- Region/leader/epoch routing changes do not change the logical participant or
  key-set identity.
- `MutationEffect`, `ResultCertainty`, `ErrorClass`, and `RetryDirective` are
  independent. A timeout or callback loss after mutation admission is not
  success and is `UNDETERMINED` unless apply/receipt evidence proves otherwise.
- Raw keys, values, SQL, credentials, and source error strings are never
  written to logs, metrics, public diagnostics, `Debug`, or `Display` output.

## Command Payload Mapping

| `CommandKind` | Request payload | Response payload |
| --- | --- | --- |
| `SNAPSHOT_GET` | `kvrpcpb.GetRequest` | `kvrpcpb.GetResponse` |
| `SNAPSHOT_SCAN` | `kvrpcpb.ScanRequest` | `kvrpcpb.ScanResponse` |
| `PESSIMISTIC_LOCK` | `kvrpcpb.PessimisticLockRequest` | `kvrpcpb.PessimisticLockResponse` |
| `PESSIMISTIC_ROLLBACK` | `kvrpcpb.PessimisticRollbackRequest` | `kvrpcpb.PessimisticRollbackResponse` |
| `PREWRITE` | `kvrpcpb.PrewriteRequest` | `kvrpcpb.PrewriteResponse` |
| `COMMIT` | `kvrpcpb.CommitRequest` | `kvrpcpb.CommitResponse` |
| `BATCH_ROLLBACK` | `kvrpcpb.BatchRollbackRequest` | `kvrpcpb.BatchRollbackResponse` |
| `TXN_HEART_BEAT` | `kvrpcpb.TxnHeartBeatRequest` | `kvrpcpb.TxnHeartBeatResponse` |
| `CHECK_TXN_STATUS` | `kvrpcpb.CheckTxnStatusRequest` | `kvrpcpb.CheckTxnStatusResponse` |
| `CHECK_SECONDARY_LOCKS` | `kvrpcpb.CheckSecondaryLocksRequest` | `kvrpcpb.CheckSecondaryLocksResponse` |
| `RESOLVE_LOCK` | `kvrpcpb.ResolveLockRequest` | `kvrpcpb.ResolveLockResponse` |

The protocol does not create a second MVCC mutation format. CSE maps accepted
payloads into its current Storage scheduler and Raft apply path. Recovery
records are versioned schemas only; their durable ownership and atomic replay
are implemented by the later coordinator/recovery unit.

## Field Policy

- Enum zero values are unspecified and invalid for admitted commands.
- Published field numbers and enum values are never reused or renumbered.
- Removed fields are reserved by name and number.
- Optional minor additions must preserve old fixture decoding.
- Field `100` is reserved in core envelopes and records; removed fields must
  also be reserved by name and number. A future incompatible semantic change
  increments the major.
