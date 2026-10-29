package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// viewsEnv is what the views commands need beyond the plan: views.toml, and the daemon for the Project catalog and Issues.
type viewsEnv struct {
	plan      LaunchPlan
	workspace string
	store     func() (data.ViewsStore, error)
	send      func(model.Cmd) model.Msg
	out       io.Writer
}

// runViews is `tern-kata views`: list the views, show one, add one or remove one. It needs no Binding, since views span Projects.
// A name that is no view is a usageError (exit 2); every other refusal is a subcommandError (exit 1).
func runViews(plan LaunchPlan, workspace string, store func() (data.ViewsStore, error), send func(model.Cmd) model.Msg, out io.Writer) error {
	e := viewsEnv{plan: plan, workspace: workspace, store: store, send: send, out: out}
	if len(plan.Operands) == 0 {
		return e.list()
	}
	name := plan.Operands[1]
	switch plan.Operands[0] {
	case "show":
		return e.show(name)
	case "add":
		return e.add(name)
	default:
		return e.remove(name)
	}
}

func (e viewsEnv) say(line string) error {
	if _, err := fmt.Fprintln(e.out, line); err != nil {
		return subcommandError{err}
	}
	return nil
}

// read is views.toml's views; an unreadable file is the error, naming the file and why.
func (e viewsEnv) read() (data.ViewsStore, data.ViewsRead, error) {
	store, err := e.store()
	if err != nil {
		return store, data.ViewsRead{}, err
	}
	read, err := store.Read()
	return store, read, err
}

// catalog is the Projects the daemon knows, from one read of the Inbox snapshot.
func (e viewsEnv) catalog() ([]model.CatalogProject, error) {
	loaded, err := e.snapshot(model.InboxCollection(), model.Filters{})
	return loaded.Snapshot.Catalog, err
}

// snapshot reads the Collection snapshot the Pane reads for c and f, always whole (an empty ETag).
func (e viewsEnv) snapshot(c model.Collection, f model.Filters) (model.SnapshotLoaded, error) {
	switch msg := e.send(model.ReadSnapshot{Workspace: e.workspace, Query: model.CollectionQuery(c, f)}).(type) {
	case model.SnapshotLoaded:
		return msg, nil
	case model.SnapshotFailed:
		return model.SnapshotLoaded{}, msg.Err
	default:
		return model.SnapshotLoaded{}, fmt.Errorf("unexpected answer %T to a snapshot read", msg)
	}
}

// names lists the built-in keys and the saved views' names, for the message of an unknown name.
func names(views []model.SavedView) string {
	var all []string
	for _, c := range model.FixedCollections() {
		all = append(all, c.View)
	}
	for _, v := range views {
		all = append(all, v.Name)
	}
	return strings.Join(all, ", ")
}

// describeFilters is the filters that are set, as name=value words.
func describeFilters(f model.Filters, projectName func(uid string) string) string {
	var words []string
	if f.ProjectUID != "" {
		words = append(words, "project="+projectName(f.ProjectUID))
	}
	if f.Status != "" {
		words = append(words, "status="+f.Status)
	}
	if f.Owner != "" {
		words = append(words, "owner="+f.Owner)
	}
	if f.Label != "" {
		words = append(words, "label="+f.Label)
	}
	if f.Relationship != "" {
		words = append(words, "relationship="+f.Relationship)
	}
	if f.Search != "" {
		words = append(words, fmt.Sprintf("search=%q", f.Search))
	}
	return strings.Join(words, " ")
}

// describeBase is a Collection's title, or `project <name>` for a Project, saying when the catalog no longer has it live.
// A nil catalog says nothing about the Project.
func describeBase(c model.Collection, catalog []model.CatalogProject, known bool) string {
	if !c.IsProject() {
		return c.Title
	}
	text := "project " + c.Title
	if !known {
		return text
	}
	switch entry, ok := model.CatalogEntry(model.Snapshot{Catalog: catalog}, c.ProjectUID); {
	case !ok:
		return text + " (Project gone)"
	case entry.Project.Archived:
		return text + " (Project archived)"
	}
	return text
}

