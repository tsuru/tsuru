# Option 2b — Dual Registry + Dual-Field Role Storage (V2)

| Field      | Value                                                              |
|------------|--------------------------------------------------------------------|
| **Status** | Draft / deep-dive                                                  |
| **Scope**  | Dynamic provider actions with **zero changes to the auth hot path** |
| **Parents**| `service-fine-grained-permissions-v2-00-analysis.md` (option 2), `...-option2-dynamic-registration.md` |
| **Superseded by** | `service-fine-grained-permissions-v2.md` (final spec — source of truth) |

## Summary

This refines option 2 to **maximize isolation** from tsuru's core authorization,
which runs on every request. Instead of making the single global permission
registry mutable and reusing the shared `Role.SchemeNames` grant list, we add a
**parallel, self-contained dynamic subsystem** that only activates for service
proxy calls on feature-enabled services:

- **A second, dynamic registry** — separate from the static one; mutable and
  thread-safe; evaluated by **scheme name** (not pointer identity).
- **A second role grant field** — `Role.DynamicSchemeNames`, alongside the
  existing `SchemeNames`, so dynamic grants never flow through the static
  resolution chain.
- **A first-class extension of the `Token` interface** — a new
  `DynamicPermissions(ctx)` method (non-optional; see rationale below).

The static registry, the static grant list, and the entire
`expandRolePermissions → PermissionsFor → filterValidSchemes → permission.Check`
chain stay **byte-for-byte unchanged**. The dynamic path is a sidecar that the
proxy handlers call explicitly.

## The gate: static vs dynamic

The decision of which subsystem authorizes a request is made in the **proxy
handlers** and is purely contextual:

> Use the **dynamic** path when the request is a service-instance proxy call
> (i.e. the operation that today requires `service-instance.update.proxy`) **and**
> the target service has the fine-grained feature enabled in its manifest.
> Otherwise, use the existing **static** path unchanged.

Consequences:

- All non-proxy checks (`app.*`, `job.*`, `pool.*`, ...) never touch the dynamic
  code. Zero risk to them.
- Proxy calls to services **without** a manifest keep today's behavior exactly:
  a single `permission.Check(ctx, t, PermServiceInstanceUpdateProxy, ...)`.
- Only proxy calls to feature-enabled services resolve `Method + Path → action`
  and run the dynamic check.

## Pillar A — the dynamic registry (`permission/dynamic.go`, new)

- A new type mirroring the tree shape of `registry`, but:
  - guarded by a `sync.RWMutex` (mutation happens at manifest ingest / startup;
    reads happen on proxy checks).
  - **evaluated by name**, not pointer identity. The static registry uses
    `PermissionScheme.IsParent` via `reflect...Pointer()`
    (`types/permission/permission.go:77`), which is fragile under mutation. The
    dynamic evaluator instead compares dotted `FullName()` prefixes:
    `granted "service-action.acl" is-parent-of requested "service-action.acl.rules.sync"`.
    This is safe under concurrent mutation and registry rebuilds — no pointer
    stability required.
- Public API:
  - `RegisterDynamic(name string, ctxs []ContextType) error` — idempotent.
  - `LookupDynamic(name string) (*PermissionScheme, bool)`.
  - `ListDynamic() PermissionSchemeList` — for role-management UIs.
  - `CheckDynamic(granted []string, requested string) bool` — name-prefix match
    plus context comparison, the dynamic analogue of `CheckFromPermList`.
- Naming: `service-action.<serviceName>.<action>`. Service names are already
  `^[a-z][a-z0-9-]{0,39}$` (`service.validate` → `validation.ValidateName`), so no
  dots collide; `<action>` may be dotted to form sub-grants
  (`service-action.acl.rules` covers `service-action.acl.rules.sync`). Using a
  dedicated `service-action` root avoids any collision with the fixed `service.*`
  built-ins.
