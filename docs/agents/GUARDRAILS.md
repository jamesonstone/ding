# Guardrails

## Hard Rules

- `docs/CONSTITUTION.md` is the canonical project contract
- Keep `AGENTS.md`, `CLAUDE.md`, and `.github/copilot-instructions.md` aligned with the repo-local docs tree
- If the user message includes an attached pasted-text file and the visible message is empty or minimal, treat the attachment as the active task instructions unless the user says otherwise
- If the attachment appears Kit-generated, follow it directly without asking what the attachment is for
- Never mix multiple features in one `docs/specs/<feature>/` directory
- Update docs first when reality diverges from documented behavior

## Work Lane Mutation Hard Gate

Before a coding agent performs any repository file or delivery mutation, it
must:

1. Load `docs/agents/GUARDRAILS.md` and
   `docs/references/rules/work-lane-gating.md`.
2. Complete read-only safety recon, including the current branch, dirty state,
   remote, active pull requests, registered worktrees, and exact primary
   checkout.
3. Ask exactly:

   > Before I make any repository changes, should I create a new GitHub issue, `GH-<issue-number>` branch, canonical worktree, and pull request for this work, or continue in the existing branch/worktree and land it through that branch's pull request?
4. Wait for the user's explicit choice unless that exact choice is already
   recorded for the same unit of work.
5. Record a Pull-Request Landing Plan with the repository, issue, branch,
   non-primary worktree, protected base, and create-or-update PR target, then
   verify that plan still matches before every mutation.

- This gate covers source, tests, documentation, specs, plans, generated files,
  configuration, and every other repository file. It also covers every delivery
  mutation, including issue, branch, staging, commit, push, worktree, and
  pull-request mutations, as well as merges. Read-only discovery and planning
  may precede it; write-capable commands such as `kit spec`,
  `kit init`, and `kit reconcile` may not.
- Never infer the choice from a clean default branch, an issue reference, or a
  generic request to produce a pull request.
- For a new lane, create or reuse one human-assigned issue, exact
  `GH-<issue-number>` branch, canonical linked worktree at
  `~/worktrees/<owner>/<repository>/GH-<issue-number>`, and one ready-PR plan
  before editing files.
- Continue existing work only after proving the non-protected branch, its exact
  owning linked worktree, issue scope, protected base, and create-or-update PR
  target. Reuse an existing pull request; do not create a second delivery lane.
- Treat the clone's primary/root checkout as read-only for coding-agent work,
  regardless of branch or cleanliness. Never edit there with a plan to move the
  diff later.
- One choice covers directly required tests, documentation, validation fixes,
  and delivery. Ask again for materially new or tangential scope.
- If an ungated or primary-checkout change is detected, stop and preserve it.
  Do not stage, commit, push, stash, reset, clean, discard, or silently transfer
  it; follow `work-lane-gating` recovery.

---

## GitHub Delivery Hard Gate

When the user asks to create or mutate an issue, branch, commit, push, or pull request in a Kit-managed project, stop before any GitHub or git mutation.

A Kit-managed project is any repository containing `.kit.yaml`, `docs/CONSTITUTION.md`, or `docs/agents/README.md`.

Before creating or mutating issues, branches, staging, commits, pushes, or PRs, agents must:

1. Load repo-local workflow entrypoints:
   - `.kit.yaml`
   - `docs/agents/README.md`
   - `docs/agents/GUARDRAILS.md`
   - `docs/agents/TOOLING.md`
   - any referenced `docs/references/rules/*` rulesets relevant to git, GitHub, branches, issues, commits, or PRs
   - `.github/pull_request_template.md` and issue templates when present
2. Run delivery recon and report the result:
   - `pwd`
   - `git status --short --branch`
   - `git remote -v`
   - current branch
   - default/base branch
   - active PRs for the current branch
   - existing matching issues
   - current git author and committer identity
3. Resolve the repo-local delivery contract before mutation:
   - issue system and required ticket format
   - issue reuse/create rules
   - branch naming convention
   - base branch refresh and staleness rules
   - self-review and no-known-errors gate before staging or commit
   - staging rule
   - commit message format
   - PR draft/ready convention
   - PR template headings
   - required validation commands
4. Present a short Delivery Contract and wait for explicit user approval if any field is unknown, ambiguous, missing, or conflicts with generic agent defaults.
5. Never use global defaults such as `codex/<slug>` branches, ad hoc issue bodies, ad hoc PR bodies, draft PRs, `git add -A`, `git add .`, or generic commit messages when repo-local Kit rules define different behavior.
6. If repo-local delivery rules cannot be found or are incomplete, stop and ask. Do not invent a substitute workflow.

Before executing GitHub delivery, output:

```text
Delivery Contract:
- Repository:
- Base branch:
- Issue source:
- Issue number/link:
- Branch name:
- Branch base:
- Branch/status/staleness check:
- Staging method:
- Commit format:
- PR title format:
- PR template:
- Draft or ready:
- Required checks:
- Cross-repo dependencies:
- Unknowns/blockers:
```

If any field is unknown, stop.

The `PR title format` field must resolve to Conventional Commits title format with the GitHub issue as scope:
`<type>(<issue_number>): <gitmoji> <short title message>`.

