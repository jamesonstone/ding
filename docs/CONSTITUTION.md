# CONSTITUTION

## PRINCIPLES

- Invoice and payment state is durable financial truth and must be changed explicitly.
- Every send path fails closed: invalid input, missing configuration, or persistence errors send nothing.
- The CLI, Lambda handlers, and HTTP server share one invoice domain model and validation behavior.
- External messages are deterministic projections of persisted invoice and customer data.

## CONSTRAINTS

<!-- TODO: define invariant rules that must never be violated -->

### Kit-Managed Baseline Rules

<!-- BEGIN KIT-MANAGED BASELINE RULES -->
- Treat `docs/CONSTITUTION.md` as the canonical project contract.
- Keep `AGENTS.md`, `CLAUDE.md`, and `.github/copilot-instructions.md` aligned with the repo-local docs tree.
- Treat `docs/notes/<feature>` as optional source material, not canonical truth; promote durable decisions into `SPEC.md`, `docs/CONSTITUTION.md`, or durable references.
- Use native agent planning for research, clarification, design, and implementation planning.
- Before implementation, inspect code and repository memory; create or adopt `SPEC.md` when material rationale exists.
- After validation, curate feature rationale, project invariants, reusable practices, and domain knowledge into their scope-appropriate canonical documents.
- Allow a justified `not required` repository-memory decision when code and tests preserve the complete durable truth.
- Prefer implementation/source code files around 300 lines or less when splitting improves clarity and ownership.
- Do not apply the code-file size guideline to documentation files, all `docs/**`, all `.kit/**`, or `.kit.yaml`.
- Do not split or rewrite docs, generated state, or Kit config artifacts solely because they exceed 300 lines.
<!-- END KIT-MANAGED BASELINE RULES -->
## CHANGE CLASSIFICATION

<!-- all work falls into one of two tracks — classify before acting -->

### Spec-Driven (Formal)

<!-- use when: new features, kit spec, substantial architectural or behavioral changes -->
<!-- workflow: kit spec <feature> → SPEC.md phases: clarify → ready → implement → validate → reflect → deliver -->
<!-- legacy staged documents: BRAINSTORM.md, legacy SPEC.md, PLAN.md, TASKS.md only when explicitly chosen -->

### Ad Hoc (Lightweight)

<!-- use when: bug fixes, security reviews, refactors, dependency updates, config changes, small refinements -->
<!-- workflow: understand → implement → verify -->
<!-- docs: update only practical docs (READMEs, inline docs, API docs) -->
<!-- do NOT create feature SPEC.md or legacy staged artifacts for ad hoc work -->

### Ad Hoc with Existing Specs

<!-- if change touches code with existing spec docs: default to updating them -->
<!-- skip spec updates only for purely mechanical changes (formatting, typo, dep bump) -->

## NON-GOALS

- Ding is not an accounting ledger, payment processor, or general customer relationship manager.
- Ding does not use ephemeral Lambda storage as the system of record.
- Ding does not infer payments or mutate invoice state from outbound email or Discord delivery.

## DEFINITIONS

- **Customer**: a configured billing recipient identified by the stable customer ID used by commands and metadata files.
- **Invoice**: an issued amount, due-date state, and recorded payment history stored in SQLite.
- **Dry run**: rendering and validation that performs no Resend or Discord delivery.
- **Send job**: the monthly summary operation, whether invoked locally, by Lambda, or by another scheduler.
