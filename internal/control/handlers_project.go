package control

// handlers_project.go — IPC verb handlers for the project.*
// mutation/read surface. Mirrors the CLI flag shape so an agent
// uses the same vocabulary in either transport.
//
// All routes bypass the TUI tea.Cmd channel: project ops only
// touch devprojects.db. Writes go through withWriteLock for
// single-writer semantics.

import (
	"encoding/json"
)

func (s *Server) handleProjectList(req Request, w *json.Encoder) {
	out, err := s.Backend.ProjectList(ProjectListArgs{})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"projects": out, "count": len(out)}))
}

func (s *Server) handleProjectShow(req Request, w *json.Encoder) {
	var args ProjectShowArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == "" && args.Name == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.id or args.name required", nil))
		return
	}
	out, err := s.Backend.ProjectShow(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{"project": out}))
}

func (s *Server) handleProjectCreate(req Request, w *json.Encoder) {
	var args ProjectCreateArgs
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
		out, perr = s.Backend.ProjectCreate(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"project": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleProjectUpdate(req Request, w *json.Encoder) {
	var args ProjectUpdateArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.id required", nil))
		return
	}
	if args.Name == nil && args.Description == nil {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.name or args.description required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.ProjectUpdate(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"project": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleProjectDelete(req Request, w *json.Encoder) {
	var args ProjectDeleteArgs
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
		out, perr = s.Backend.ProjectDelete(args)
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

func (s *Server) handleProjectAddItem(req Request, w *json.Encoder) {
	var args ProjectAddItemArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ProjectID == "" || args.Kind == "" || args.Ref == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.project_id, args.kind, args.ref required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.ProjectAddItem(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"item": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleProjectRemoveItem(req Request, w *json.Encoder) {
	var args ProjectRemoveItemArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ProjectID == "" || args.Kind == "" || args.Ref == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.project_id, args.kind, args.ref required", nil))
		return
	}
	var out any
	err := s.withWriteLock(func() error {
		var perr error
		out, perr = s.Backend.ProjectRemoveItem(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"item": out})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handleProjectItems(req Request, w *json.Encoder) {
	var args ProjectItemsArgs
	if err := decodeArgs(req.Args, &args); err != nil {
		_ = w.Encode(fail(req, ErrInvalidArgument, err.Error(), nil))
		return
	}
	if args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument, "args.id required", nil))
		return
	}
	out, err := s.Backend.ProjectItems(args)
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(success(req, map[string]any{
		"project_id": args.ID,
		"items":      out,
		"count":      len(out),
	}))
}