## No Generic GitHub Defaults In Kit Projects

In a Kit-managed project, global agent/plugin GitHub workflows are fallback tools only. They do not define process.

Do not create:

- `codex/*` branches
- ad hoc issue bodies
- ad hoc PR bodies
- draft PRs by default
- commits using generic messages
- PRs that omit the repo template

unless the repo-local Kit rules explicitly require them or the user explicitly overrides the Kit contract.

## GitHub Merge Authorization Hard Gate

- Merge is a distinct mutation boundary. PR-delivery consent, automatic lane allocation, approval, check success, subagent assignment, and a program ledger never imply merge consent.
- Merge only after a direct user request or accepted bounded merge plan names the exact authorized PR set.
- Before any merge or merge-queue mutation, resolve `pull-request-merge` and load `docs/references/rules/github-pr-merge.md`.
- Reconcile the authorization source, authenticated actor, expected head/base, repository merge policy, current reviews/checks, dependencies, and infrastructure or deployment effects before every wave.
- Only exact current `MERGE_READY` nodes may merge. Pending, missing, stale-head, or policy-ineligible skipped checks are not passing.
- Revalidating an authorized target does not require another prompt. Adding a target or materially changing actor, method, environment, infrastructure effect, or recovery requires follow-up authorization.
- Never bypass protection, reviews, required checks, a merge queue, repository policy, or identity safeguards.
- Report merge, hosted workflow, deployment/runtime, and production evidence as separate claims.

## Infrastructure Change Approval Hard Gate

- Before mutating public-cloud resources, Kubernetes resources or cluster state, or infrastructure-as-code source, configuration, or state, load `docs/references/rules/infrastructure-change-approval.md`.
- Read-only discovery may precede confirmation only when it does not alter cloud resources, Kubernetes objects, remote state, or repository-owned infrastructure source.
- Put one consolidated outline of the target context, resource actions, execution boundary, material impact and risk, rollback or recovery, and validation evidence into the task plan when planning is used; otherwise present it once before the first covered mutation. Obtain one explicit user confirmation for the complete bounded batch.
- Approval of a task plan containing the complete outline counts as confirmation. A sufficiently detailed initial request may also count only when it clearly authorizes the exact bounded batch and the batch does not delete or remove infrastructure.
- Deleting, destroying, or removing infrastructure always requires explicit confirmation after the consolidated outline, even when the initial request asked for it; one confirmation covers every deletion named in that batch.
- After confirmation, execute the exact approved batch and continue the rest of the task to completion in one pass without routine command-by-command approval.
- If additional covered infrastructure changes become necessary, collect all then-known changes into one follow-up outline, obtain one confirmation, and execute that follow-up batch in one pass. Do not re-confirm actions already included in an approved batch.
- Treat a material change to target identity, environment, region or cluster, resource set, action type, impact, or recovery as a follow-up batch; compatible tools, commands, and retries inside the approved boundary do not require another prompt.

## AWS Context Hard Gate

When .kit.yaml defines an enabled aws context, agents must:

1. Run kit aws verify before the first AWS-dependent command in the task.
2. Run kit aws verify again immediately before any command that can mutate AWS resources or deploy through AWS-backed tooling.
3. Treat the returned account ID and ARN as authoritative. A profile name alone is not proof of identity because environment credentials can change resolution.
4. Use the verified configured profile explicitly for AWS CLI, SDK, Terraform, CDK, deployment, and project scripts where supported.
5. Stop on missing AWS CLI, expired or unavailable credentials, incomplete .kit.yaml AWS fields, or an account mismatch. Read .kit.yaml and ask the user when the intended context remains ambiguous.
6. Never fall back to default, another discovered profile, or ambient credentials after verification fails.

## Completion Bar

- For v2 feature work, populate all required `SPEC.md` sections and keep front matter `workflow_version`, `phase`, references, relationships, and skills current
- For legacy staged workflows, populate all required sections in the staged artifact being used
- Replace placeholder-only sections with `not applicable`, `not required`, or `no additional information required`
- Always update affected documentation and ensure touched docs are current and properly formatted before calling work complete
- Never claim tests passed unless they ran
- Never claim files were inspected unless they were inspected
- Never guess file contents, APIs, or behavior
- If validation cannot run, state why
- Fix relevant lint and test failures before calling work complete
- Before staging or committing, self-review the diff against the ask, acceptance criteria, and repo-local rules; fix known relevant errors first
- Keep canonical front matter references and relationships current when those docs are touched

## Code Hygiene

- Remove dead code, unused exports, and public surfaces that are not strictly necessary
- If a symbol is only used locally, reduce its visibility instead of keeping it exported
- Load `docs/references/rules/source-file-size.md` before editing implementation/source or test files
- Keep every version-control-eligible handwritten implementation/source and test file at 300 physical lines or less
- Keep implementation/source code files around 300 lines or less when splitting improves clarity
- Do not apply the 300-line guideline to documentation files, `docs/**`, `.kit/**`, or `.kit.yaml`

## Safety

- Prefer explicit error handling over silent failure
- Keep changes minimal and reversible
- Do not run `git add` or `git commit` without explicit approval
- Do not run `coderabbit --prompt-only` unless explicitly requested or approved
