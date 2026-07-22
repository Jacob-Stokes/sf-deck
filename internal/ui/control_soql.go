package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/control"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// IPC verb handlers for SOQL: run + saved-query CRUD + history + seed. Split out of control_backend.go.

// SOQLSeed forwards a seed-the-SOQL-editor request. Runs only
// when args.Open==true OR args.Run==true (a bare seed doesn't
// require nav). The Tea update path is responsible for the
// actual editor mutation + optional run kickoff.
func (s *ControlState) SOQLSeed(args control.SOQLSeedArgs) error {
	if s == nil {
		return errors.New("control backend not initialised")
	}
	select {
	case s.writes <- controlSeedSOQLMsg{args: args}:
		return nil
	default:
		return ErrBusy
	}
}

func (s *ControlState) SOQLRun(args control.SOQLRunArgs) (any, error) {
	target, username, err := s.resolveTargetForIPC(args.OrgAlias, args.OrgUser)
	if err != nil {
		return nil, err
	}
	q := args.Query
	if q == "" && args.QueryFile != "" {
		body, rerr := readFileTrim(args.QueryFile)
		if rerr != nil {
			return nil, rerr
		}
		q = body
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, errors.New("empty query")
	}
	var (
		result sf.QueryResult
	)
	t0 := time.Now()
	if args.Limit > 0 {
		result, err = sf.QueryCapped(target, q, args.Tooling, args.Limit)
	} else {
		result, err = sf.Query(target, q, args.Tooling)
	}
	tookMs := int(time.Since(t0) / time.Millisecond)

	errMsg := ""
	rowCount := 0
	if err == nil {
		rowCount = len(result.Records)
	} else {
		errMsg = err.Error()
	}
	if s.devProjects != nil {
		_, _ = s.devProjects.LogSOQLHistory(username, q, tookMs, rowCount, errMsg)
	}
	if err != nil {
		return nil, err
	}
	truncated := args.Limit > 0 && len(result.Records) < result.TotalSize
	return map[string]any{
		"records":    result.Records,
		"total_size": result.TotalSize,
		"returned":   len(result.Records),
		"done":       result.Done,
		"tooling":    args.Tooling,
		"truncated":  truncated,
		"took_ms":    tookMs,
	}, nil
}

func (s *ControlState) SOQLHistoryList(args control.SOQLHistoryListArgs) ([]any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.devProjects.ListSOQLHistory(args.OrgUser, limit)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, r)
	}
	return out, nil
}

func (s *ControlState) SOQLSavedList(_ control.SOQLSavedListArgs) ([]any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	rows, err := s.devProjects.ListSavedQueries()
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, r)
	}
	return out, nil
}

func (s *ControlState) SOQLSavedShow(args control.SOQLSavedShowArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	id := args.ID
	if id == "" && args.Name != "" {

		rows, err := s.devProjects.ListSavedQueries()
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if r.Name == args.Name {
				id = r.ID
				break
			}
		}
		if id == "" {
			return nil, fmt.Errorf("no saved query named %q", args.Name)
		}
	}
	return s.devProjects.GetSavedQuery(id)
}

func (s *ControlState) SOQLSavedCreate(args control.SOQLSavedCreateArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	return s.devProjects.CreateSavedQuery(args.Name, args.Description, args.Body)
}

func (s *ControlState) SOQLSavedUpdate(args control.SOQLSavedUpdateArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}

	cur, err := s.devProjects.GetSavedQuery(args.ID)
	if err != nil {
		return nil, err
	}
	name, desc, body := cur.Name, cur.Description, cur.Body
	if args.Name != nil {
		name = *args.Name
	}
	if args.Description != nil {
		desc = *args.Description
	}
	if args.Body != nil {
		body = *args.Body
	}
	if err := s.devProjects.UpdateSavedQuery(args.ID, name, desc, body); err != nil {
		return nil, err
	}
	return s.devProjects.GetSavedQuery(args.ID)
}

func (s *ControlState) SOQLSavedDelete(args control.SOQLSavedDeleteArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	if err := s.devProjects.DeleteSavedQuery(args.ID); err != nil {
		return nil, err
	}
	return map[string]any{"id": args.ID, "deleted": true}, nil
}
