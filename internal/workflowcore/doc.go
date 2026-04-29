// Package workflowcore owns neutral request, plan, environment, and state
// primitives shared by command subsystems.
//
// It deliberately avoids orchestration: backup execution, health checks,
// restore flows, and command dispatch live in their owning packages. Keep this
// package small so it does not become the old workflow package under a new
// name.
package workflowcore
