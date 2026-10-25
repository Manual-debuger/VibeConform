// Package contract implements the contract audit (ADR 0025, spec 0051,
// spec 0052): the vibe.yaml version 2 configuration, the rules of
// contract 1.1.0 and the checks that decide them, static, execution and
// probe (spec 0053, spec 0055).
//
// The CLI imports this package, never the reverse. DetectSchema tells a
// version 2 configuration (a top-level contract key) from a version 1
// one of the removed template-sync system (spec 0059).
package contract