- Contexts: `[CtxServiceInstance, CtxService, CtxTeam]` so proxy handlers pass the
  same `contextsForServiceInstance(...)` they use today.

## Pillar B — dual-field role storage (`permission/role.go`)

Extend the persisted `Role` document additively:

```go
type Role struct {
    Name               string
    ContextType        permTypes.ContextType
    Description        string
    SchemeNames        []string   // static (unchanged, hot path)
    DynamicSchemeNames []string   // NEW: dynamic action grants
    Events             []string
}
```

- Additive & backwards-compatible: an absent BSON field decodes to `nil`.
- **The static chain never reads `DynamicSchemeNames`.** `filterValidSchemes`
  (role.go:245) keeps operating only on `SchemeNames`, so it cannot silently drop
  a dynamic grant — the historical silent-drop behavior stays confined to static
  grants.
- New, parallel role methods (none touch the existing ones):
  - `AddDynamicPermissions(ctx, names...)` — validates each name against the
    **dynamic** registry and its `AllowedContexts()` vs the role context, then
    `$addToSet` into `dynamicschemenames`. Mirrors `AddPermissions` (role.go:186)
    but against the dynamic registry.
  - `RemoveDynamicPermissions(ctx, names...)`.
  - `DynamicPermissionsFor(contextValue) []permTypes.Permission` — mirrors
    `PermissionsFor` (role.go:266) reading `DynamicSchemeNames`.

## Pillar C — extend the `Token` interface (non-optional)

`Token` is defined in `types/auth/token.go:18` and aliased as `auth.Token`
(auth/token.go:19), which is what the proxy handlers receive. **Add the method to
the interface itself** rather than a side interface + type assertion:

```go
type Token interface {
    GetValue() string
    GetUserName() string
    User(ctx context.Context) (*User, error)
    Engine() string
    Permissions(ctx context.Context) ([]permission.Permission, error)
    DynamicPermissions(ctx context.Context) ([]permission.Permission, error) // NEW
}
```

Rationale for non-optional (per review decision): all seven token implementers
already live in-tree and share role-expansion plumbing, so extending the contract
is cheap, keeps every token type consistent, and avoids scattering
`if dt, ok := t.(...)` type assertions through the handlers. A missing
implementation becomes a compile error instead of a silent authorization gap.

Implementation is a thin mirror of the existing expansion:

- `expandDynamicRolePermissions(ctx, roleInstances)` — a sibling of
  `expandRolePermissions` (auth/user.go:172) that calls `role.DynamicPermissionsFor`
  instead of `role.PermissionsFor`.
- `BaseTokenDynamicPermission(ctx, t)` — sibling of `BaseTokenPermission`
  (auth/token.go:56); resolves the user and returns `User.DynamicPermissions`.
- Per implementer:
  - `User.DynamicPermissions` → `expandDynamicRolePermissions(ctx, u.Roles + groups)`.
    Note: it MUST NOT prepend the implicit `PermUser` self-permission (that is a
    static concept only).
  - `teamToken.DynamicPermissions` → `expandDynamicRolePermissions(ctx, t.Roles)`.
  - `APIToken`, native, oauth, oidc, peer → `BaseTokenDynamicPermission(ctx, t)`
    (they already delegate `Permissions` to the user via `BaseTokenPermission`).

Crucially, `Permissions(ctx)` is left untouched, so dynamic grants never leak into
the generic per-request permission list.

## Pillar D — manifest ingest, storage, startup

- Add `Manifest` to `service.Service` (persisted): the per-service feature config
  plus a flat list of operations `{ name, method, path, action, scope }` derived
  from the V3 manifest. Feature-level fields:
  - `strictActions` (bool, default `true`) — governs **unmatched** paths (see
    Pillar E). Default deny.
  - `legacyCompat` (bool) — the migration bridge (see Pillar E / Lifecycle).
