// Package config defines the single, per-deployment tenant configuration for
// Kaimi and resolves it from flags, environment variables, and an optional
// config file.
//
// Historically each entry point (cmd/pipeline, cmd/dashboard) read its own
// scattered set of envOr/getEnv calls and flag definitions. That made the set
// of tenant-specific inputs — GCP project/region, model names, store path,
// Drive target, SAM/secret references — hard to see in one place and hard to
// repoint at a different tenant. This package collects all of those inputs into
// one Config value with logically grouped sub-structs.
//
// Resolution precedence, highest first:
//
//	flag > environment variable > config file > built-in default
//
// Load reuses the same env-or-default semantics the binaries already shipped
// with (an environment variable counts as "set" only when non-empty), so
// threading Config through the entry points is a pure refactor with no change
// in runtime behavior: the same env var names, flags, and defaults map to the
// same effective values.
//
// The Config carries Tenant identity (ID/DisplayName) for forward
// compatibility with multi-tenant deployments; nothing reads it yet, but
// modeling it here means new code can rely on a single source of truth rather
// than re-introducing scattered lookups.
//
// There are no package-level globals: Load returns a value the caller owns.
package config
