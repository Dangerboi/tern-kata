package data

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/filelock"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

const HostIDFile = "host-id"

const hostIDLockWait = 5 * time.Second

// ReadHostID is the Host id kept in the state folder, made on the first call: random, written once, never changed (ADR-0018). An empty file is made again.
// Two Panes asking at once both get the id the first one linked into place.
func ReadHostID(state string) (string, error) {
	path := filepath.Join(state, HostIDFile)
	if id, err := readHostIDFile(path); err != nil || id != "" {
		return id, err
	}
	if err := os.MkdirAll(state, 0o700); err != nil {
		return "", err
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(state, HostIDFile+"-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.WriteString(hex.EncodeToString(raw[:]) + "\n")
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	// A hard link fails when the file exists, so the id another Pane just made is never replaced.
	if err := os.Link(tmp.Name(), path); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		// The file exists. One another Pane has just filled is kept; an empty one (a crash, a truncated copy) is replaced.
		if id, err := readHostIDFile(path); err != nil || id != "" {
			return id, err
		}
		if id, replaced, err := replaceEmptyHostID(path, tmp.Name()); err != nil || !replaced {
			return id, err
		}
	}
	id, err := readHostIDFile(path)
	if err == nil && id == "" {
		err = fmt.Errorf("%s is empty", path)
	}
	return id, err
}

// replaceEmptyHostID renames the file at tmp over the empty one at path, under a lock beside it, and reports that it did. Two Panes
// both finding the file empty take the lock in turn: the second reads the id the first renamed in and keeps it.
func replaceEmptyHostID(path, tmp string) (id string, replaced bool, err error) {
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", false, err
	}
	defer lock.Close()
	if err := filelock.Wait(lock, hostIDLockWait); err != nil {
		return "", false, err
	}
	defer filelock.Unlock(lock)
	if id, err := readHostIDFile(path); err != nil || id != "" {
		return id, false, err
	}
	return "", true, os.Rename(tmp, path)
}

// ThisHostID is the Host id of this machine's tern-kata state folder, "" when the folder cannot be read or made; the Pane then
// lists no remote agents, for it could not tell its own records from theirs.
func ThisHostID() string {
	dirs, err := worker.DefaultDirs()
	if err != nil || dirs.State == "" {
		return ""
	}
	id, _ := ReadHostID(dirs.State)
	return id
}

func readHostIDFile(path string) (string, error) {
	text, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(text)), err
}

type agentHost struct {
	ID, Name string
}

// hostOf reads the Host id and the host name: agent.toml's host, else this machine's host name. A config that does not read
// names the machine, for the record is display only and a start has already refused such a file.
func (g *Gateway) hostOf(dirs agentDirs) (agentHost, error) {
	id, err := ReadHostID(dirs.state)
	if err != nil {
		return agentHost{}, fmt.Errorf("read the host id: %w", err)
	}
	cfg, _ := ReadAgentConfig(filepath.Join(dirs.config, AgentConfigFile))
	return agentHost{ID: id, Name: cmp.Or(cfg.Host, g.host, HostName())}, nil
}

type agentDirs struct{ config, state string }

// Its actor is the entry's, else the Address's.
func agentRecordOf(host agentHost, e model.AgentEntry) model.AgentRecord {
	actor := cmp.Or(e.Actor, AddressActor(e.Address))
	from := e.MessageFromActors()
	if from == nil {
		from = []string{}
	}
	return model.AgentRecord{Address: e.Address, Host: host.Name, HostID: host.ID, Block: e.Block, Name: e.Teammate, Actor: actor,
		StartedAt: e.StartedAt, NoAddress: e.NoAddress, MessageFrom: from}
}

// withDaemon runs fn against the daemon of workspace. A Gateway built without a Locator has no daemon (the tests of the local
// steps), so nothing is published or removed there.
func (g *Gateway) withDaemon(ctx context.Context, workspace string, fn func(*Client) error) error {
	if g.locate == nil {
		return nil
	}
	_, err := g.call(ctx, workspace, func(ctx context.Context, c *Client) (model.Msg, error) { return nil, fn(c) })
	return err
}

