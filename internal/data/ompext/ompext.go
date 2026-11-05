// Package ompext holds tern-kata's omp extension, tern-kata.ts, for the Pane's agents and the worker's runs to load. Its Bun
// tests sit beside it.
package ompext

import _ "embed"

// Source is tern-kata.ts: in an omp agent it names the session to `kata close` (KATA_TRANSCRIPT_AGENT and
// KATA_TRANSCRIPT_SESSION_ID) and, given an Agent address, delivers the messages written to it.
//
//go:embed tern-kata.ts
var Source []byte

// FileName is the name the extension is written under; omp loads it by path.
const FileName = "tern-kata.ts"
