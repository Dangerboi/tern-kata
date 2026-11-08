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
	"os"
	"path/filepath"
	"slices"
	"strings"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// HostIDFile holds the Host id in tern-kata's state folder.
const HostIDFile = "host-id"

// ReadHostID is the Host id kept in the state folder, made on the first call: random, written once, never changed (ADR-0018).
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
	if err := os.Link(tmp.Name(), path); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", err
	}
	id, err := readHostIDFile(path)
	if err == nil && id == "" {
		err = fmt.Errorf("%s is empty", path)
	}
	return id, err
}

func readHostIDFile(path string) (string, error) {
	text, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(text)), err
}

// agentHost is this host as an Agent record names it.
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

// agentDirs are the config and state folders of agent.toml and agents.json.
type agentDirs struct{ config, state string }

// agentRecordOf is the record of entry on host. Its actor is the entry's, else the Address's (<actor>/<name>-<block>).
func agentRecordOf(host agentHost, e model.AgentEntry) model.AgentRecord {
	actor := e.Actor
	if i := strings.LastIndex(e.Address, "/"); actor == "" && i > 0 {
		actor = e.Address[:i]
	}
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

// issueRecords reads the keys of an Issue's metadata that hold Agent records, as stored.
func (c *Client) issueRecords(ctx context.Context, projectID int64, uid string) (map[string]json.RawMessage, error) {
	r, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: projectID, Ref: uid}})
	if err != nil {
		return nil, daemonError(err)
	}
	var body struct {
		Issue struct {
			Metadata map[string]json.RawMessage `json:"metadata"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(r.Body, &body); err != nil {
		return nil, err
	}
	return recordsOf(body.Issue.Metadata), nil
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

// addressHolder is the host name of the record of another host id on the Issue that holds address, "" when none does.
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
	records, err := c.issueRecords(ctx, e.ProjectID, e.IssueUID)
	if err != nil {
		return "", err
	}
	if usedBy = addressHolder(records, host.ID, e.Address); usedBy != "" {
		return usedBy, nil
	}
	rec := agentRecordOf(host, e)
	key := model.AgentRecordKey(host.ID, e.Block)
	if raw, held := records[key]; held {
		if again, err := model.ReadAgentRecord(key, string(raw)); err == nil && again.Address == rec.Address && again.Host == rec.Host &&
			again.Name == rec.Name && again.Actor == rec.Actor && again.NoAddress == rec.NoAddress && again.StartedAt.Equal(rec.StartedAt) &&
			slices.Equal(again.MessageFrom, rec.MessageFrom) {
			return "", nil
		}
	}
	return "", c.patchRecords(ctx, e.ProjectID, e.IssueUID, rec.Actor, map[string]any{key: rec})
}

// removeAgentRecords removes, from the Issue, the records that carry hostID and that match picks (by key and, when it reads, value).
// A record of another host id is never removed.
func (c *Client) removeAgentRecords(ctx context.Context, projectID int64, uid, hostID string, pick func(key string, r *model.AgentRecord) bool) error {
	records, err := c.issueRecords(ctx, projectID, uid)
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

// removeAgentRecordsOf removes the record of each entry on its Issue; an entry that does not name its Project cannot be found there.
// A failure leaves the record for the next reconcile, which removes any record of this host with no entry, so it is returned for
// the caller to show and never undoes what removed the entry.
func (g *Gateway) removeAgentRecordsOf(ctx context.Context, entries []model.AgentEntry) error {
	entries = slices.DeleteFunc(slices.Clone(entries), func(e model.AgentEntry) bool { return e.ProjectID == 0 || e.IssueUID == "" })
	if len(entries) == 0 {
		return nil
	}
	state, err := g.stateDir()
	if err != nil {
		return err
	}
	hostID, err := ReadHostID(state)
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		key := model.AgentRecordKey(hostID, e.Block)
		errs = append(errs, g.withDaemon(ctx, "", func(c *Client) error {
			return c.removeAgentRecords(ctx, e.ProjectID, e.IssueUID, hostID, func(k string, _ *model.AgentRecord) bool { return k == key })
		}))
	}
	return errors.Join(errs...)
}

// removeAgentRecordAt removes the record this host id published for the agent at address on an Issue: the exit of an agent.
func (g *Gateway) removeAgentRecordAt(ctx context.Context, c *Client, projectID int64, uid, address string) error {
	state, err := g.stateDir()
	if err != nil {
		return err
	}
	hostID, err := ReadHostID(state)
	if err != nil {
		return err
	}
	return c.removeAgentRecords(ctx, projectID, uid, hostID, func(_ string, r *model.AgentRecord) bool { return r != nil && r.Address == address })
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

// noteAddressHolder keeps usedBy as the reason entry e publishes no record ("" clears an earlier one), in agents.json as it stands.
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

// issueRecordsListed is what a read of every Issue found of Agent records: per Issue its Project and the record keys it holds.
type issueRecordsListed struct {
	projectID int64
	records   map[string]json.RawMessage
}

// listIssueRecords reads every Issue of the daemon, closed ones too, with the records each holds, as ListAttention reads the
// requests: one read of all Projects.
func (c *Client) listIssueRecords(ctx context.Context) (map[string]issueRecordsListed, error) {
	unlimited := int64(0)
	listed, err := c.ListAllIssuesWithResponse(ctx, &g.ListAllIssuesRequestOptions{Query: &g.ListAllIssuesQuery{Limit: &unlimited}})
	if err != nil {
		return nil, daemonError(err)
	}
	var body struct {
		Issues []struct {
			UID       string                     `json:"uid"`
			ProjectID int64                      `json:"project_id"`
			Metadata  map[string]json.RawMessage `json:"metadata"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(listed.Body, &body); err != nil {
		return nil, err
	}
	issues := map[string]issueRecordsListed{}
	for _, issue := range body.Issues {
		issues[issue.UID] = issueRecordsListed{projectID: issue.ProjectID, records: recordsOf(issue.Metadata)}
	}
	return issues, nil
}

// syncAgentRecords is the Pane's reconcile of its records with agents.json (entries as just read): a mapped agent that has no
// record gets one (written, unless another host id's record holds its Address, which is kept on the entry), and a record carrying
// this host id with no entry goes. An agent whose state says it exited is not published again: its record went with its address.
// Records of other host ids are never touched.
func (g *Gateway) syncAgentRecords(ctx context.Context, workspace string, dirs agentDirs, file string, entries []model.AgentEntry) error {
	host, err := g.hostOf(dirs)
	if err != nil {
		return err
	}
	states, _ := readAgentStates(filepath.Join(dirs.state, AgentStatesFile))
	exited := map[string]bool{}
	for _, s := range states {
		exited[s.Block] = s.State == model.AgentExited
	}
	mapped := map[string]model.AgentEntry{}
	for _, e := range entries {
		mapped[e.Block] = e
	}
	var errs []error
	err = g.withDaemon(ctx, workspace, func(c *Client) error {
		issues, err := c.listIssueRecords(ctx)
		if err != nil {
			return err
		}
		for uid, issue := range issues {
			errs = append(errs, c.dropAgentRecords(ctx, issue.projectID, uid, host.ID, issue.records, func(key string, _ *model.AgentRecord) bool {
				_, block, _ := model.ParseAgentRecordKey(key)
				_, held := mapped[block]
				return !held
			}))
		}
		for _, e := range entries {
			if e.ProjectID == 0 || e.IssueUID == "" || exited[e.Block] {
				continue
			}
			if issue, ok := issues[e.IssueUID]; ok {
				if _, held := issue.records[model.AgentRecordKey(host.ID, e.Block)]; held {
					continue
				}
			}
			usedBy, err := c.publishAgentRecord(ctx, host, e)
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
