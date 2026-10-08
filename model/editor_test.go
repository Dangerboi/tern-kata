package model_test

import (
 "reflect"
 "testing"

 "github.com/Dangerboi/tern-kata/model"
 g "go.kenn.io/kata/pkg/client/generated"
)

func editorModel() model.Model {
 issue := model.Issue{UID:"a", ProjectID:2, Body:"a😀b", Revision:7}
 return model.Model{Workspace:"/repo", SelectedUID:"a", DetailUID:"a", Detail:&g.UISelectedAuthority{Issue:&issue}, Focus:model.ColumnDetail}
}

func TestDraftNativeEditAndSave(t *testing.T) {
 m := editorModel()
 for _, msg := range []model.Msg{model.KeyMsg{Chord:"e"}, model.EditMsg{ID:"layer.editor.body", From:1, To:3, Text:"new", Cursor:4, Len:4}} {
  m, _ = model.Update(m,msg)
 }
 m, cmds := model.Update(m, model.KeyMsg{Chord:"ctrl+o"})
 want := []model.Cmd{model.SaveDraft{Workspace:"/repo", UID:"a", ProjectID:2, BaseRevision:7, Text:"anewb"}}
 if m.Drafts["a"].Editor.Text != "anewb" || !m.Drafts["a"].Saving || !reflect.DeepEqual(cmds,want) { t.Fatalf("Draft/save: %+v %#v",m.Drafts,cmds) }
}

func TestDraftSequences(t *testing.T) {
 latest:=model.Issue{UID:"a",ProjectID:2,Body:"Remote body",Revision:8}
 key:=func(chord string) model.Msg { return model.KeyMsg{Chord:chord} }
 typed:=model.KeyMsg{Chord:"paste",Text:"local"}
 conflict:=model.DraftConflict{UID:"a",Issue:latest}
 for _,tt:=range []struct {
  name string
  messages []model.Msg
  text string
  mode model.Mode
  present bool
  command model.Cmd
 }{
  {"typed undo",[]model.Msg{key("e"),typed,key("ctrl+z")},"a😀b",model.ModeEditor,true,nil},
  {"native undo",[]model.Msg{key("e"),model.EditMsg{ID:"layer.editor.body",From:1,To:3,Text:"x",Cursor:2,Len:4},model.UndoMsg{ID:"layer.editor.body"}},"a😀b",model.ModeEditor,true,nil},
  {"stale edit",[]model.Msg{key("e"),model.EditMsg{ID:"layer.editor.body",From:0,To:1,Text:"x",Cursor:1,Len:99}},"a😀b",model.ModeEditor,true,nil},
  {"surrogate split",[]model.Msg{key("e"),model.EditMsg{ID:"layer.editor.body",From:2,To:3,Text:"x",Cursor:3,Len:4}},"a😀b",model.ModeEditor,true,nil},
  {"ordinary backspace",[]model.Msg{key("e"),key("left"),key("backspace")},"ab",model.ModeEditor,true,nil},
  {"selection replacement",[]model.Msg{key("e"),key("shift+left"),typed},"a😀local",model.ModeEditor,true,nil},
  {"discard asks",[]model.Msg{key("e"),key("escape")},"a😀b",model.ModeDiscard,true,nil},
  {"discard cancel",[]model.Msg{key("e"),key("escape"),key("n")},"a😀b",model.ModeEditor,true,nil},
  {"discard confirmed",[]model.Msg{key("e"),key("escape"),key("y")},"",model.ModeDetail,false,nil},
  {"empty comment escape",[]model.Msg{key("c"),key("escape")},"",model.ModeDetail,false,nil},
  {"conflict keeps both",[]model.Msg{key("e"),typed,key("ctrl+o"),conflict},"a😀blocal",model.ModeConflict,true,nil},
  {"conflict overwrite",[]model.Msg{key("e"),typed,conflict,key("o")},"a😀blocal",model.ModeEditor,true,model.SaveDraft{Workspace:"/repo",UID:"a",ProjectID:2,BaseRevision:8,Text:"a😀blocal"}},
  {"conflict discard",[]model.Msg{key("e"),typed,conflict,key("d")},"",model.ModeDetail,false,nil},
  {"live detail preserves Draft",[]model.Msg{key("e"),typed,model.DetailLoaded{UID:"a",Selected:&g.UISelectedAuthority{Issue:&latest}}},"a😀blocal",model.ModeEditor,true,nil},
  {"comment save",[]model.Msg{key("c"),typed,key("meta+o")},"local",model.ModeEditor,true,model.SaveDraft{Workspace:"/repo",UID:"a",ProjectID:2,BaseRevision:7,Text:"local",Comment:true}},
  {"external editor",[]model.Msg{key("e"),key("ctrl+e")},"a😀b",model.ModeEditor,true,model.OpenDraftEditor{UID:"a",Text:"a😀b"}},
  {"external edit undo",[]model.Msg{key("e"),model.DraftEditorClosed{UID:"a",Text:"external"},key("ctrl+z")},"a😀b",model.ModeEditor,true,nil},
 } {
  t.Run(tt.name,func(t *testing.T) {
   m:=editorModel(); var cmds []model.Cmd
   for _,msg:=range tt.messages { m,cmds=model.Update(m,msg) }
   draft,present:=m.Drafts["a"]
   if present!=tt.present || draft.Editor.Text!=tt.text || m.CurrentMode()!=tt.mode { t.Fatalf("Draft=%+v mode=%s",draft,m.CurrentMode()) }
   var want []model.Cmd; if tt.command!=nil { want=[]model.Cmd{tt.command} }
   if !reflect.DeepEqual(cmds,want) { t.Fatalf("Cmds=%#v want %#v",cmds,want) }
   if draft.Conflict!=nil && draft.Conflict.Body!="Remote body" { t.Fatal("remote version lost") }
   if present && draft.BaseRevision!=7 && tt.name!="conflict overwrite" { t.Fatal("live update rebased Draft") }
   if tt.name=="conflict discard" && m.Detail.Issue.Body!="Remote body" { t.Fatal("discard did not retain remote version") }
  })
 }
}
