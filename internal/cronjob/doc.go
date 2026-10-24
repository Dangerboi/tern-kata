// Package cronjob is where the rules of a kata Cron job live, so the Pane and the Worker read a job the same way (ADR-0011):
// the typed kinds of a trigger, an action and an Issue policy; the Cron job value; the Run value with its typed status; which jobs are Native jobs and which are
// Adapter jobs; the schedule of a job with the anchor rule written once; Issues a day and the high-rate warning; the
// delivery state a Native job keeps on its parent; and kata's rules for a Workflow definition (ValidateWorkflow), which the
// Workflow card and the Worker both apply.
//
// The package is a leaf: it imports neither the model, the Worker nor kata's generated client. The translation from kata's
// client types lives beside it, in cronjobclient.
package cronjob
