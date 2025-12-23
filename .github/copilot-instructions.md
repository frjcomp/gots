# GitHub Copilot Usage Guidelines

- Keep code changes minimal and well-scoped; prefer small, reviewable PRs.
- Never insert secrets, tokens, or credentials. Use environment variables and documented config instead.
- Follow existing project style, imports, and lint rules; run `go test ./...` before pushing.
- Document risky or non-obvious behavior directly in code comments or commit messages.
- Avoid generating or accepting license-incompatible code snippets; write original implementations.
- For security-sensitive areas (auth, TLS, secrets), explain reasoning briefly in PR descriptions.
- Do not auto-fix unrelated files; only modify what the task requires.
