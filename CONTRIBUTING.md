# Contributing to Token Pulse

Thank you for your interest in contributing to Token Pulse! This guide will help you get started.

## Code of Conduct

- Be respectful and inclusive
- Help others learn and grow
- Report issues constructively
- Focus on the code, not the coder

## Getting Started

### Prerequisites
- Go 1.25.1+
- Make
- Git

### Setup Development Environment

```bash
# Clone the repository
git clone https://github.com/salayhin/token-pulse.git
cd token-pulse

# Install dependencies
go mod download

# Build the binary
make build

# Run tests
make test

# Run the dashboard
make run
```

## Development Workflow

### 1. Create a Branch
```bash
git checkout -b feature/your-feature-name
# or
git checkout -b fix/your-bug-fix
```

Branch naming conventions:
- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation updates
- `refactor/` - Code refactoring
- `test/` - Test additions/improvements

### 2. Make Your Changes

- Write clean, readable code
- Follow Go style guidelines (use `gofmt`)
- Add tests for new functionality
- Update documentation as needed

### 3. Test Your Changes

```bash
# Run all tests with race detection
make test

# Run specific test
go test ./internal/parser -run TestParseFile -race -count=1

# Run linter
make lint

# Build the binary
make build
```

### 4. Commit Changes

```bash
git add <changed-files>
git commit -m "brief description of changes

Longer explanation of what and why, if needed.
Include any relevant issue numbers.

Fixes #123
"
```

### 5. Push & Create Pull Request

```bash
git push origin feature/your-feature-name
```

Then create a Pull Request on GitHub with:
- Clear title (what changed)
- Description (why it changed)
- Related issues (if any)
- Screenshots/demos (if applicable)

### Bug Fixes
- Check [GitHub Issues](https://github.com/salayhin/token-pulse/issues)
- Look for "help wanted" or "good first issue" labels

### Documentation
- Improve README clarity
- Add more examples
- Create video tutorials
- Document API endpoints
- Improve error messages

### Testing
- Add unit tests (target: >80% coverage)
- Add integration tests
- Add UI tests
- Improve test documentation

### Performance
- Profile hot paths
- Optimize database queries
- Reduce bundle size
- Improve startup time

## Code Style Guide

### Go Code
```go
// Follow Go conventions
// - Use clear, concise variable names
// - Add comments for exported functions
// - Keep functions focused and small
// - Use interfaces for abstractions

// Good
func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
    resp, err := h.eng.Stats(r.Context())
    if err != nil {
        writeErr(w, http.StatusInternalServerError, err.Error())
        return
    }
    writeJSON(w, resp)
}

// Bad
func (h *Handlers) S(w http.ResponseWriter, r *http.Request) {
    // ambiguous naming, unclear purpose
}
```

### JavaScript/HTML/CSS
```javascript
// ES6+, no frameworks required
// - Keep functions pure
// - Use const/let (no var)
// - Add JSDoc comments for exported functions

// Good
function renderStats(data) {
  return `<div>${data.cost}</div>`;
}

// Bad
var st = data => `<div>${data.cost}</div>`;  // unclear name, no comment
```

### Commit Messages
```
[scope] Brief description of what changed

Longer explanation of why this change matters, what problem it solves,
and any important context for reviewers.

Use bullet points for multiple changes:
- Change 1
- Change 2

Fixes #123
Related to #456
```

## Testing Requirements

### Unit Tests
```go
// Test file: module_test.go
func TestFunction(t *testing.T) {
    // Arrange
    input := "test"
    
    // Act
    result := Function(input)
    
    // Assert
    if result != "expected" {
        t.Errorf("got %v, want %v", result, "expected")
    }
}
```

### Test Coverage
- New features: minimum 80% coverage
- Bug fixes: add tests that would catch the bug
- Run: `go test -cover ./...`

## Pull Request Process

1. **Before submitting:**
   - [ ] Tests pass (`make test`)
   - [ ] Linter passes (`make lint`)
   - [ ] Code builds (`make build`)
   - [ ] Documentation updated
   - [ ] Commits are clean and logical

2. **PR checklist in description:**
   ```markdown
   - [ ] Tests added/updated
   - [ ] Documentation updated
   - [ ] No breaking changes
   - [ ] Screenshots/demos (if UI change)
   ```

3. **Code review process:**
   - At least one maintainer approval required
   - Address feedback in new commits (don't amend)
   - Request re-review after changes

4. **Merging:**
   - Squash commits into logical units (usually one per feature)
   - Use meaningful commit message from PR description
   - Delete branch after merge

## Reporting Issues

### Bug Report Template
```
**Describe the bug**
A clear description of what the bug is.

**To Reproduce**
Steps to reproduce:
1. ...
2. ...

**Expected behavior**
What should happen.

**Actual behavior**
What actually happened.

**Environment**
- OS: [macOS / Linux / Windows]
- Go version: 1.25.1
- Token Pulse version: v0.1.0

**Logs/Screenshots**
Include relevant error messages or screenshots.
```

### Feature Request Template
```
**Is your feature related to a problem?**
Describe the problem.

**Describe the solution**
How should it work?

**Alternative solutions**
Other approaches you considered.

**Additional context**
Why is this important?
```

## Documentation Guidelines

### README Updates
- Keep quick start at the top
- Add features as they're implemented
- Update architecture diagram if changed
- Link to detailed docs in `/docs`

### Code Comments
```go
// Single-line comments for explaining why, not what

// Package parser converts raw JSONL lines into typed Records.
// Each line must be a complete JSON object; partial lines are left
// for the next read to avoid data loss under concurrent appends.
package parser

// parseRecord decodes a JSON line and returns the appropriate Record type.
// It returns io.EOF if the line is incomplete (no newline).
func parseRecord(line string) (Record, error) {
    // ...
}
```

### API Documentation
```
GET /api/v1/stats
  Description: Return overall token and cost statistics
  Query params:
    - None
  Response: StatsResponse
  Errors:
    - 500: Database error
```

## Performance Considerations

### Before Optimizing
- Measure first (use `pprof`)
- Profile hot paths
- Understand the bottleneck
- Optimize at the right level

### Common Areas
- **Database:** Use indexes, batch operations, limit result sets
- **API:** Cache responses, implement pagination, compress
- **UI:** Lazy-load components, debounce events, minimize re-renders
- **Build:** Reduce binary size, use embedding for static files

## Security Best Practices

- [ ] No secrets in code (use env vars)
- [ ] Validate all inputs
- [ ] Escape output properly
- [ ] Use HTTPS in production (not applicable for localhost-only tool)
- [ ] Update dependencies regularly
- [ ] Run security linter: `go sec ./...`

## Resources

- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://golang.org/doc/effective_go)
- [GoDoc Best Practices](https://pkg.go.dev/cmd/cgo)
- [SQLite Documentation](https://www.sqlite.org/docs.html)

## Getting Help

- **Questions?** Open a discussion on GitHub
- **Need guidance?** Comment on the issue or PR
- **Found a bug?** Open an issue with details
- **Have an idea?** Start a discussion first

## Recognition

Contributors will be:
- Added to CONTRIBUTORS.md
- Mentioned in release notes
- Listed in GitHub repository

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

---

Thank you for contributing to Token Pulse! 🙏
