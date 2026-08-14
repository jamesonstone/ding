# References

## Purpose

- Use `rules/aws-agent-toolkit-guidance.md` before AWS-dependent work to select current Agent Toolkit skills, official documentation, the AWS MCP Server or CLI fallback, verified identity, name-aware material targets, infrastructure approval, and secret-safe handling
- Use `rules/testing-and-environment-validation.md` before implementation and validation to preserve code-level checks and add environment evidence safely
- Use `rules/source-file-size.md` before editing implementation/source or test files and for whole-project reconcile audits
- This directory holds durable repo-local references that are broader than one feature
- Keep long-lived background context here instead of in injected top-level instruction files
- Link these files from feature front matter references when they materially shape work
- Store durable rulesets under `rules/<slug>.md` and link them with `kit rules link` instead of copying rules into agent instruction files
- Use `rules/kit-capabilities-usage.md` in downstream projects for Kit command discovery guidance
- Use `rules/infrastructure-change-approval.md` before mutating public-cloud resources, Kubernetes resources or cluster state, or infrastructure-as-code source, configuration, or state
- Use `rules/cross-repository-program-coordination.md` before implementing or resuming an accepted multi-repository program with dependent deliverables, staged activation, or agent handoff
- Use `kit rules add` to import or activate available registry rulesets from the Kit GitHub `main` branch
- Use `kit rules view <slug>` to preview a local or registry ruleset before importing it
- Use `kit init --refresh` to adopt existing registry rules into `.kit.yaml` registry state and pick up safe upstream ruleset updates
- Use `kit rules add --custom` for the interactive `$EDITOR` ruleset builder
- `kit rule` is the singular alias for `kit rules`

## Starter Files

- `testing.md` — repo-wide testing norms and evidence expectations
- `tooling.md` — local tooling and command references that are broader than one feature
- `external-systems.md` — durable notes about external systems, APIs, or integrations
- `rules/` — pointer-loaded durable rulesets such as frontend UI rules, testing rules, API conventions, security constraints, or domain rules