// readMetadata reads an Issue's metadata from the body as sent, so a guard holds the value kata stores, not a re-encoding of a
// decoded one; found is false when kata has no such Issue.
func (c *Client) readMetadata(ctx context.Context, projectID int64, uid string) (metadata map[string]json.RawMessage, found bool, err error) {
	r, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: projectID, Ref: uid}})
	if client.StatusCode(err) == http.StatusNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, daemonError(err)
	}
	var body struct {
		Issue struct {
			Metadata map[string]json.RawMessage `json:"metadata"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(r.Body, &body); err != nil {
		return nil, false, err
	}
	return body.Issue.Metadata, true, nil
}

// readIssueRecords reads the keys of an Issue's metadata that hold Agent records, as stored, and tells whether kata has the Issue;
// one it no longer has holds none.
func (c *Client) readIssueRecords(ctx context.Context, projectID int64, uid string) (map[string]json.RawMessage, bool, error) {
	metadata, found, err := c.readMetadata(ctx, projectID, uid)
	if err != nil {
		return nil, false, err
	}
	return recordsOf(metadata), found, nil
}

func recordsOf(metadata map[string]json.RawMessage) map[string]json.RawMessage {
	records := map[string]json.RawMessage{}
	for key, raw := range metadata {
		if strings.HasPrefix(key, model.AgentRecordPrefix) {
			records[key] = raw
		}
	}
	return records
}

// patchRecords writes keys of an Issue's metadata, a nil value removing one, as actor ("" leaves the writer to kata's default).
func (c *Client) patchRecords(ctx context.Context, projectID int64, uid, actor string, patch map[string]any) error {
	body := g.PatchIssueMetadataBody{Patch: patch}
	if actor != "" {
		body.Actor = &actor
	}
	_, err := c.PatchIssueMetadataWithResponse(ctx, &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: projectID, Ref: uid}, Body: &body})
	return daemonError(err)
}

// ForgetAgentRecord removes the record of an agent on another host, whichever host id it carries, and the message left at its
// address, which nobody reads again. The message goes first, so a failure leaves the record for another Forget. The other host is
// never touched; one of its Panes finding the agent alive again publishes the record anew on its next reconcile.
func (c *Client) ForgetAgentRecord(ctx context.Context, cmd model.ForgetAgentRecord) error {
	if cmd.Address != "" {
		if err := c.ClearAgentAddress(ctx, model.ClearAgentAddress{ProjectID: cmd.ProjectID, UID: cmd.UID, Actor: cmd.Actor, Address: cmd.Address}); err != nil {
			return err
		}
	}
	return c.patchRecords(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, map[string]any{model.AgentRecordKey(cmd.HostID, cmd.Block): nil})
}

func addressHolder(records map[string]json.RawMessage, hostID, address string) string {
	if address == "" {
		return ""
	}
	for _, key := range slices.Sorted(maps.Keys(records)) {
		if keyHost, _, ok := model.ParseAgentRecordKey(key); !ok || keyHost == hostID {
			continue
		}
		if r, err := model.ReadAgentRecord(key, string(records[key])); err == nil && r.Address == address {
			return cmp.Or(r.Host, r.HostID)
		}
	}
	return ""
}

// publishAgentRecord writes the record of e on its Issue, unless another host id's record on the Issue holds the same Address: then
// nothing is written and the answer is that record's host name. A record already there for e is written again as it should be.
func (c *Client) publishAgentRecord(ctx context.Context, host agentHost, e model.AgentEntry) (usedBy string, err error) {
	records, _, err := c.readIssueRecords(ctx, e.ProjectID, e.IssueUID)
	if err != nil {
		return "", err
	}
	usedBy, write := planAgentRecord(records, host, e)
	if !write {
		return usedBy, nil
	}
	rec := agentRecordOf(host, e)
	return "", c.patchRecords(ctx, e.ProjectID, e.IssueUID, rec.Actor, map[string]any{model.AgentRecordKey(host.ID, e.Block): rec})
}

func planAgentRecord(records map[string]json.RawMessage, host agentHost, e model.AgentEntry) (usedBy string, write bool) {
	if usedBy = addressHolder(records, host.ID, e.Address); usedBy != "" {
		return usedBy, false
	}
	rec := agentRecordOf(host, e)
	key := model.AgentRecordKey(host.ID, e.Block)
	if raw, held := records[key]; held {
		if again, err := model.ReadAgentRecord(key, string(raw)); err == nil && again.Address == rec.Address && again.Host == rec.Host &&
			again.Name == rec.Name && again.Actor == rec.Actor && again.NoAddress == rec.NoAddress && again.StartedAt.Equal(rec.StartedAt) &&
			slices.Equal(again.MessageFrom, rec.MessageFrom) {
			return "", false
		}
	}
	return "", true
}

// removeAgentRecords removes, from the Issue, the records that carry hostID and that match picks (by key and, when it reads, value).
// A record of another host id is never removed.
func (c *Client) removeAgentRecords(ctx context.Context, projectID int64, uid, hostID string, pick func(key string, r *model.AgentRecord) bool) error {
	records, _, err := c.readIssueRecords(ctx, projectID, uid)
	if err != nil {
		return err
	}
	return c.dropAgentRecords(ctx, projectID, uid, hostID, records, pick)
}

func (c *Client) dropAgentRecords(ctx context.Context, projectID int64, uid, hostID string, records map[string]json.RawMessage, pick func(key string, r *model.AgentRecord) bool) error {
	var errs []error
	for _, key := range slices.Sorted(maps.Keys(records)) {
		if keyHost, _, ok := model.ParseAgentRecordKey(key); !ok || keyHost != hostID {
			continue
		}
		var rec *model.AgentRecord
		actor := ""
		if r, err := model.ReadAgentRecord(key, string(records[key])); err == nil {
			rec, actor = &r, r.Actor
		}
		if pick(key, rec) {
			errs = append(errs, c.patchRecords(ctx, projectID, uid, actor, map[string]any{key: nil}))
		}
	}
	return errors.Join(errs...)
}

// removeAgentRecordsOf clears what is left at the Agent address of each entry on its Issue, nobody reading it again, then removes
// its record; an entry that does not name its Project cannot be found there. The record stays when the clear failed, so the
// failure is returned and the message is not lost with the only note of where it is. A failure leaves the record for the next reconcile, which removes any record of this host with no entry, so it is returned for
// the caller to show and never undoes what removed the entry.
func (g *Gateway) removeAgentRecordsOf(ctx context.Context, entries []model.AgentEntry) error {
	entries = slices.DeleteFunc(slices.Clone(entries), func(e model.AgentEntry) bool { return e.ProjectID == 0 || e.IssueUID == "" })
	if len(entries) == 0 {
		return nil
	}
	hostID, err := g.thisHostID()
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		errs = append(errs, g.withDaemon(ctx, "", func(c *Client) error {
			if e.Address != "" {
				if err := c.ClearAgentAddress(ctx, model.ClearAgentAddress{ProjectID: e.ProjectID, UID: e.IssueUID, Actor: model.AgentAddressActor(e.Address), Address: e.Address}); err != nil {
					return err
				}
			}
			return c.removeHostRecord(ctx, e.ProjectID, e.IssueUID, hostID, e.Block)
		}))
	}
	return errors.Join(errs...)
}

func (g *Gateway) removeAgentRecordOf(ctx context.Context, c *Client, projectID int64, uid, block string) error {
	hostID, err := g.thisHostID()
	if err != nil {
		return err
	}
	return c.removeHostRecord(ctx, projectID, uid, hostID, block)
}

func (c *Client) removeHostRecord(ctx context.Context, projectID int64, uid, hostID, block string) error {
	key := model.AgentRecordKey(hostID, block)
	return c.removeAgentRecords(ctx, projectID, uid, hostID, func(k string, _ *model.AgentRecord) bool { return k == key })
}

func (g *Gateway) thisHostID() (string, error) {
	state, err := g.stateDir()
	if err != nil {
		return "", err
	}
	return ReadHostID(state)
}

func (g *Gateway) stateDir() (string, error) {
	dirs, err := g.workerDirs()
	if dirs.State == "" {
		return "", cmp.Or(err, errors.New("tern-kata's state folder is not known"))
	}
	return dirs.State, nil
}

// publishAgent publishes the record of the entry just recorded in agents.json and keeps, on the entry, which host's record held its
// Address when one did. It returns the entry as it stands.
func (g *Gateway) publishAgent(ctx context.Context, workspace string, dirs agentDirs, file string, e model.AgentEntry) (model.AgentEntry, error) {
	host, err := g.hostOf(dirs)
	if err != nil {
		return e, err
	}
	var usedBy string
	err = g.withDaemon(ctx, workspace, func(c *Client) (err error) {
		usedBy, err = c.publishAgentRecord(ctx, host, e)
		return err
	})
	if err != nil {
		return e, err
	}
	return g.noteAddressHolder(file, e, usedBy)
}

func (g *Gateway) noteAddressHolder(file string, e model.AgentEntry, usedBy string) (model.AgentEntry, error) {
	if e.AddressUsedBy == usedBy {
		return e, nil
	}
	e.AddressUsedBy = usedBy
	_, err := UpdateAgents(file, func(list []model.AgentEntry) []model.AgentEntry {
		for i := range list {
			if list[i].Block == e.Block {
				list[i].AddressUsedBy = usedBy
			}
		}
		return list
	})
	return e, err
}

type issueRecordsListed struct {
	projectID int64
	records   map[string]json.RawMessage
	missing   bool
}

// listIssueRecords reads the Issues the Pane shows, as its Project's open Issues page by page (none when the workspace is bound
// to no Project), and the Issues of entries, with the records each holds. Nothing else of the daemon is read.
func (c *Client) listIssueRecords(ctx context.Context, workspace string, entries []model.AgentEntry) (map[string]issueRecordsListed, error) {
	issues := map[string]issueRecordsListed{}
	// A bound Project this daemon has not initialised is left unresolved: the entries' Issues are still reconciled.
	loaded, err := c.readProject(ctx, workspace, true)
	if err != nil {
		return nil, err
	}
	if loaded.Project.UID != "" {
		err := c.eachOpenIssuePage(ctx, loaded.Project.ID, func(page *g.ListIssuesResp) error {
			var body struct {
				Issues []struct {
					UID       string                     `json:"uid"`
					ProjectID int64                      `json:"project_id"`
					Metadata  map[string]json.RawMessage `json:"metadata"`
				} `json:"issues"`
			}
			if err := json.Unmarshal(page.Body, &body); err != nil {
				return err
			}
			for _, issue := range body.Issues {
				issues[issue.UID] = issueRecordsListed{projectID: issue.ProjectID, records: recordsOf(issue.Metadata)}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	for _, e := range entries {
		if _, seen := issues[e.IssueUID]; seen || e.ProjectID == 0 || e.IssueUID == "" {
			continue
		}
		records, found, err := c.readIssueRecords(ctx, e.ProjectID, e.IssueUID)
		if err != nil {
			continue // an Issue that cannot be read now keeps its records until the next reconcile
		}
		issues[e.IssueUID] = issueRecordsListed{projectID: e.ProjectID, records: records, missing: !found}
	}
	return issues, nil
}

// syncAgentRecords is the Pane's reconcile of its records with agents.json (read again after the slow listing of records, so an
// agent started meanwhile keeps its record): a mapped agent whose record is missing or differs gets one (written, unless another host id's record holds its Address, which is kept on the entry), and a record carrying
// this host id with no entry, or whose agent exited, goes. An agent whose state says it exited is not published again: its record went with its address.
// Records of other host ids are never touched.
func (g *Gateway) syncAgentRecords(ctx context.Context, workspace string, dirs agentDirs, file string) error {
	host, err := g.hostOf(dirs)
	if err != nil {
		return err
	}
	states, _ := readAgentStates(filepath.Join(dirs.state, AgentStatesFile))
	exited := map[string]bool{}
	for _, s := range states {
		exited[s.Block] = s.State == model.AgentExited
	}
	var errs []error
	err = g.withDaemon(ctx, workspace, func(c *Client) error {
		listed, err := ReadAgents(file)
		if err != nil {
			return err
		}
		issues, err := c.listIssueRecords(ctx, workspace, listed)
		if err != nil {
			return err
		}
		// Read again after the slow listing: an agent another Pane started meanwhile has its record, which no entry of the first read holds.
		fresh, err := ReadAgents(file)
		if err != nil {
			return err
		}
		mapped := map[string]model.AgentEntry{}
		for _, e := range fresh {
			mapped[e.Block] = e
		}
		for uid, issue := range issues {
			errs = append(errs, c.dropAgentRecords(ctx, issue.projectID, uid, host.ID, issue.records, func(key string, _ *model.AgentRecord) bool {
				_, block, _ := model.ParseAgentRecordKey(key)
				_, held := mapped[block]
				return !held || exited[block]
			}))
		}
		for _, e := range fresh {
			if e.ProjectID == 0 || e.IssueUID == "" || exited[e.Block] {
				continue
			}
			usedBy, skip, err := c.publishListed(ctx, host, e, issues[e.IssueUID])
			if skip {
				continue
			}
			if err != nil {
				errs = append(errs, err)
				continue
			}
			_, err = g.noteAddressHolder(file, e, usedBy)
			errs = append(errs, err)
		}
		return nil
	})
	return errors.Join(append(errs, err)...)
}

// publishListed publishes e given what a listing found on its Issue (the zero value when the listing did not read it). The listing
// is seconds old: a record that must be written is decided again over a read taken now, so a record of another host that appeared
// since still keeps the Address from being published twice. Nothing is read or written when the listing shows the record in place.
// skip is true for an Issue kata no longer has.
func (c *Client) publishListed(ctx context.Context, host agentHost, e model.AgentEntry, listed issueRecordsListed) (usedBy string, skip bool, err error) {
	if listed.records == nil {
		usedBy, err = c.publishAgentRecord(ctx, host, e)
		return usedBy, false, err
	}
	if listed.missing {
		return "", true, nil
	}
	usedBy, write := planAgentRecord(listed.records, host, e)
	if !write {
		return usedBy, false, nil
	}
	usedBy, err = c.publishAgentRecord(ctx, host, e)
	return usedBy, false, err
}
