package model

import (
 "errors"
 "fmt"
 "regexp"
 "strings"
 "unicode/utf8"
 g "go.kenn.io/kata/pkg/client/generated"
)

const ModeCloseForm Mode = "close-form"
const (
 OpenClose Action = "close-issue"
 ReopenAction Action = "reopen-issue"
 CloseNext Action = "close-next"
 ClosePrevious Action = "close-previous"
 CloseSave Action = "close-save"
 CloseQuick Action = "close-quick"
 CloseCancel Action = "close-cancel"
 CloseErase Action = "close-erase"
 CloseClear Action = "close-clear"
)

type CloseFormState struct {
 Open, Pending bool
 Field int
 UID string
 ProjectID, Revision int64
 Values [7]string
 Err error
}
type CloseFieldMsg struct { Field int; Value string }
func (CloseFieldMsg) msg() {}
type CloseSubmitMsg struct { Quick bool }
func (CloseSubmitMsg) msg() {}
type CloseResultMsg struct { UID string; Err error }
func (CloseResultMsg) msg() {}
type CloseIssueCmd struct { Workspace, UID string; ProjectID, Revision int64; Body g.CloseIssueBody }
func (CloseIssueCmd) cmd() {}
type ReopenIssueCmd struct { Workspace, UID, Actor string; ProjectID int64 }
func (ReopenIssueCmd) cmd() {}

func closeIssue(m Model) *Issue {
 if m.Detail != nil && m.Detail.Issue != nil && m.Detail.Issue.UID == m.SelectedUID { return m.Detail.Issue }
 for i := range m.Issues { if m.Issues[i].UID == m.SelectedUID { return &m.Issues[i] } }
 return nil
}

func openClose(m Model) Model {
 if m.Stale { return m }
 issue := closeIssue(m)
 if issue == nil { return m }
 m.CloseForm = CloseFormState{Open:true, UID:issue.UID, ProjectID:issue.ProjectID, Revision:issue.Revision}
 m.CloseForm.Values[0] = "done"
 return m
}

func submitClose(m Model, quick bool) (Model, []Cmd) {
 f := &m.CloseForm
 if !f.Open || f.Pending { return m, nil }
 if m.Stale { f.Err = errors.New("Daemon disconnected; reconnect before closing."); return m, nil }
 reason := g.CloseActionRequestBodyReason(strings.TrimSpace(f.Values[0]))
 protocol := g.CloseActionRequestBodyRetryProtocol("close-v1")
 body := g.CloseIssueBody{Actor:&m.Actor, Reason:&reason, RetryProtocol:&protocol}
 if quick { reason = "done"; source := g.CloseActionRequestBodySource("tui"); body.Source = &source } else {
  message := strings.TrimSpace(f.Values[1]); body.Message = &message
  if sha := strings.TrimSpace(f.Values[2]); sha != "" { body.Evidence = append(body.Evidence, g.Evidence{Type:"commit", Sha:&sha}) }
  if url := strings.TrimSpace(f.Values[3]); url != "" { body.Evidence = append(body.Evidence, g.Evidence{Type:"pr", URL:&url}) }
  if command := strings.TrimSpace(f.Values[4]); command != "" { body.Evidence = append(body.Evidence, g.Evidence{Type:"test", Command:&command}) }
  if ref := strings.TrimSpace(f.Values[5]); ref != "" { kind := "duplicate-of"; if reason == "superseded" { kind = "superseded-by" }; body.Evidence = append(body.Evidence, g.Evidence{Type:kind, IssueRef:&ref}) }
  if rationale := strings.TrimSpace(f.Values[6]); rationale != "" { body.Evidence = append(body.Evidence, g.Evidence{Type:"no-change-audit", Rationale:&rationale}) }
  if err := validateClose(body); err != nil { f.Err = err; return m, nil }
 }
 f.Pending, f.Err = true, nil
 return m, []Cmd{CloseIssueCmd{Workspace:m.Workspace, UID:f.UID, ProjectID:f.ProjectID, Revision:f.Revision, Body:body}}
}