// list prints the built-in Collections, then the saved views. An unreadable views.toml is reported after the built-ins.
func (e viewsEnv) list() error {
	_, read, readErr := e.read()
	needsCatalog := slices.ContainsFunc(read.Views, func(v model.SavedView) bool { return v.Base.IsProject() || v.Filters.ProjectUID != "" })
	var catalog []model.CatalogProject
	known := false
	if needsCatalog {
		// The Project note is left out when the daemon cannot say; the list is of this machine's file.
		if c, err := e.catalog(); err == nil {
			catalog, known = c, true
		}
	}
	projectName := func(uid string) string {
		if entry, ok := model.CatalogEntry(model.Snapshot{Catalog: catalog}, uid); ok {
			return entry.Project.Name
		}
		return uid
	}
	width := 0
	for _, c := range model.FixedCollections() {
		width = max(width, len(c.View))
	}
	for _, v := range read.Views {
		width = max(width, len(v.Name))
	}
	if err := e.say("Built-in Collections:"); err != nil {
		return err
	}
	for _, c := range model.FixedCollections() {
		if err := e.say(strings.TrimRight(fmt.Sprintf("  %-*s  %s", width, c.View, c.Title), " ")); err != nil {
			return err
		}
	}
	if readErr == nil {
		if len(read.Views) == 0 {
			if err := e.say("Saved views: none (add one with `tern-kata views add <name>`)"); err != nil {
				return err
			}
		} else if err := e.say("Saved views:"); err != nil {
			return err
		}
	}
	for _, v := range read.Views {
		line := fmt.Sprintf("  %-*s  %s  %s", width, v.Name, describeBase(v.Base, catalog, known), describeFilters(v.Filters, projectName))
		if err := e.say(strings.TrimRight(line, " ")); err != nil {
			return err
		}
	}
	for _, problem := range read.Problems {
		if err := e.say("Not listed: " + problem.Error()); err != nil {
			return err
		}
	}
	if readErr != nil {
		return subcommandError{readErr}
	}
	return nil
}

// shownGroup and shownIssue are `views show --json`: one object per group, with the Issues in the Pane's order.
type shownGroup struct {
	ID     string       `json:"id"`
	Title  string       `json:"title"`
	Issues []shownIssue `json:"issues"`
}

