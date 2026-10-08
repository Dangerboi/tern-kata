package model

import (
 "errors"
 "maps"
 "unicode/utf16"
)

var errDraftReadOnly = errors.New("Read-only: reconnecting to kata; Draft kept")
func len16(text string) int { return len(utf16.Encode([]rune(text))) }

type Draft struct {
 Editor TextEditor
 ProjectID, BaseRevision int64
 Comment, Saving, DiscardConfirm bool
 Conflict *Issue
 Err error
}

type SaveDraft struct { Workspace, UID, Text, Actor string; ProjectID, BaseRevision int64; Comment bool }
func (SaveDraft) cmd() {}
type OpenDraftEditor struct { UID, Text string }
func (OpenDraftEditor) cmd() {}
type EditMsg struct { ID string; From,To int; Text string; Cursor,Len int }
func (EditMsg) msg() {}
type UndoMsg struct { ID string }
func (UndoMsg) msg() {}
type DraftSaved struct { UID string }
func (DraftSaved) msg() {}
type DraftFailed struct { UID string; Err error }
func (DraftFailed) msg() {}
type DraftConflict struct { UID string; Issue Issue }
func (DraftConflict) msg() {}
type DraftEditorClosed struct { UID, Text string; Err error }
func (DraftEditorClosed) msg() {}

func (m Model) ActiveDraft() (Draft,bool) { draft,ok:=m.Drafts[m.EditorUID]; return draft,ok && m.EditorUID!="" }
func (m Model) EditorMode() Mode {
 draft,ok:=m.ActiveDraft(); if !ok { return "" }
 if draft.Conflict!=nil { return ModeConflict }
 if draft.DiscardConfirm { return ModeDiscard }
 return ModeEditor
}
func putDraft(m Model, uid string, draft Draft) Model {
 m.Drafts=maps.Clone(m.Drafts); if m.Drafts==nil { m.Drafts=make(map[string]Draft) }; m.Drafts[uid]=draft; return m
}
func dropDraft(m Model,uid string) Model {
 if draft,ok:=m.Drafts[uid]; ok && draft.Conflict!=nil && uid==m.DetailUID && m.Detail!=nil {
  detail:=*m.Detail; detail.Issue=draft.Conflict; m.Detail=&detail
 }
 m.Drafts=maps.Clone(m.Drafts); delete(m.Drafts,uid); if m.EditorUID==uid { m.EditorUID="" }; return m
}
func startDraft(m Model,comment bool) Model {
 if m.Detail==nil || m.Detail.Issue==nil || m.Detail.Issue.UID!=m.SelectedUID { return m }
 uid:=m.SelectedUID
 if _,ok:=m.Drafts[uid]; !ok {
  issue:=m.Detail.Issue; text:=issue.Body; if comment { text="" }
  m=putDraft(m,uid,Draft{Editor:NewTextEditor(text), ProjectID:issue.ProjectID, BaseRevision:issue.Revision, Comment:comment})
 }
 m.EditorUID=uid; m.Focus=ColumnDetail; m.DetailHidden=false
 return m
}
func saveDraft(m Model,draft Draft) (Model,[]Cmd) {
 if draft.Saving { return m,nil }
 if m.Stale { draft.Err=errDraftReadOnly; return putDraft(m,m.EditorUID,draft),nil }
 draft.Saving=true; draft.Err=nil
 m=putDraft(m,m.EditorUID,draft)
 return m,[]Cmd{SaveDraft{Workspace:m.Workspace, UID:m.EditorUID, ProjectID:draft.ProjectID, BaseRevision:draft.BaseRevision, Text:draft.Editor.Text, Comment:draft.Comment, Actor:m.Actor}}
}

func updateEditor(m Model,msg Msg) (Model,[]Cmd,bool) {
 switch msg:=msg.(type) {
 case DraftSaved:
  m=dropDraft(m,msg.UID)
  if msg.UID==m.DetailUID { m.DetailLoading=true; return m,[]Cmd{LoadDetail{Workspace:m.Workspace, UID:msg.UID}},true }
  return m,nil,true
 case DraftFailed:
  if draft,ok:=m.Drafts[msg.UID]; ok { draft.Saving=false; draft.Err=msg.Err; m=putDraft(m,msg.UID,draft) }; return m,nil,true
 case DraftConflict:
  if draft,ok:=m.Drafts[msg.UID]; ok { draft.Saving=false; draft.Conflict=&msg.Issue; draft.Err=nil; m=putDraft(m,msg.UID,draft) }; return m,nil,true
 case DraftEditorClosed:
  if draft,ok:=m.Drafts[msg.UID]; ok { draft.Err=msg.Err; if msg.Err==nil { draft.Editor=draft.Editor.ApplyEdit(0,len16(draft.Editor.Text),msg.Text,len16(msg.Text),len16(draft.Editor.Text)) }; m=putDraft(m,msg.UID,draft) }; return m,nil,true
 case KeyMsg:
  draft,active:=m.ActiveDraft()
  binding,ok:=LookupBindingForMode(msg.Chord,m.CurrentMode())
  if !active {
   if ok && (binding.Action==EditBody || binding.Action==AddComment) { return startDraft(m,binding.Action==AddComment),nil,true }
   return m,nil,false
  }
  if draft.Saving { return m,nil,true }
  if draft.Conflict!=nil {
   if ok && binding.Action==OverwriteDraft { draft.BaseRevision=draft.Conflict.Revision; draft.Conflict=nil; next,cmds:=saveDraft(m,draft); return next,cmds,true }
   if ok && binding.Action==DiscardDraft { return dropDraft(m,m.EditorUID),nil,true }
   return m,nil,true
  }
  if draft.DiscardConfirm {
   if ok && binding.Action==DiscardDraft { return dropDraft(m,m.EditorUID),nil,true }
   if ok && binding.Action==KeepDraft { draft.DiscardConfirm=false; return putDraft(m,m.EditorUID,draft),nil,true }
   return m,nil,true
  }
  if ok {
   switch binding.Action {
   case SaveDraftAction: next,cmds:=saveDraft(m,draft); return next,cmds,true
   case CancelDraft:
    if draft.Editor.Text=="" { return dropDraft(m,m.EditorUID),nil,true }; draft.DiscardConfirm=true
   case ExternalEditorAction: return m,[]Cmd{OpenDraftEditor{UID:m.EditorUID,Text:draft.Editor.Text}},true
   default: draft.Editor=draft.Editor.Key(binding.Action,editorSelectionChord(msg.Chord))
   }
  } else if msg.Text!="" { draft.Editor=draft.Editor.Insert(msg.Text) }
  return putDraft(m,m.EditorUID,draft),nil,true
 case EditMsg:
  draft,active:=m.ActiveDraft(); if active && m.EditorMode()==ModeEditor && !draft.Saving && msg.ID=="layer.editor.body" { draft.Editor=draft.Editor.ApplyEdit(msg.From,msg.To,msg.Text,msg.Cursor,msg.Len); m=putDraft(m,m.EditorUID,draft) }; return m,nil,true
 case UndoMsg:
  draft,active:=m.ActiveDraft(); if active && m.EditorMode()==ModeEditor && !draft.Saving && msg.ID=="layer.editor.body" { draft.Editor=draft.Editor.Undo(); m=putDraft(m,m.EditorUID,draft) }; return m,nil,true
 }
 return m,nil,false
}
