// Package contract implements the contract audit (ADR 0025, spec 0051,
// spec 0052): the vibe.yaml version 2 configuration, the rules of
// contract 1.0.0 and the checks that decide them.
//
// It stands beside the old reconcile-based audit and imports none of its
// packages (reconcile, state, resource, standard, module, manifest), so
// that #112 can remove them. The CLI imports this package and selects it
// for a configuration with a top-level contract key.
package contract
