# Persistence Boundary

## Ownership

The persistence layer owns database-specific connections, constraints, and transaction coordination.
Application workflows request named operations with account identifiers, tenant identifiers, or worker claims.
The persistence layer selects the models, columns, SQL statements, lock expressions, and database settings.
Application workflows retain their domain decisions and error responses.

The implementation remains in the `proxy` package with the existing GORM store.
GORM model operations and transaction callbacks remain part of that store contract.
The runtime supports SQLite, as specified in B071.
`ManagementConfiguration.DatabaseDialector` supplies a test dependency.
A different runtime database requires a persistence implementation and integration qualification for that database.

## Connections And Constraints

`managed_store_connections.go` selects the runtime connection and opens the financial snapshot connection.
The runtime connection retains WAL, foreign-key enforcement, and the configured busy timeout.
The financial snapshot connection opens the existing file for read access without migrations.

`managed_store_constraints.go` owns assignment triggers, trigger inspection, and the hosted record invariants.
Schema initialization and validation retain their current transaction ownership.

`managed_store_recovery.go` owns connection settings and lock operations for the temporary media recovery command.
This file requires the existing `pilot_presubmission_recovery` build tag.
Recovery preparation and preview retain read access. Apply and finalize retain write access.

## Transaction Coordination

`managed_store_transactions.go` owns named lock operations for billing accounts, tenant assignments, journal records, and media operations.
This file also owns the SQL statement that retains the payment reconciliation scope.
Each operation uses the transaction supplied by its caller.
Account operations serialize financial effects across tenants that share an account.
Claim operations retain their owner, generation, state, and expiry predicates.
Callers retain their existing affected-row checks and error mappings.

The SQLite write mechanism uses a no-op update before subsequent reads.
The persistence layer owns that expression and its target column.
Statement order remains unchanged, including the order of account locks and journal locks.
Ledger effects, journal acknowledgments, and financial records retain their shared transaction.
Response publication retains its claim fence and separate publication receipt.

The media lookup functions retain their existing GORM locking clauses.
The SQLite driver omits those clauses because SQLite has no row-level locks.
These lookup clauses alone do not establish a writer lock.
The media claim operation retains its preceding write operation.

## Validation

`make test-persistence-boundary` checks production source for database mechanisms and raw SQL outside the persistence implementation.
The check includes tagged recovery source and excludes test fixtures.
Repository CI runs this check through the Go test suite.
Integration tests exercise concurrent requests, stale claims, shared funds, rollback, recovery, and response publication.