type shownIssue struct {
	Ref         string   `json:"ref"`
	UID         string   `json:"uid"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Priority    *int64   `json:"priority"`
	Owner       *string  `json:"owner"`
	Labels      []string `json:"labels"`
	ScheduledOn *string  `json:"scheduled_on"`
	DeadlineOn  *string  `json:"deadline_on"`
	ClosedOn    *string  `json:"closed_on"`
}

func dayOrNil(day string) *string {
	if day == "" {
		return nil
	}
	return &day
}

// show prints the Issues of a built-in Collection or saved view, grouped as the Pane groups them.
func (e viewsEnv) show(name string) error {
	_, read, readErr := e.read()
	ref, ok := model.ResolveViewName(name, read.Views)
	if !ok && readErr != nil {
		return subcommandError{readErr} // the name may be a saved view the unreadable file holds
	}
	if !ok {
		return usageError{fmt.Errorf("no view named %q; the views are %s", strings.TrimSpace(name), names(read.Views))}
	}
	collection, filters := model.Collection{}, model.Filters{}
	if ref.Collection != nil {
		collection = *ref.Collection
	} else {
		collection, filters = ref.View.Base, ref.View.FiltersFor(e.plan.Actor)
	}
	loaded, err := e.snapshot(collection, filters)
	if err != nil {
		return subcommandError{err}
	}
	cal := model.NewPaneCalendar(loaded.LocalDate, loaded.TimeZone, loaded.TimeZoneNote)
	groups := model.GroupCollection(collection, filters, loaded.Snapshot.Issues, cal)
	if e.plan.JSON {
		shown := make([]shownGroup, len(groups))
		for i, group := range groups {
			shown[i] = shownGroup{ID: group.ID, Title: group.Title, Issues: make([]shownIssue, len(group.Issues))}
			for j, issue := range group.Issues {
				scheduled, deadline := model.IssueDates(issue)
				closed := ""
				if issue.ClosedAt != nil {
					closed = cal.DayOf(*issue.ClosedAt)
				}
				shown[i].Issues[j] = shownIssue{Ref: issue.QualifiedID, UID: issue.UID, Title: issue.Title, Status: issue.Status,
					Priority: issue.Priority, Owner: issue.Owner, Labels: append([]string{}, issue.Labels...),
					ScheduledOn: dayOrNil(scheduled), DeadlineOn: dayOrNil(deadline), ClosedOn: dayOrNil(closed)}
			}
		}
		if err := writeJSON(e.out, shown); err != nil {
			return subcommandError{err}
		}
		return nil
	}
	if len(groups) == 0 {
		return e.say("No issues.")
	}
	dated := collection.Def().Dated
	for _, group := range groups {
		indent := ""
		if group.Heading {
			if err := e.say(group.Title); err != nil {
				return err
			}
			indent = "  "
		}
		for _, issue := range group.Issues {
			priority := "-"
			if issue.Priority != nil {
				priority = fmt.Sprintf("P%d", *issue.Priority)
			}
			words := []string{indent + issue.QualifiedID, issue.Status, priority}
			if dated {
				scheduled, deadline := model.IssueDates(issue)
				if scheduled != "" {
					words = append(words, "scheduled "+scheduled)
				}
				if deadline != "" {
					words = append(words, "deadline "+deadline)
				}
			}
			if issue.ClosedAt != nil && collection.View == "logbook" {
				words = append(words, "closed "+cal.DayOf(*issue.ClosedAt))
			}
			words = append(words, issue.Title)
			if err := e.say(strings.Join(words, "  ")); err != nil {
				return err
			}
		}
	}
	return nil
}

// project is the catalog's live or archived Project called name, ignoring case; the refusal lists the names when it is unknown or ambiguous.
func project(catalog []model.CatalogProject, name string) (model.Project, error) {
	name = strings.TrimSpace(name)
	var found []model.Project
	var known []string
	for _, entry := range catalog {
		known = append(known, entry.Project.Name)
		if strings.EqualFold(entry.Project.Name, name) {
			found = append(found, entry.Project)
		}
	}
	switch {
	case len(found) == 1:
		return found[0], nil
	case len(found) > 1:
		return model.Project{}, fmt.Errorf("Project %q names more than one Project", name)
	case name == "":
		return model.Project{}, errors.New("Project is required")
	}
	return model.Project{}, fmt.Errorf("Project %q is not a Project of this daemon; the Projects are %s", name, strings.Join(known, ", "))
}

// view is the saved view the flags of `views add` describe, its Projects looked up in the daemon's catalog when it names one.
func (e viewsEnv) view(name string) (v model.SavedView, filterProject string, err error) {
	flags := e.plan.ViewFlags
	v = model.SavedView{Name: name, Filters: model.Filters{Status: flags["status"], Owner: flags["owner"], Label: flags["label"], Relationship: flags["relationship"], Search: flags["search"]}}
	if v.Filters.Status == "all" {
		v.Filters.Status = ""
	}
	base, hasBase := flags["base"]
	if !hasBase {
		base = "all-open"
	}
	projectBase, isProjectBase := strings.CutPrefix(base, "project:")
	if !isProjectBase {
		c, ok := model.FixedCollectionNamed(base)
		if !ok {
			return v, "", fmt.Errorf("Base %q is not a Collection (%s) or project:<name>", base, names(nil))
		}
		v.Base = c
	}
	_, hasProject := flags["project"]
	if !isProjectBase && !hasProject {
		return v, "", nil
	}
	catalog, err := e.catalog()
	if err != nil {
		return v, "", err
	}
	if isProjectBase {
		p, err := project(catalog, projectBase)
		if err != nil {
			return v, "", fmt.Errorf("Base: %w", err)
		}
		v.Base = model.ProjectCollection(p.UID, p.Name)
	}
	if hasProject {
		p, err := project(catalog, flags["project"])
		if err != nil {
			return v, "", err
		}
		if isProjectBase && p.UID != v.Base.ProjectUID {
			return v, "", fmt.Errorf("Project %q is not the base Project %q, so no Issue would match", p.Name, v.Base.Title)
		}
		v.Filters.ProjectUID, filterProject = p.UID, p.Name
	}
	return v, filterProject, nil
}

// add saves the view the flags describe, refusing an existing name without --replace.
func (e viewsEnv) add(name string) error {
	store, read, err := e.read()
	if err != nil {
		return subcommandError{err}
	}
	existing := ""
	for _, v := range read.Views {
		if strings.EqualFold(v.Name, strings.TrimSpace(name)) {
			existing = v.Name
		}
	}
	if existing != "" && !e.plan.Replace {
		return subcommandError{fmt.Errorf("a view named %q exists; --replace saves over it", existing)}
	}
	v, filterProject, err := e.view(name)
	if err != nil {
		return subcommandError{err}
	}
	if v, err = model.CheckSavedView(v, read.Views, existing); err != nil {
		return subcommandError{err}
	}
	if existing != "" {
		err = store.Replace(read.Token, existing, v)
	} else {
		err = store.Add(read.Token, v)
	}
	if err != nil {
		return subcommandError{err}
	}
	verb := "Saved"
	if existing != "" {
		verb = "Replaced"
	}
	return e.say(strings.TrimSpace(fmt.Sprintf("%s view %q: %s  %s", verb, v.Name, v.Base.Title, describeFilters(v.Filters, func(string) string { return filterProject }))))
}

// remove deletes a saved view without asking.
func (e viewsEnv) remove(name string) error {
	store, read, err := e.read()
	if err != nil {
		return subcommandError{err}
	}
	ref, ok := model.ResolveViewName(name, read.Views)
	switch {
	case !ok:
		return subcommandError{fmt.Errorf("no saved view named %q; the views are %s", strings.TrimSpace(name), names(read.Views))}
	case ref.Collection != nil:
		return subcommandError{fmt.Errorf("%q is the built-in Collection %s, not a saved view", strings.TrimSpace(name), ref.Collection.Title)}
	}
	if err := store.Delete(read.Token, ref.View.Name); err != nil {
		return subcommandError{err}
	}
	return e.say(fmt.Sprintf("Removed view %q", ref.View.Name))
}
