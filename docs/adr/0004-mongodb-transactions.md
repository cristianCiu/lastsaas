# ADR 0004: MongoDB Replica Set and Transactions

## Status

Accepted

## Decision

Development, CI integration tests, and production use MongoDB in replica-set mode. MongoDB Atlas is the initial development and production target. Transaction capability is mandatory before implementing the inventory journal.

Product integrity collections receive JSON Schema validation and critical indexes. Startup failures applying their required schema or indexes will be fatal rather than warnings.

## Consequences

- Standalone local MongoDB is insufficient for full integration testing.
- Unit and build gates can run without database credentials, but integration/E2E completion cannot be claimed without a dedicated test database.
- Inventory journal and materialized balance updates will execute in the same transaction.
