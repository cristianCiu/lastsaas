# ADR 0007: Catalog and Quantity Foundation

## Status

Accepted

## Decision

- Units, categories, items, and future supplier terms are tenant-owned master data. Operational stock remains location-scoped.
- Inventory quantities use signed `int64` micro-units with a fixed scale of 1,000,000. API representations that can exceed JavaScript's safe integer range will use decimal strings.
- Unit display precision is constrained to 0-6 decimal places. Arithmetic is centralized and rejects overflow and inexact rational conversion rather than silently rounding.
- Item-specific conversions will use positive reduced rational factors and connect directly to an item's base unit. Their persistence follows after the item model is stable.
- Catalog access uses `catalog.read` and `catalog.manage`. Company owners and operations managers inherit both through existing role defaults; explicit overrides still win.
- Unit, category, and item codes are canonical lower-case slugs and unique within a tenant. Lifecycle removal uses deactivation, not deletion.

## Consequences

- Standard units are seeded or created per tenant rather than shared through a global mutable collection.
- Exact arithmetic and tenant isolation can be tested before inventory journals depend on them.
- Allergen vocabulary, shelf-life semantics, lot-tracking defaults, and CSV idempotency keys remain decisions for the item/import slices and are not guessed by the unit slice.
