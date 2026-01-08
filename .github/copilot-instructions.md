# GitHub Copilot Usage Guidelines

- Keep code changes minimal and well-scoped; prefer small, reviewable PRs.
- Never insert secrets, tokens, or credentials. Use environment variables and documented config instead.
- Follow existing project style, imports, and lint rules; run `go test ./...` before pushing.
- Ensure tests are always passing; add or adapt tests whenever functionality changes or new features are introduced.
- Author tests to pass across Linux, macOS, and Windows; account for OS-specific behavior when writing assertions.
- Document risky or non-obvious behavior directly in code comments or commit messages.
- Avoid generating or accepting license-incompatible code snippets; write original implementations.
- For security-sensitive areas (auth, TLS, secrets), explain reasoning briefly in PR descriptions.
- Do not auto-fix unrelated files; only modify what the task requires.
- Keep comments minimal; only add comments for necessary context or non-obvious behavior.
- Remove all unused code, functions, imports, and variables before committing.