var closeSHA = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
func validateClose(body g.CloseIssueBody) error {
 reason := string(*body.Reason)
 floor := 40
 switch reason { case "done", "audit-no-change": case "wontfix": floor = 60; case "duplicate", "superseded": floor = 20; default: return errors.New("Reason must be done, wontfix, duplicate, superseded, or audit-no-change.") }
 message := strings.TrimRight(strings.ToLower(strings.Join(strings.Fields(*body.Message), " ")), ".?!")
 if utf8.RuneCountInString(message) < floor { return fmt.Errorf("Explain this closure in at least %d characters.", floor) }
 for _, e := range body.Evidence { if e.Type == "commit" && !closeSHA.MatchString(*e.Sha) { return errors.New("Commit must be a 7–40 character hexadecimal SHA.") } }
 kinds := map[string]int{}
 for _, e := range body.Evidence { kinds[e.Type]++ }
 switch reason {
 case "done": if kinds["commit"]+kinds["pr"]+kinds["test"] == 0 { return errors.New("Done requires commit, PR, or test evidence; use Quick done for owner-local interactive closing.") }; if kinds["duplicate-of"]+kinds["no-change-audit"] > 0 { return errors.New("Done accepts commit, PR, or test evidence, not reference or audit evidence.") }
 case "duplicate", "superseded": kind := "duplicate-of"; if reason == "superseded" { kind = "superseded-by" }; if len(body.Evidence) != 1 || kinds[kind] != 1 { return errors.New("Duplicate/superseded requires exactly one Issue reference and no other evidence.") }
 case "audit-no-change": if len(body.Evidence) != 1 || kinds["no-change-audit"] != 1 { return errors.New("Audit requires an audit rationale and no other evidence.") }
 case "wontfix": if kinds["duplicate-of"]+kinds["no-change-audit"] > 0 { return errors.New("Wontfix does not accept reference or audit evidence.") }
 }
 return nil
}

func updateClose(m Model, msg Msg) (Model, []Cmd) {
 switch msg := msg.(type) {
 case CloseFieldMsg: if m.CloseForm.Open && !m.CloseForm.Pending && msg.Field >= 0 && msg.Field < len(m.CloseForm.Values) { m.CloseForm.Values[msg.Field] = msg.Value; m.CloseForm.Err = nil }
 case CloseSubmitMsg: return submitClose(m, msg.Quick)
 case CloseResultMsg:
  if m.CloseForm.UID != msg.UID { return m, nil }
  m.CloseForm.Pending = false
  m.CloseForm.Err = msg.Err
  if msg.Err == nil { m.CloseForm.Open = false; m.DetailUID = ""; return refreshLive(m) }
 case KeyMsg:
  if binding, ok := LookupBindingForMode(msg.Chord, ModeCloseForm); ok {
   switch binding.Action {
   case CloseCancel: if !m.CloseForm.Pending { m.CloseForm.Open = false }
   case CloseNext: m.CloseForm.Field = (m.CloseForm.Field+1)%7
   case ClosePrevious: m.CloseForm.Field = (m.CloseForm.Field+6)%7
   case CloseSave: return submitClose(m, false)
   case CloseQuick: return submitClose(m, true)
   case CloseClear: if !m.CloseForm.Pending { m.CloseForm.Values[m.CloseForm.Field] = "" }
   case CloseErase: if !m.CloseForm.Pending { s := m.CloseForm.Values[m.CloseForm.Field]; _, n := utf8.DecodeLastRuneInString(s); m.CloseForm.Values[m.CloseForm.Field] = s[:len(s)-n] }
   case QuitAction: return updateNavigation(m, QuitAction)
   default: if !m.CloseForm.Pending { m.CloseForm.Values[m.CloseForm.Field] += msg.Text }
   }
  } else if !m.CloseForm.Pending && msg.Text != "" { m.CloseForm.Values[m.CloseForm.Field] += strings.ReplaceAll(strings.ReplaceAll(msg.Text, "\n", " "), "\r", " ") }
 }
 return m, nil
}
