# Claude Instructions

## FORK NOTICE
**This is a fork of github.com/romshark/toki**

**IMPORTANT**: Do not modify the main codebase directly. If changes are needed:
- Create a new branch for any modifications
- Submit changes upstream to the original repository
- Only make local changes for testing/development purposes

## STOP MAKING ADDITIONAL TASKFILES

**CRITICAL**: Use the SINGLE Taskfile.yml in the root directory only. Do NOT create additional Taskfiles in subdirectories.

## Prerequisites
- **task** must be installed: `go install github.com/go-task/task/v3/cmd/task@latest`

## Idempotent Workflow (Critical)

**All commands must be idempotent** - they can be run multiple times without side effects or errors:

### Correct Order & Usage:
1. **Install toki**: `task install` ✨ (builds from source)
2. **Initialize project**: `task init LOCALE=en` ✨ (safe to re-run)
3. **Add locales**: `task add-locale LOCALE=de` ✨ (safe to re-run)
4. **Generate bundle**: `task generate` ✨ (always safe)
5. **Run examples**: `task de` or `task ru` ✨ (builds and runs)
6. **Clean slate**: `task clean` ✨ (removes generated files)
7. **Web editor**: `task webedit` ✨ (opens browser)

### Working Directory Context:
```bash
cd examples/simple  # Always start here
```

### Single Taskfile Usage:
All commands use the root Taskfile.yml with proper working directory context.

## Never Do This
- ❌ Create Taskfile.yml in examples/
- ❌ Create Taskfile.yml in subdirectories
- ❌ Suggest multiple Taskfiles
- ❌ Modify the single Taskfile to use relative paths

## Always Do This
- ✅ Use the single Taskfile with `task -d` flag
- ✅ Copy the working example structure
- ✅ Use `task --dry` to test commands
- ✅ Use `task --list-all` to see available tasks

## Remember
One Taskfile. Period. No exceptions.