- **Source of truth = the manifest only (decided; Option A).** No separate
  dynamic-scheme ledger collection. The dynamic registry is a pure derivation of
  the union of all services' manifest actions. This is sufficient because
  **check-time resolves the action from the manifest, not the registry** (Pillar
  E), so a role grant that no longer corresponds to any declared action is
  **inert by construction** — no path maps to it, so it can never be the requested
  action. The registry exists only as the *catalog* for role management
  (validation + listing).
- On manifest set (admin upload and/or provider fetch via a new
  `GET /resources/manifest`, mirroring `Plans` at `service/endpoint.go:371`):
  validate, then `permission.RegisterDynamic("service-action."+svc+"."+action, ...)`
  for each distinct action, and persist.
  - The admin-triggered ingest endpoint (e.g. `PUT /services/{service}/manifest`)
    accepts a **`?force=true` query parameter** that overrides the Guarded
    rejection when a change orphans active grants (see Action renames below).
- **Startup re-population (mandatory):** the dynamic registry is in-memory and
  rebuilt each boot, but grants persist as names in `DynamicSchemeNames`. On API
  startup, before serving traffic, iterate all services (`service.GetServices`,
  service.go:160 — a single query) and re-register their manifest actions. If this
  step fails, only dynamic service-proxy auth is affected — the static core is
  untouched (isolated failure domain).

### Action renames — Guarded policy (decided)

The `action` string is the **stability contract**. Because check-time maps
`Method + Path → action`, providers MAY freely change an operation's `path`,
`method`, or `name` **as long as the `action` stays the same** — existing grants
keep working untouched. Renaming the `action` string itself is a *remove old +
add new* and is therefore a **breaking authorization change**.

Adopted policy: **Guarded.** On manifest ingest, if a change would remove or rename
an `action` that still has **active role grants** (any role lists
`service-action.<svc>.<oldAction>` in `DynamicSchemeNames`), tsuru **rejects the
manifest** unless the ingest request carries **`?force=true`**. This prevents
silent denial of previously-authorized subjects. When forced (or when no active
grants exist), tsuru applies the change and logs the removed actions and affected
roles.

> **Pivot-back option — "Minimal" (documented for later).** If the Guarded policy
> proves too strict operationally, we can fall back to *Minimal*: never block a
> manifest change; instead treat `action` rename as remove+add and make ingest
> **loud** — diff old vs new actions and emit a log/event listing (a) removed
> actions and (b) exactly which roles currently grant them, so operators can
> re-grant. Under Minimal, orphaned grants remain inert-by-construction (safe, just
> non-functional) until an admin cleans them up or re-grants the new name. The only
> code difference from Guarded is swapping the ingest-time *reject* for an
> ingest-time *warn*; storage, registry, and check paths are identical, so the
> pivot is a one-line policy change plus surfacing the diff. This makes Minimal a
> cheap, always-available escape hatch.

## Pillar E — proxy handler enforcement

In `serviceInstanceProxy` / `serviceInstanceProxyV2` (`api/service_instance.go`)
and, if desired, `serviceProxy` (`api/service.go`):

1. Resolve service + instance as today.
2. If the service manifest does **not** enable the feature → existing behavior:
   `permission.Check(ctx, t, PermServiceInstanceUpdateProxy, contextsForServiceInstance(...)...)`.
