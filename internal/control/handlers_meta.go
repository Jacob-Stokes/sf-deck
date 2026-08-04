package control

// handlers_meta.go — IPC verb handlers for the schema /
// description nouns: metadata.* (Tooling API CRUD),
// object.describe, tag.*, org.safety.*.

import (
	"encoding/json"
)

func (s *Server) handleMetadataGet(req Request, w *json.Encoder) {
	var args MetadataGetArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.Type == "" || (args.ID == "" && args.FullName == "") {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.type plus args.id or args.full_name required", nil))
		return
	}
	out, err := s.Backend.MetadataGet(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"result": out}))
}

func (s *Server) handleMetadataCreate(req Request, w *json.Encoder) {
	var args MetadataCreateArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.Type == "" || args.FullName == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.type and args.full_name required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.MetadataCreate(args)
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

func (s *Server) handleMetadataUpdate(req Request, w *json.Encoder) {
	var args MetadataUpdateArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.Type == "" || args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.type and args.id required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.MetadataUpdate(args)
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

func (s *Server) handleMetadataDelete(req Request, w *json.Encoder) {
	var args MetadataDeleteArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.Type == "" || args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.type and args.id required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.MetadataDelete(args)
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

func (s *Server) handleObjectDescribe(req Request, w *json.Encoder) {
	var args ObjectDescribeArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.SObject == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.sobject required", nil))
		return
	}
	out, err := s.Backend.ObjectDescribe(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"describe": out}))
}

func (s *Server) handleVerbsList(req Request, w *json.Encoder) {
	var args VerbsListArgs
	_ = decodeArgs(req.Args, &args)
	out, err := s.Backend.VerbsList(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"verbs": out, "count": len(out)}))
}

func (s *Server) handleReportList(req Request, w *json.Encoder) {
	var args ReportListArgs
	_ = decodeArgs(req.Args, &args)
	out, err := s.Backend.ReportList(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"reports": out, "count": len(out)}))
}

func (s *Server) handleReportRun(req Request, w *json.Encoder) {
	var args ReportRunArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.id required", nil))
		return
	}
	out, err := s.Backend.ReportRun(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"report": out}))
}

func (s *Server) handleTagShow(req Request, w *json.Encoder) {
	var args TagShowArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == 0 && args.Name == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.id or args.name required", nil))
		return
	}
	out, err := s.Backend.TagShow(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"tag": out}))
}

func (s *Server) handleTagCreate(req Request, w *json.Encoder) {
	var args TagCreateArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.Name == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.name required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.TagCreate(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"tag": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleTagUpdate(req Request, w *json.Encoder) {
	var args TagUpdateArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == 0 {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.id required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.TagUpdate(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"tag": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleTagDelete(req Request, w *json.Encoder) {
	var args TagDeleteArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == 0 {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.id required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.TagDelete(args)
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

func (s *Server) handleTagRemove(req Request, w *json.Encoder) {
	var args TagRemoveArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.TagID == 0 || args.Kind == "" || args.Ref == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.tag_id, args.kind, args.ref required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.TagRemove(args)
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

func (s *Server) handleTagSet(req Request, w *json.Encoder) {
	var args TagSetArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.Kind == "" || args.Ref == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.kind and args.ref required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.TagSet(args)
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

func (s *Server) handleTagList(req Request, w *json.Encoder) {
	var args TagListArgs
	_ = decodeArgs(req.Args, &args)
	out, err := s.Backend.TagList(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"tags": out, "count": len(out)}))
}

func (s *Server) handleTagApply(req Request, w *json.Encoder) {
	var args TagApplyArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.TagID == 0 || args.Kind == "" || args.Ref == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.tag_id, args.kind, args.ref required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.TagApply(args)
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

func (s *Server) handleOrgSafetyGet(req Request, w *json.Encoder) {
	var args OrgSafetyGetArgs
	_ = decodeArgs(req.Args, &args)
	out, err := s.Backend.OrgSafetyGet(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"safety": out}))
}

func (s *Server) handleOrgSafetySet(req Request, w *json.Encoder) {
	var args OrgSafetySetArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if !args.Clear && args.Level == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.level or args.clear=true required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.OrgSafetySet(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"safety": out})
	resp.Changed = true
	_ = w.Encode(resp)
}
