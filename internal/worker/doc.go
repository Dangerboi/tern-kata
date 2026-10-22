// Package worker is tern-kata's adapter for kata's cron jobs: it runs adapter prompt jobs through omp and records
// every run in kata, with no Tern tab or window open. It follows the ADRs of the herdr-kata design review:
//
//   - ADR-0002 run identity and admission: a Run's uid is derived (RunUID); a revision-0 running observation is the
//     fence that authorizes execution; existing evidence in kata vetoes a launch.
//   - ADR-0003 one worker per Project: a lease in the Project's metadata (lease.go), one worker.lock per machine (lock.go).
//   - ADR-0004 event stream and reconciliation: one stream per Project with a debounced refresh and slow, jittered
//     fallbacks (project.go), on an injected Clock.
//   - ADR-0006 journals: one run.json per run, saved before each side effect, with named save points a test hook can
//     kill the worker at (journal.go); recovery continues the journal and never infers a new attempt.
//   - ADR-0007 capacity: a slot is reserved before preparation and freed when the run's process is provably gone.
//   - ADR-0009 prompt and contract: BuildPrompt, agentArgs and the closure check in report (agent.go, run.go).
//   - ADR-0010 configuration: one strict worker.toml (config.go).
//
// Extension points for later work: Worker.prepare (the one place a Run is admitted to a slot; scheduling calls it with
// a derived occurrence), Worker.advance and Worker.recoverRun (the journal state machine every phase flows through),
// the Intent list on Journal (reporting writes), Hooks.AfterSave (crash tests), projectState.jobs (the definitions
// cache refreshed by the event follower), and the control Request ops in control.go (resume, abandon).
//
// The worker reaches kata only through Kata, the HTTP client; the only place the generated types appear.
package worker
