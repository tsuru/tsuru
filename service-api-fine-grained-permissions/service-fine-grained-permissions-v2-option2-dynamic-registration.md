# Option 2 — Dynamic Permission Registration from Service Manifests (V2)

| Field      | Value                                                            |
|------------|-----------------------------------------------------------------|
| **Status** | Draft / deep-dive                                                |
| **Scope**  | Register provider-declared actions as first-class tsuru perms    |
| **Parent** | `service-fine-grained-permissions-v2-00-analysis.md` (option 2)             |

## Idea in one paragraph

When a service registers a manifest, tsuru **registers each declared `action` as a
real permission scheme** in its permission registry (e.g.
`service.<serviceName>.<action>`). These schemes become **assignable to roles**
exactly like the built-in permissions, and the proxy handlers resolve the request's
`Method + Path → action → scheme` and run the normal
`permission.Check`. This makes provider actions first-class citizens of tsuru's
authorization model — visible in `GET /permissions`, grantable via roles, honored
by the existing parent-chain semantics — at the cost of making the historically
static registry mutable at runtime.

## Why this is faithful to V3 (and harder than option 1)

V3 wants every operation to be an authorization target with its own action name.
Option 2 delivers that literally: `acl.rules.sync` becomes a grantable tsuru
permission. The difficulty is entirely about **the registry being static today**:
built at package init in `permission/permlist.go`, never mutated, and relied upon
by code that assumes stable pointers and a fixed set of names.

## How tsuru's permission internals actually work (the constraints)

These are the load-bearing facts that dictate the design. All paths are in the
tsuru repo.

### 1. The registry is a package-global, init-time tree

`permission/permlist.go`:

```go
var PermissionRegistry = (&registry{}).addWithCtx("app", ...).add(...)...
```

`registry` (`permission/registry.go`) is a tree of `permTypes.PermissionScheme`
nodes. `addWithCtx` splits a dotted name and creates child nodes. It runs once at
process start. There is **no locking** — it assumes single-threaded construction
followed by read-only use.

### 2. Authorization compares schemes by POINTER IDENTITY

`permTypes.PermissionScheme.IsParent` (`types/permission/permission.go:77`):

```go
myPointer := reflect.ValueOf(s).Pointer()
for root != nil {
    if reflect.ValueOf(root).Pointer() == myPointer { return true }
    root = root.Parent
}
```

`permission.CheckFromPermList` calls `perm.Scheme.IsParent(scheme)`. This means the
scheme pointer attached to a **token's permission** must be the *same pointer* as
the one the handler passes to `Check`. Both must therefore come from the **same
registry instance**:

- Handler side: today `permission.PermServiceInstanceUpdateProxy` =
  `PermissionRegistry.get("service-instance.update.proxy")` → `&subR.PermissionScheme`.
- Token side: `Role.PermissionsFor` → `filterValidSchemes` →
  `PermissionRegistry.getSubRegistry(name)` → `&scheme.PermissionScheme`.

Because both resolve through the one global `PermissionRegistry`, and each named
node is a single stable `registry` struct, the `&...PermissionScheme` pointers
match across calls. **Any dynamic-registration design MUST keep this single-source
invariant**: register into the global `PermissionRegistry` (or a wrapper that
delegates to it), and resolve both handler-side and token-side through it.

### 3. Roles store names, resolve to schemes lazily

`permission.Role` (`permission/role.go`) persists only `SchemeNames []string` in
Mongo. Resolution happens at read time:

- `AddPermissions` (role.go:186) validates a permission by
  `PermissionRegistry.getSubRegistry(permName)`; unknown name → `ErrPermissionNotFound`.
  It also checks the scheme's `AllowedContexts()` includes the role's context type.
- `filterValidSchemes` (role.go:245) **silently drops** scheme names that no longer
  resolve: *"permission schemes might be removed or renamed, invalid entries in the
  database shouldn't be a problem."*

Two consequences for dynamic registration:

- **Good:** once an action scheme is registered, `AddPermissions` accepts it and
  roles can grant it with no other change. No special-casing needed.
- **Sharp edge:** if a manifest changes or a service is deleted and we *unregister*
  the scheme, every role that granted it will silently lose that permission on next
  resolution (no error, no audit). De-registration is effectively a silent bulk
  revoke. This must be a deliberate, logged operation — or avoided (see Lifecycle).

### 4. The permission list API auto-exposes everything

