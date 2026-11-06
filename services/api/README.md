# API optimizer

The API uses Go for Fiber HTTP routes, PostgreSQL queries and JSON models. The
normal `GET /api/v1/lists/:id/optimize` route requires the Rust `swiss-optimizer`
executable for offer selection, store allocation, subtotals, total cost and
savings. React Native remains the mobile client; Python remains the scraper.
There is no runtime Go optimizer fallback.

## Build and test

From the repository root, with Go **1.26.1 or later** and Rust **1.93.1** installed:

```sh
make api-build
make api-test
make api-fmt-check
```

`make api-build` builds the release optimizer and the API together into
`.build/api/`. Keep `server` and `swiss-optimizer` in the same directory when
copying the package. `make api` builds both and starts the API with its usual
database configuration. Running `go run main.go` alone does not package the
required optimizer. For a custom development layout, `SWISS_OPTIMIZER_BIN` may
specify an absolute path to the real executable; otherwise the API resolves it
beside its own executable, independently of the current working directory.
`CARGO_TARGET_DIR` can select an existing shared build cache.

The Rust crate uses only the standard library and builds offline with its lock
file. Builds set `GOTOOLCHAIN=local` so a missing Go version fails explicitly.
The API Docker build context remains `services/api`, as used by Compose. Its
multi-stage build includes both executables at `/server` and `/swiss-optimizer`.
The Go image uses 1.26.10, a patched release of the module's required minor line,
instead of the previous incompatible 1.24 image. Rust and runtime images use
Alpine 3.23 together. Docker builds are separate from the host test suite.

`make api-test` builds the normal release artifact, runs Rust tests, compiles Go
test binaries beside that artifact, and executes the transport and real Fiber
handler tests through `go tool test2json`. They use synthetic offers and a
standard-library SQL driver, with no database, retailer requests or running
server. Missing native artifacts cause test failure, including for empty lists.
The PR workflow runs these same targets with read-only repository permissions;
it does not deploy or publish packages/images.

## Compatibility and limits

- Selection remains keyed by the exact item name, regardless of product ID.
  Repeated names collapse to one offer. Equal prices retain the first candidate
  in SQL row order; the query has no secondary ordering for equal-price offers.
- Quantities and units are not used in optimization. Unlinked products and
  products without offers do not contribute to the plan. These are existing
  limitations, not quantity-aware shopping or a complete-list guarantee.
- The worst price per name forms the savings baseline. Totals and subtotals sum
  raw binary64 prices before rounding. Savings subtracts the unrounded best
  total from the unrounded worst total. All use half-away-from-zero rounding
  after multiplication by 100, matching Go `math.Round(x*100)/100`.
- Currency values are still compared numerically without conversion and the
  response still says `CHF`, even for mixed-currency source rows. This existing
  policy is preserved, not a currency conversion feature.
- The store grouping defect is corrected: chosen offers use the scanned store
  ID. Previously a separate, never-populated ID grouped every offer under zero.
- Name iteration and floating-point addition now follow UTF-8 byte order;
  store plans sort by numeric store ID and items by name. This replaces random
  Go map order with deterministic output and defines results at rounding
  boundaries where the old addition order could vary. Strings are not normalized.
- Public JSON models are unchanged: empty plans serialize as `[]`, optional
  empty store URLs/item units remain omitted, and no product IDs or quantities
  are added. SQL scan failures still skip the affected row. Query, stream and
  close failures now return HTTP 500 rather than a partial plan.

Each request makes one batched native call, including empty requests. SQL work,
the native queue and execution share a two-second request deadline. Up to four
native processes run per API instance; queued requests obey the same deadline.
The native program reads a single bounded request, writes a response and exits.
The host always waits/reaps it; cancellation kills it and pipe waits are bounded.
Unwinding Rust panics are caught, abort/nonzero exits are contained by the
process boundary, and malformed output fails closed. The route returns the
existing generic `500 {"error":"failed to optimize"}` on any such failure.

Limits are 100,000 successfully scanned candidates, 64 KiB per retained string
field, a 16 MiB budget for field bytes plus 20 bytes per record (excluding Go
object overhead), 16 MiB native input,
4 MiB native stdout and 4 KiB stderr. Non-finite inputs or calculated totals
are rejected. These limits bound memory and per-request process work; they do
not constitute an OS sandbox. No speedup is claimed without measurements.

## Native transport

The private process protocol uses little-endian fixed-width numbers and exact
binary64 price bits. A request is `SGOP0001`, a u32 row count, then each row's
u32 name byte length, UTF-8 name, i64 store ID and f64 price. A response is
`SGRS0001`, f64 total cost, f64 savings, u32 store count, then each store's i64
ID, f64 subtotal, u32 item count and u32 selected input row indices. Go retains
SQL model metadata and reconstructs the existing JSON response from those
indices; all selection and calculation decisions belong to Rust. Version,
length, UTF-8, finite numbers, counts, indices, ordering, duplicate/missing
selections and trailing bytes are checked. Native stderr contains only a
generic failure message, never offer data or panic payloads.

Tests preserve an independent frozen Go reference, correcting only the store-ID
defect and defining canonical map iteration. A separate characterization retains
the original defect, and the HTTP multi-store regression asserts the corrected
result. Coverage includes repeated names/products, row-order ties, Unicode,
empty/no-offer plans, rounding neighbors, worst-price savings, deterministic
generated batches, the candidate limit, scan errors, malformed transport,
missing artifacts, real native calculation failure and process timeout/reaping.
