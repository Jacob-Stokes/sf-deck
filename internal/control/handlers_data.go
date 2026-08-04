package control

// handlers_data.go — IPC verb handlers for the data-plane nouns:
// SOQL, Apex, records. All delegate to Backend, which the UI layer
// fulfils against the internal/sf package.
//
// Reads (soql.run, apex.get, apex.log, record.get) run on the
// listener goroutine. Writes (apex.run, record.create/update/delete)
// go through withWriteLock for single-writer semantics — apex
// anonymous in particular can mutate arbitrary state.

import (
	"encoding/json"
)

func (s *Server) handleSOQLRun(req Request, w *json.Encoder) {
	var args SOQLRunArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.Query == "" && args.QueryFile == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.query or args.query_file required", nil))
		return
	}
	out, err := s.Backend.SOQLRun(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"result": out}))
}

func (s *Server) handleApexRun(req Request, w *json.Encoder) {
	var args ApexRunArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.Body == "" && args.BodyFile == "" && args.SnippetID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.body, args.body_file, or args.snippet_id required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.ApexRun(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"result": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleRecordGet(req Request, w *json.Encoder) {
	var args RecordGetArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.SObject == "" || args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.sobject and args.id required", nil))
		return
	}
	out, err := s.Backend.RecordGet(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"record": out}))
}

func (s *Server) handleSOQLSeed(req Request, w *json.Encoder) {
	var args SOQLSeedArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.Query == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.query required", nil))
		return
	}
	if err := s.Backend.SOQLSeed(args); err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"seeded": true, "run": args.Run})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleSOQLHistoryList(req Request, w *json.Encoder) {
	var args SOQLHistoryListArgs
	_ = decodeArgs(req.Args, &args)
	out, err := s.Backend.SOQLHistoryList(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"history": out, "count": len(out)}))
}

func (s *Server) handleSOQLSavedList(req Request, w *json.Encoder) {
	var args SOQLSavedListArgs
	_ = decodeArgs(req.Args, &args)
	out, err := s.Backend.SOQLSavedList(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"saved": out, "count": len(out)}))
}

func (s *Server) handleSOQLSavedShow(req Request, w *json.Encoder) {
	var args SOQLSavedShowArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == "" && args.Name == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.id or args.name required", nil))
		return
	}
	out, err := s.Backend.SOQLSavedShow(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"saved": out}))
}

func (s *Server) handleSOQLSavedCreate(req Request, w *json.Encoder) {
	var args SOQLSavedCreateArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.Name == "" || args.Body == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.name and args.body required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.SOQLSavedCreate(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"saved": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleSOQLSavedUpdate(req Request, w *json.Encoder) {
	var args SOQLSavedUpdateArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.id required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.SOQLSavedUpdate(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"saved": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleSOQLSavedDelete(req Request, w *json.Encoder) {
	var args SOQLSavedDeleteArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.id required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.SOQLSavedDelete(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"result": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleRecordRecent(req Request, w *json.Encoder) {
	var args RecordRecentArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.SObject == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.sobject required", nil))
		return
	}
	out, err := s.Backend.RecordRecent(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"result": out}))
}

func (s *Server) handleRecordCreate(req Request, w *json.Encoder) {
	var args RecordCreateArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.SObject == "" || len(args.Fields) == 0 {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.sobject and args.fields required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.RecordCreate(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"result": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleRecordDelete(req Request, w *json.Encoder) {
	var args RecordDeleteArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.SObject == "" || args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.sobject and args.id required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.RecordDelete(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"result": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleRecordUpdate(req Request, w *json.Encoder) {
	var args RecordUpdateArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.SObject == "" || args.ID == "" || len(args.Fields) == 0 {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.sobject, args.id, and args.fields required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.RecordUpdate(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"result": out})
	resp.Changed = true
	_ = w.Encode(resp)
}
