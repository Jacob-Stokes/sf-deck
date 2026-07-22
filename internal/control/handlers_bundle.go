package control

// handlers_bundle.go — IPC verb handlers for bundle.* + project.import-bundle.
//
// Reads (list / show / report) run inline on the listener goroutine.
// Writes (create / link / retrieve / validate / deploy / delete /
// import-bundle) go through the same withWriteLock the existing
// write verbs use — single-writer per instance.
//
// All handlers delegate to Backend, which the UI layer fulfils
// against the existing services/bundles + services/projects packages.

import (
	"encoding/json"
)

func (s *Server) handleBundleList(req Request, w *json.Encoder) {
	var args BundleListArgs
	_ = json.Unmarshal(req.Args, &args)
	out, err := s.Backend.BundleList(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"bundles": out, "count": len(out)}))
}

func (s *Server) handleBundleShow(req Request, w *json.Encoder) {
	var args BundleShowArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.id required", nil))
		return
	}
	out, err := s.Backend.BundleShow(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"bundle": out}))
}

func (s *Server) handleBundleCreate(req Request, w *json.Encoder) {
	var args BundleCreateArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ProjectID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.project_id required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.BundleCreate(args)
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

func (s *Server) handleBundleLink(req Request, w *json.Encoder) {
	var args BundleLinkArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ProjectID == "" || args.Path == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.project_id and args.path required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.BundleLink(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"bundle": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleBundleRetrieve(req Request, w *json.Encoder) {
	var args BundleRetrieveArgs
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
		out, perr = s.Backend.BundleRetrieve(args)
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

func (s *Server) handleBundleValidate(req Request, w *json.Encoder) {
	var args BundleValidateArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == "" || args.OrgAlias == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.id and args.org_alias required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.BundleValidate(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"result": out}))
}

func (s *Server) handleBundleDeploy(req Request, w *json.Encoder) {
	var args BundleDeployArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == "" || args.OrgAlias == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.id and args.org_alias required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.BundleDeploy(args)
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

func (s *Server) handleBundleReport(req Request, w *json.Encoder) {
	var args BundleReportArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == "" || args.OrgAlias == "" || args.DeployID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.id, args.org_alias and args.deploy_id required", nil))
		return
	}
	out, err := s.Backend.BundleReport(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"result": out}))
}

func (s *Server) handleBundleDelete(req Request, w *json.Encoder) {
	var args BundleDeleteArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.id required", nil))
		return
	}
	err := s.withWriteLock(func() error {
		return s.Backend.BundleDelete(args)
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"id": args.ID})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleProjectImportBundle(req Request, w *json.Encoder) {
	var args ProjectImportBundleArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ProjectID == "" || args.Path == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.project_id and args.path required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.ProjectImportBundle(args)
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

// decodeArgs is the shared json.Unmarshal wrapper that treats empty
// args as "no fields set" rather than a parse failure — the bundle
// verbs all have required fields the handler validates explicitly,
// so we want the validator's error message rather than a generic
// "unexpected end of JSON input."
func decodeArgs(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}