`listPermissions` (`api/permission.go:429`) just walks
`PermissionRegistry.Permissions()`. So dynamically registered schemes appear in
`GET /permissions` for free — role-management UIs/CLIs immediately see provider
actions with their allowed contexts. This is a real advantage of option 2 over the
manifest-only option 3.

### 5. Contexts are validated against the scheme

`AddPermissions` enforces that the role's `ContextType` is one of the scheme's
`AllowedContexts()` (which always includes `CtxGlobal` plus the declared ones, or
the nearest ancestor's). So a dynamic action scheme must declare sensible context
types (`CtxServiceInstance`, `CtxService`, `CtxTeam`) or it won't be grantable in
those role contexts.

## Proposed naming & context design

- Scheme name: `service.<serviceName>.<action>`.
  - `<serviceName>` is already constrained to `^[a-z][a-z0-9-]{0,39}$`
    (`service.validate` → `validation.ValidateName`) — **no dots**, so it is a safe
    single path segment.
  - `<action>` may itself be dotted (`rules.sync`), naturally producing sub-nodes
    (`service.acl.rules.sync` with parent `service.acl.rules`). This gives free
    hierarchical grants: granting `service.acl` covers all ACL actions; granting
    `service.acl.rules` covers all rule actions. Validate `<action>` to the same
    charset plus `.` as separator; reject empty segments and leading/trailing dots.
- Reuse the existing `service` root scheme? The built-in `service` scheme
  (`permlist.go:114`, contexts `CtxService, CtxTeam`) already exists. Nesting
  provider actions *under it* is attractive (consistent parent chain) but risks
  collisions with built-ins like `service.update.proxy`, `service.delete`. Safer to
  nest under a dedicated reserved subtree, e.g. `service.<name>.action.<action>` or
  a new root `service-action.<name>.<action>`. **Recommendation:** dedicated root
  `service-action` to avoid any collision with the fixed `service.*` verbs.
- Context types for action schemes: `[CtxServiceInstance, CtxService, CtxTeam]`
  so the same grant model as `service-instance.update.proxy` applies, and handlers
  can pass `contextsForServiceInstance(...)` unchanged.

## What must change in tsuru (by file)

### A. Make the registry mutable & thread-safe — `permission/registry.go`

- Add a `sync.RWMutex` to `registry` (or to a new top-level guard around the
  global). All mutations (`addWithCtx`, `add`) and reads (`getSubRegistry`,
  `Permissions`, `PermissionsWithContextType`) must take the lock.
  - Note `getSubRegistry` currently **writes** during reads
    (`child.PermissionScheme.Parent = &parent.PermissionScheme`), so even "reads"
    mutate. Either precompute `Parent` at insert time (preferred) so reads become
    truly read-only, or guard with the write lock. Precomputing parents also makes
    the pointer graph stable, which matters for the identity-based `IsParent`.
- Add public dynamic APIs:
  - `RegisterDynamic(name string, ctxs []ContextType) (*PermissionScheme, error)` —
    idempotent; returns the stable pointer.
  - `UnregisterDynamic(name string) error` — removes a subtree (see lifecycle
    caveats). Consider *not* implementing removal initially and instead marking
    schemes inactive.
  - Track which nodes are dynamic (a `dynamic bool` on `registry`) so
    `Permissions()`/listing can optionally distinguish provider actions, and so a
    future GC never touches built-ins.

### B. Manifest storage & ingestion — `service/service.go`, `service/manifest.go` (new)

- Add `Manifest` to `service.Service` (persisted). Minimal shape needed for auth:
  `service`, and a flat list of `{ name, method, path, action, scope }` derived
  from the V3 manifest (entities flattened into path patterns).
- On manifest set (admin upload endpoint and/or provider fetch via a new
  `GET /resources/manifest`, mirroring `Plans` at `service/endpoint.go:371`):
  1. Validate the manifest (unique op names, method enum, action charset, path
     patterns compilable).
  2. For each distinct `action`, call `permission.RegisterDynamic(
     "service-action."+svc+"."+action, [CtxServiceInstance,CtxService,CtxTeam])`.
  3. Persist the manifest on the `Service` document.

### C. Registry rebuild on startup — `service` bootstrap / `api/server.go`

- **The registry is in-memory and rebuilt every boot.** Roles persist scheme
  *names* only. So after a restart, a role granting `service-action.acl.rules.sync`
  would have that name **silently dropped** by `filterValidSchemes` unless the
  scheme is re-registered *before* any role resolution happens.
- Therefore, on API startup, after DB is available and before serving traffic,
  **iterate all services and re-register their manifest actions**. This is the most
  important operational requirement of option 2. Order matters: registration must
  complete before the first `permission.Check`.

### D. Proxy handlers — `api/service.go`, `api/service_instance.go`

- Build a per-service matcher from the manifest (`Method + normalizedPath → action`).
  Normalize the proxied path from `callback` (`serviceProxy`, `serviceInstanceProxy`)
  or the `:path` catch-all (`serviceInstanceProxyV2`).
- Resolve `action`, then `scheme := permission.SafeGet("service-action."+svc+"."+action)`
  and `permission.Check(ctx, t, scheme, contextsForServiceInstance(...)...)`.
- Keep parent-chain compatibility: because `service-instance.update.proxy` is a
  *different* subtree, holders of the old coarse permission would **no longer**
  automatically pass an action check. To stay backwards compatible, `Check` should
  pass if **either** the action scheme **or** the legacy `PermServiceInstanceUpdateProxy`
  is granted. Decide per fallback policy (default-compatible vs strict).
- Set the event `Kind` to the resolved action scheme for per-action audit.

### E. Role/permission validation — mostly free

- `AddPermissions` and `listPermissions` work unchanged once schemes are
  registered. No code change needed beyond ensuring registration happens first.

## Lifecycle & data-hygiene (the hard part)

- **Manifest update that removes an action:** unregistering silently strips the
  grant from roles (via `filterValidSchemes`). Options: (a) never auto-unregister,
  keep schemes as tombstones; (b) unregister but emit an admin-visible event/log
  listing affected roles; (c) block manifest changes that would orphan active
  grants. Recommend (a)+(b): keep by default, log on real removal.
- **Service deletion:** same problem, larger blast radius. At minimum log and,
  ideally, offer a cleanup that also `RemovePermissions` from roles so the DB
  doesn't accumulate dangling names.
- **Name collisions across services:** namespacing by `service-action.<name>`
  prevents cross-service collisions since service names are unique `_id`s.
- **Registry growth:** many services × many actions inflates the tree and the
  `GET /permissions` payload. Bounded by manifests; monitor size. Listing may want
  a filter to exclude/segregate dynamic action schemes.
- **Concurrency:** manifest ingestion (writes) races with live `permission.Check`
  (reads). The `RWMutex` from change A is mandatory, not optional.

## Comparison vs option 1 (coarse static verbs)

| Aspect | Option 1 (static verbs) | Option 2 (dynamic registration) |
|--------|-------------------------|----------------------------------|
| Fidelity to V3 | Low (verb buckets) | High (real per-action grants) |
| Registry changes | None | Mutable + thread-safe + rebuild |
| Role UX | Grant a few verbs | Grant exact provider actions, visible in `/permissions` |
| Startup coupling | None | Must re-register before serving |
| De-registration risk | N/A | Silent bulk revoke via `filterValidSchemes` |
| Backwards compat | Trivial (parent chain) | Needs explicit dual-check with legacy proxy perm |

## Requirements specific to option 2

- OR1 — The permission registry MUST support thread-safe runtime registration and
  lookup (RWMutex; precomputed parent pointers).
- OR2 — Dynamic action schemes MUST be registered into the **same** global registry
  used by both role resolution and handler checks (pointer-identity invariant).
- OR3 — On startup, tsuru MUST re-register all persisted service manifest actions
  **before** serving requests, so role grants resolve.
- OR4 — Action scheme names MUST be namespaced per service
  (`service-action.<service>.<action>`) with validated charset and dot-hierarchy.
- OR5 — Action schemes MUST declare contexts `[service-instance, service, team]`
  so they are grantable in the same role contexts as today's proxy permission.
- OR6 — De-registration MUST be explicit, logged, and identify affected roles;
  default behavior SHOULD retain schemes to avoid silent revocation.
- OR7 — For backwards compatibility, the proxy check SHOULD pass when the caller
  holds either the resolved action scheme or the legacy `service-instance.update.proxy`
  (configurable strict mode to disable the legacy fallback).

## Open questions

- Reserved subtree: `service-action.*` (new root) vs nesting under existing
  `service.*` — confirm no collision policy and CLI/UI expectations.
  - Reserved subtree
- Tombstone vs true removal on manifest change / service delete — which default?
- Sould dynamic schemes be persisted (a registration ledger) in addition to being
  derivable from manifests, to make startup re-registration order-independent and
  auditable?
- Do we expose a `dynamic: true` flag on `GET /permissions` so UIs can group
  provider actions separately?
- Migration: do we auto-create a per-service default role granting the service's
  actions to its owner teams on manifest registration?
