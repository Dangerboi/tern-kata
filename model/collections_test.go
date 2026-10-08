package model_test

import (
 "reflect"
 "testing"

 "github.com/Dangerboi/tern-kata/model"
 g "go.kenn.io/kata/pkg/client/generated"
)

func TestCollectionSwitching(t *testing.T) {
 for _, tt := range []struct{name string; project model.Project; view string}{
  {"unbound opens Inbox", model.Project{}, "inbox"},
  {"bound opens Project", model.Project{UID:"p",Name:"Work"}, "project"},
 } {
  t.Run(tt.name, func(t *testing.T){
   m,_ := model.Update(model.Model{}, model.ProjectLoaded{Project:tt.project, Snapshot:g.UISnapshotResponseBody{Catalog:[]g.UIProject{{Project:model.Project{UID:"p",Name:"Work"}}}}, ETag:"original"})
   if m.Collection.View != tt.view || len(m.Collections)!=7 {t.Fatalf("sidebar: %+v",m)}
   today := m.Collections[1]
   m,cmds := model.Update(m, model.CollectionSelected{Collection:today})
   if !m.Loading || m.Collection!=today || !reflect.DeepEqual(cmds, []model.Cmd{model.LoadCollection{Workspace:m.Workspace,Collection:today}}) {t.Fatalf("switch: %+v %#v",m,cmds)}
   m,_ = model.Update(m,model.CollectionLoaded{Collection:today,Snapshot:g.UISnapshotResponseBody{Collection:[]model.Issue{{UID:"a",Status:"open",Title:"Today",ScheduledOnDate:new("2026-05-15")}}},ETag:"today",LocalDate:"2026-05-15",TimeZone:"America/Los_Angeles"})
   if m.Loading || m.SelectedUID!="a" || len(m.Groups)!=1 || m.Groups[0].Title!="Today" {t.Fatalf("loaded: %+v",m)}
   m,cmds = model.Update(m,model.CollectionSelected{Collection:today})
   if !reflect.DeepEqual(cmds,[]model.Cmd{model.LoadCollection{Workspace:m.Workspace,Collection:today,ETag:"today"}}){t.Fatalf("validator: %#v",cmds)}
   m,_=model.Update(m,model.CollectionLoaded{Collection:today,NotModified:true})
   if m.Loading || m.SelectedUID!="a" || m.ETag!="today" {t.Fatalf("304 loses state: %+v",m)}
  })
 }
}

func TestCollectionIgnoresSupersededSnapshot(t *testing.T) {
 m,_:=model.Update(model.Model{},model.ProjectLoaded{Snapshot:g.UISnapshotResponseBody{}})
 inbox,today:=m.Collections[0],m.Collections[1]
 m,_=model.Update(m,model.CollectionSelected{Collection:today})
 m,cmds:=model.Update(m,model.CollectionSelected{Collection:inbox})
 if !reflect.DeepEqual(cmds,[]model.Cmd{model.LoadCollection{Collection:inbox}}){t.Fatalf("wrong intent validator: %#v",cmds)}
 m,_=model.Update(m,model.CollectionLoaded{Collection:today,Snapshot:g.UISnapshotResponseBody{Collection:[]model.Issue{{UID:"stale"}}}})
 if m.Collection!=inbox || !m.Loading || len(m.Issues)!=0 {t.Fatalf("stale response replaced Collection: %+v",m)}
}