3. Else (feature enabled):
   a. Normalize the proxied path (`callback` query for v1 proxy, `:path` catch-all
      for v2) and match `Method + Path → action` from the manifest.
   b. **Matched** → gather `t.DynamicPermissions(ctx)` and evaluate with
      `permission.CheckDynamic(granted, "service-action."+svc+"."+action, contexts...)`.
   c. **Unmatched** → governed by `strictActions` (**default `true` → deny**;
      decided). Only when `strictActions=false` does an unmatched path defer to the
      `legacyCompat` fallback below.
   d. **`legacyCompat` bridge (decided migration mechanism)** — if not yet allowed
      and `svc.Manifest.LegacyCompat` is set, additionally pass when the caller
      holds the legacy coarse permission:
      `permission.Check(ctx, t, PermServiceInstanceUpdateProxy, contextsForServiceInstance(...)...)`.
      This makes a feature-enabled service a **strict superset** of today's
      behavior: existing instance-owner teams keep working while admins roll out
      fine-grained grants. Flipping `legacyCompat=false` removes the bridge and
      yields pure least-privilege enforcement.
      - **Migration nudge:** while `legacyCompat` is enabled, tsuru emits a
        **recurring warning** (log/event) that the service is running in
        backwards-compat mode, to prod operators toward completing migration. There
        is **no hard expiry/TTL** — the bridge never auto-disables, avoiding abrupt
        loss of access; the record SHOULD capture `enabledAt` so a TTL could be
        layered on later if desired.
4. Set the event `Kind` / custom data to the resolved action for per-action audit.

Decision truth table for a feature-enabled service:

| Request | Caller has `service-action.<action>` | `legacyCompat` | Result |
|---------|--------------------------------------|----------------|--------|
| Matched action | yes | any | **allow** |
| Matched action | no | true + holds `update.proxy` | **allow** (bridge) |
| Matched action | no | false | **deny** |
| Unmatched path | — | true + holds `update.proxy` | **allow** (bridge) |
| Unmatched path | — | false, `strictActions=true` (default) | **deny** |

Note: `strictActions` and `legacyCompat` are two facets of the same underlying
question — "is the coarse `service-instance.update.proxy` still honored, and for
which cases?". `legacyCompat` covers *matched-but-ungranted*; `strictActions`
covers *unmatched paths*. During migration both defer to the coarse permission;
after the flip, both deny.

## Pillar F — role-management surface

- Keep `/permissions` (`listPermissions`, api/permission.go:429) and the existing
  role-permission add/remove endpoints **unchanged** (static only).
- Add sibling endpoints for the dynamic set, e.g.
  `GET /dynamic-permissions` and
  `POST/DELETE /roles/{name}/dynamic-permissions` (call
  `AddDynamicPermissions`/`RemoveDynamicPermissions`). Additive; no change to the
  existing role API handlers.
- **`GET /dynamic-permissions` shape (decided): grouped by service, each action
  enriched with its manifest metadata** so admins understand what a grant allows
  before assigning it. Sketch:

  ```json
  [
    {
      "service": "acl",
      "actions": [
        {
          "name": "service-action.acl.rules.sync",
          "action": "acl.rules.sync",
          "method": "POST",
          "path": "/sync",
          "scope": "entity",
          "entityType": "rules",
          "contexts": ["service-instance", "service", "team"]
        }
      ]
    }
  ]
  ```

  The `name` is the grantable scheme (what goes into `DynamicSchemeNames`); the
  remaining fields are carried from the service manifest for human context. The
  handler walks `ListDynamic()` and joins each scheme back to its manifest
  operation to populate method/path/scope/entityType.

## What stays untouched (the point of this design)

- `permission/permlist.go`, `permission/registry.go` static tree — unchanged.
- `permission.Check`, `CheckFromPermList`, `IsParent`, `SafeGet` — unchanged.
- `Role.SchemeNames`, `AddPermissions`, `PermissionsFor`, `filterValidSchemes`
  — unchanged.
- `expandRolePermissions`, `BaseTokenPermission`, every token's `Permissions()`
  — unchanged.
- The `Token` interface gains **one** new method; no existing method changes.

## Lifecycle & hygiene (dynamic side only)

- **Manifest changes an `action`:** handled by the **Guarded** ingest policy
  (Pillar D) — reject if it orphans active grants unless `force`d; log affected
  roles when applied.
- **Onboarding / migration:** handled by the `legacyCompat` bridge (Pillar E), not
  by auto-provisioning roles. Rationale: the subjects who perform proxy operations
  are the **instance-owner teams** (`ServiceInstance.Teams`), which are dynamic and
  unknown at manifest-registration time — so a role auto-granted to the service's
  `OwnerTeams` (the provider) would target the wrong persona. Admins assign
  fine-grained roles explicitly while the bridge keeps existing access working.
