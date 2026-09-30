# drp-notification-api

> notification bounded context: service API

Part of the **SpaceHub (Distributed Reservation Platform)** distributed system — team `distributed-reservation-platform`, Grupo 1.
Governance and documentation live in [`drp-docs`](https://github.com/code-corhuila/drp-docs).

## Branching

Three permanent branches. **None of them accepts a direct commit** — you enter through a child
branch and leave through a Pull Request.

```
develop  <--PR--  feat/... fix/... chore/...
qa       <--PR--  qa/...
main     <--PR--  release/...  hotfix/...
```

Promotion happens **by re-application** (`git cherry-pick -x`), never by merging one permanent
branch into another: `merge develop -> qa` and `merge qa -> main` do not exist in this model.

`main` requires **1 approval from `ariel5253`**. On `develop` and `qa` the team sets its own review
rule.

Full policy: `00-governance/branching-policy.md` in `drp-docs`.
