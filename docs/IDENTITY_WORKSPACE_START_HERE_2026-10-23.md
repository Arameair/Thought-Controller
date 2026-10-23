# Thought Controller — the home screen for independent identities

**Proposal only; not a running multi-identity implementation.** [Cross-repo Identity onboarding](https://github.com/Arameair/identity/blob/experiment/fresh-admin-bootstrap-design-20261023/docs/BOOTSTRAP_START_HERE_2026-10-23.md) · [Technical protocol](ENTITY_REGISTRY_AND_WORKSPACE_2026-10-23.md) · [Acceptance tests](MULTI_IDENTITY_ACCEPTANCE_2026-10-23.md)

## The owner's concept

Think **Netflix / Amazon Prime, but instead of movies there are thoughts**. Each entity is a profile/collection, and its thoughts are easy to browse. The screen is our MAIN developer workspace, not another separate TASS dashboard. **Project Controller remains the engineering tool** for branches/builds/Play/Stop; this Thought Controller dashboard is for observing and testing thinking.

~~~text
THOUGHT CONTROLLER                             [Logs] [Metrics] [Settings]
Profiles / identities:
  [TASS - AI companion]  [Example Workshop - Business]  [System Services]

Viewing: TASS                           Worker: NOT READY / MOCK / LIVE
Continue Thinking                  [Thought A] [Thought B]
Active Thoughts                    [Thought C] [Thought D]
Sleeping / Waiting                 [Thought E] [Thought F]
Resolved / History                 [Thought G] [Thought H]

[Open a thought]  -> Origin | State | Timeline | Notes | Related thoughts
                      Worker calls and timings | Accepted / rejected / stale
                      [Think Once] [Sleep] [Wake] [Resolve] [Copy Diagnostics]
~~~

The rows above are examples, **not actual thought data or currently available categories**. Start with status-based rows because their meaning can be tested. The owner has **not yet decided** whether additional categories such as goals/questions/ideas/investigations are useful. Do not hardcode that taxonomy prematurely. Use low-drama calm visual tiles and an actual empty state, never invented demo thoughts in a real session.

## Identity is not thinking

- **Identity app:** Creates/restores installation and administrator; holds accounts, startup consent, permanent IDs, permissions and capability registry. First program a new administrator starts.
- **Thought Controller:** Itself a registered **system-service identity** with a stable ID from Identity and independently verified running instance; separately manages thoughts for each authorized managed entity.
- **TASS:** An example managed companion identity. Its thought records and activity belong to TASS's ID, not the controller's system ID.
- **Example Workshop:** A different managed business identity. It can have zero thoughts, or its own authorized thought collection later.
- **Other developer's controller:** Same **reference design** and approved binary origin, but completely different controller service ID, administrator account, thought data, and local changes.

An identity need not use Thought Controller to exist. A controller being a system identity **does not authorize it to grant itself access**, impersonate another entity, or run speculative thoughts on its own.

## Starting and reconnecting

A user starts **Identity** first. Identity checks for an existing compatible Thought Controller. On the first absence it asks permission to automatically start the **specific trusted approved controller build** on future launches; after consent it requests startup from Project Controller or another explicitly approved launcher. Thought Controller's **own service ID** remains stable after Stop/Play. The controller presents its actual health/version, while Identity binds an authenticated registration.

When it appears, the dashboard opens to authorized identities from the local Identity registry. Reopening the same identity shows its saved thoughts, including waiting thoughts; changing identities switches to the other entity's isolated thought collection. The service itself stays separate under 'System Services' and need not have a thought stream.

GitHub authorization is checked by Identity, not by Thought Controller. A private repository per entity can be added after user approval. This screen must remain usable without GitHub or a ready AI worker.

## One thing the experiment should prove

**Can a new developer create two identities and inspect two independent thought collections through ONE Thought Controller without mixing their histories or permissions?**

This is the first real architectural proof, not the final theory of artificial thinking.

A mock thought or mock model can validate UI and scoping, but must be labeled **SIMULATED**. Thought Controller's prior V1 thought engine should retain its parser, beat safety, persistence, diagnostics and no-hidden-send policy. Do not infer real model quality from test-site results.

## Reference and updates

The administrator gets an approved copy of the original Thought Controller architecture, but the installation **owns its data and changes**. Show reference origin/version/SHA and whether it is a local fork or modified build. No forced upstream upgrade, no silent migration or changes to other admins' thoughts. **There are presently no published releases, and the reference repository is private; public installation is not implemented.**

[Next: protocol and team work](ENTITY_REGISTRY_AND_WORKSPACE_2026-10-23.md).