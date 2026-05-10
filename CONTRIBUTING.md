# Contributing

Thank you for considering contributing to Tlaude Code!

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/tetexu1994-star/tlaude-code.git`
3. Install Go 1.23+
4. Build: `go build ./cmd/tlaude-code/`
5. Run tests: `go test ./...`

## Development Workflow

1. **Spec-first**: Check `SPEC.md` for architecture decisions before making changes
2. **Tests pass**: Always run `go test ./...` and `go vet ./...` before committing
3. **Changelog**: Update `CHANGELOG.md` for any user-facing changes
4. **CLAUDE.md**: Keep updated if architecture changes

## Pull Request Process

1. Create a branch with a descriptive name
2. Make your changes, keeping them focused on a single concern
3. Ensure all tests pass: `go test -race ./...`
4. Update documentation as needed
5. Submit a PR with a clear description of what changed and why

## Code Style

- Follow standard Go formatting (`gofmt`)
- Use `llm.RegisterFactory()` for provider registration
- All public API surfaces should have Go doc comments
- Error handling: use structured logging via `internal/logging`

## Project Structure

See `SPEC.md` for the full architecture specification and module responsibility matrix.
