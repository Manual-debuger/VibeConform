package module

// GoTestCommands exposes goTestCommands to the module_test package, whose
// Taskfile walks need the same reading of a go test inside a script.
var GoTestCommands = goTestCommands
