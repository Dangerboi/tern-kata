package model_test

import (
 "reflect"
 "testing"
 "time"
 "github.com/Dangerboi/tern-kata/model"
 g "go.kenn.io/kata/pkg/client/generated"
)

func TestCollectionDateGroups(t *testing.T){
 closed:=func(uid,date string)model.Issue{v,err:=time.Parse(time.RFC3339,date);if err!=nil{t.Fatal(err)};return model.Issue{UID:uid,Status:"closed",ClosedAt:&v}}
 scheduled:=func(uid,date string)model.Issue{return model.Issue{UID:uid,Status:"open",ScheduledOnDate:new(date)}}
 due:=func(uid,date string)model.Issue{return model.Issue{UID:uid,Status:"open",DeadlineOnDate:new(date)}}
 for _,tt:=range []struct{name,view,today string; issues []model.Issue; headings []string; uids []string}{
  {"upstream Upcoming", "upcoming","2026-05-15",[]model.Issue{scheduled("today","2026-05-15"),scheduled("tomorrow","2026-05-16"),scheduled("week","2026-05-22"),{UID:"undated",Status:"open"}},[]string{"2026-05-16","2026-05-22"},[]string{"tomorrow","week"}},
  {"projection wins month edge","upcoming","2026-08-31",[]model.Issue{{UID:"local-today",Status:"open",ScheduledOnDate:new("2026-08-31"),Metadata:map[string]any{"scheduled_on":"2026-09-01T00:30:00Z"}},{UID:"tomorrow",Status:"open",ScheduledOnDate:new("2026-09-01"),Metadata:map[string]any{"scheduled_on":"2026-09-01T07:30:00Z"}}},[]string{"2026-09-01"},[]string{"tomorrow"}},
  {"year and week edges","upcoming","2026-12-31",[]model.Issue{scheduled("new-year","2027-01-01"),scheduled("sunday","2027-01-03"),scheduled("monday","2027-01-04")},[]string{"2027-01-01","2027-01-03","2027-01-04"},[]string{"new-year","sunday","monday"}},
  {"empty projection excludes fallback","upcoming","2026-05-15",[]model.Issue{{UID:"empty",Status:"open",ScheduledOnDate:new(""),Metadata:map[string]any{"scheduled_on":"2026-05-16T00:00:00Z"}},{UID:"raw",Status:"open",Metadata:map[string]any{"scheduled_on":"2026-05-16T00:00:00+14:00"}}},[]string{"2026-05-16"},[]string{"raw"}},
  {"Deadlines", "deadlines","2026-05-15",[]model.Issue{due("overdue","2026-05-10"),due("today","2026-05-15"),due("future","2026-05-20"),due("later","2026-05-22")},[]string{"Overdue","Today","2026-05-20","2026-05-22"},[]string{"overdue","today","future","later"}},
  {"Today merges evening", "today","2026-05-15",[]model.Issue{scheduled("overdue","2026-05-14"),{UID:"evening",Status:"open",ScheduledOnDate:new("2026-05-15"),Metadata:map[string]any{"today_bucket":"evening"}},due("today","2026-05-15"),scheduled("future","2026-05-16")},[]string{"Overdue","Today"},[]string{"overdue","evening","today"}},
  {"upstream Logbook", "logbook","2026-05-15",[]model.Issue{closed("earlier","2026-05-14T09:00:00Z"),closed("latest","2026-05-15T15:00:00Z"),{UID:"open",Status:"open"},{UID:"undated",Status:"closed"}},[]string{"2026-05-15","2026-05-14"},[]string{"latest","earlier"}},
  {"Logbook literal offset date", "logbook","2026-01-01",[]model.Issue{closed("offset","2026-01-01T00:30:00+14:00"),closed("prior-year","2025-12-31T23:59:00Z")},[]string{"2026-01-01","2025-12-31"},[]string{"offset","prior-year"}},
 }{t.Run(tt.name,func(t *testing.T){
  collection:=model.Collection{View:tt.view,Title:tt.view}
  m:=model.Model{Collection:collection}
  m,_=model.Update(m,model.CollectionLoaded{Collection:collection,LocalDate:tt.today,TimeZone:"America/Los_Angeles",Snapshot:g.UISnapshotResponseBody{Collection:tt.issues}})
  var headings,uids []string
  for _,group:=range m.Groups{headings=append(headings,group.Title)}
  for _,issue:=range m.Issues{uids=append(uids,issue.UID)}
  if !reflect.DeepEqual(headings,tt.headings)||!reflect.DeepEqual(uids,tt.uids){t.Fatalf("headings %v Issues %v",headings,uids)}
 })}
}
