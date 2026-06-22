# Tooling

## Skills

- Repo-local canonical skills live under `.agents/skills/*/SKILL.md`
- For feature-scoped work, start with the current feature's canonical front matter `skills`, falling back to the legacy `SPEC.md` `## SKILLS` table only when front matter is absent
- Keep the selected skill set minimal and actionable

## Command Capability Discovery

- Use `kit capabilities` when choosing among Kit commands and the mutation, network, write, or git behavior is not already obvious.
- Use `kit capabilities <command> --json` for one command path, including nested paths such as `rules add` or `skill mine`.
- Use `kit capabilities --search <term> --json` for compact filtered discovery, and `kit capabilities --full --json` only when hidden or deprecated compatibility commands matter.
- Treat `kit capabilities` itself as read-only: it does not require a Kit project root and does not load project config, write files, call the network, run subprocesses, or mutate git.
- In downstream Kit-managed projects, load `docs/references/rules/kit-capabilities-usage.md` when command discovery affects the task.
- Downstream projects should use `kit capabilities` for command discovery; do not maintain Kit's internal command catalog from a downstream project.

## Dispatch

- Use `kit dispatch` when broad work must be turned into safe multi-lane execution
- Use subagents when the work cleanly separates into low-overlap lanes after discovery
- Keep broad or noisy discovery in RLM first; use dispatch or direct subagent execution only after the relevant workstreams are narrow enough to predict overlap
- Predict overlap conservatively before parallelizing
- Keep the main agent responsible for synthesis, integration, validation, and communication

## Project Directory

- Work in the existing project directory by default
- Do not create or use git worktrees for agent work
- If the current branch or dirty state is unsuitable, stop and ask the user how to proceed instead of creating an alternate checkout

## Secondary Global Inputs

- `~/.claude/CLAUDE.md`
- `${CODEX_HOME}/AGENTS.md`
- `${CODEX_HOME}/instructions.md`
- `${CODEX_HOME}/skills/*/SKILL.md`

- Treat these as secondary context after repo-local docs
- Do not use `.claude/skills` as canonical discovery input