- **Service deletion:** log and optionally sweep `DynamicSchemeNames` from roles.
- **Collisions:** namespacing by unique service `_id` prevents cross-service
  clashes.
- **Concurrency:** only the dynamic registry needs the `RWMutex`; the static hot
  path remains lock-free.

### Personas (for authorization context)

- **Service provider / owner** (`Service.OwnerTeams`): registers the service +
  manifest, runs the backend tsuru proxies to. Not the day-to-day caller.
- **Instance owner / consumer** (`ServiceInstance.Teams`): app teams that create
  instances and **actually invoke proxy operations**. Authorization uses
  `contextsForServiceInstance` → these teams. Dynamic set, grows over time.
- **Tsuru admin**: manages roles/grants (`role.*`), decides which teams get which
  `service-action.*` grants.

## Requirements (option 2b)

- OR1 — The dynamic registry MUST be a separate, thread-safe structure; the static
  registry MUST remain immutable and lock-free.
- OR2 — Dynamic authorization MUST be evaluated by scheme **name** (dotted-prefix
  parenthood), not pointer identity.
- OR3 — Role storage MUST carry dynamic grants in a distinct `DynamicSchemeNames`
  field; the static resolution chain MUST NOT read it.
- OR4 — The `Token` interface MUST declare `DynamicPermissions(ctx)`, implemented
  by all token types; `Permissions(ctx)` MUST remain static-only.
- OR5 — The dynamic path MUST be gated to feature-enabled service proxy calls; all
  other requests MUST follow the unchanged static path.
- OR6 — On startup, tsuru MUST re-register persisted manifest actions into the
  dynamic registry before serving traffic; failure MUST NOT affect static auth.
- OR7 — The manifest is the sole source of truth for dynamic schemes (no ledger);
  action removal/rename MUST follow the Guarded ingest policy (reject on orphaned
  active grants unless `force`d, with logging).
- OR8 — For migration, a feature-enabled service MUST support a per-service
  `legacyCompat` flag that additionally honors the legacy
  `service-instance.update.proxy` grant for matched-but-ungranted requests (and,
  when `strictActions=false`, for unmatched paths).
- OR9 — Unmatched paths on feature-enabled services MUST be governed by a
  per-service `strictActions` flag, defaulting to `true` (deny).
- OR10 — `DynamicPermissions` MUST NOT be cached beyond the existing (uncached)
  `Permissions()` behavior; a shared per-request role cache MAY be added later if
  profiling warrants.

## Resolved decisions

| # | Question | Decision |
|---|----------|----------|
| Q1 | Reserved root scheme name | `service-action.<service>.<action>` |
| Q2 | Source of truth / ledger | Manifest-only (Option A); no ledger. Renames use the **Guarded** policy; **Minimal** documented as pivot-back |
| Q3 | Unmatched-path fallback | Per-service `strictActions`, default **deny** |
| Q4 | Cache `DynamicPermissions` | No caching now (mirror uncached `Permissions()`) |
| Q5 | Auto-provision roles | No — rely on the `legacyCompat` migration bridge |

## Remaining open questions

All previously-open questions are resolved (see Resolved decisions above and the
follow-ups below, now decided):

- **`force` wire format** → **query parameter** `?force=true` on the admin
  manifest-set endpoint (Pillar D).
- **`legacyCompat` nudging** → **recurring warning** event/log while enabled, **no
  hard expiry**; record `enabledAt` for a possible future TTL (Pillar E).
- **`GET /dynamic-permissions` shape** → **grouped by service**, each action
  enriched with manifest metadata (method/path/scope/entityType) (Pillar F).

No open questions remain for option 2b at the design level. Implementation-time
details (exact route versions, CLI command names, event kind strings) are left to
the coding phase.
