package control

import (
	"context"
	"encoding/json"
	"errors"
	"net"
)

func (s *Server) handleStateGet(req Request, w *json.Encoder) {
	state, err := s.Backend.State()
	if err != nil {
		_ = w.Encode(fail(req, ErrInternal, err.Error(), nil))
		return
	}
	_ = w.Encode(success(req, state))
}

// handleStateSubscribe streams snapshots on every UI state change.
// Long-lived: holds the connection until the client closes it or
// the listener shuts down. Each snapshot is a Response with command
// "state.subscribe" and the new state under Data.
//
// Coalesces backpressure: if the writer can't keep up, the channel
// drops older snapshots in favour of newer ones (the backend
// implementation enforces this).
func (s *Server) handleStateSubscribe(ctx context.Context, req Request, conn net.Conn, w *json.Encoder) {
	ch, cancel, err := s.Backend.Subscribe()
	if err != nil {
		_ = w.Encode(fail(req, ErrInternal, err.Error(), nil))
		return
	}
	defer cancel()
	if init, err := s.Backend.State(); err == nil {
		_ = w.Encode(success(req, init))
	}
	for {
		select {
		case <-ctx.Done():
			return
		case state, ok := <-ch:
			if !ok {
				return
			}
			if err := w.Encode(success(req, state)); err != nil {
				return
			}
		}
	}
}

// handleTabOpen synthesizes a tab navigation. Single-writer-locked.
func (s *Server) handleTabOpen(req Request, w *json.Encoder) {
	var args OpenTabArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			_ = w.Encode(fail(req, ErrInvalidArgument,
				"args: "+err.Error(), nil))
			return
		}
	}
	if args.Tab == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.tab is required", nil))
		return
	}
	err := s.withWriteLock(func() error {
		return s.Backend.OpenTab(args)
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(Response{ID: req.ID, OK: true, Command: req.Command, Changed: true})
}

func (s *Server) handleChipApply(req Request, w *json.Encoder) {
	var args ApplyChipArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			_ = w.Encode(fail(req, ErrInvalidArgument,
				"args: "+err.Error(), nil))
			return
		}
	}
	if args.Domain == "" || args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.domain and args.id are required", nil))
		return
	}
	err := s.withWriteLock(func() error {
		return s.Backend.ApplyChip(args)
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(Response{ID: req.ID, OK: true, Command: req.Command, Changed: true})
}

func (s *Server) handleOrgSwitch(req Request, w *json.Encoder) {
	var args SwitchOrgArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			_ = w.Encode(fail(req, ErrInvalidArgument,
				"args: "+err.Error(), nil))
			return
		}
	}
	if args.OrgUser == "" && args.Alias == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.org_user or args.alias is required", nil))
		return
	}
	err := s.withWriteLock(func() error {
		return s.Backend.SwitchOrg(args)
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(Response{ID: req.ID, OK: true, Command: req.Command, Changed: true})
}

func (s *Server) handleProjectLoad(req Request, w *json.Encoder) {
	var args LoadProjectArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			_ = w.Encode(fail(req, ErrInvalidArgument,
				"args: "+err.Error(), nil))
			return
		}
	}
	if args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.id is required (use project.unload to clear)", nil))
		return
	}
	err := s.withWriteLock(func() error {
		return s.Backend.LoadProject(args)
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(Response{ID: req.ID, OK: true, Command: req.Command, Changed: true})
}

func (s *Server) handleProjectUnload(req Request, w *json.Encoder) {
	err := s.withWriteLock(func() error {
		return s.Backend.LoadProject(LoadProjectArgs{})
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(Response{ID: req.ID, OK: true, Command: req.Command, Changed: true})
}

func (s *Server) handlePreviewChip(req Request, w *json.Encoder) {
	var args PreviewChipArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			_ = w.Encode(fail(req, ErrInvalidArgument,
				"args: "+err.Error(), nil))
			return
		}
	}
	if args.Domain == "" || args.Label == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.domain and args.label are required", nil))
		return
	}
	var res PreviewChipResult
	err := s.withWriteLock(func() error {
		var perr error
		res, perr = s.Backend.PreviewChip(args)
		return perr
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	resp := success(req, map[string]any{"chip": res})
	resp.Changed = true
	_ = w.Encode(resp)
}

func (s *Server) handlePreviewSaveChip(req Request, w *json.Encoder) {
	var args PreviewSaveChipArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			_ = w.Encode(fail(req, ErrInvalidArgument,
				"args: "+err.Error(), nil))
			return
		}
	}
	if args.ID == "" || args.NewID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.id and args.new_id are required", nil))
		return
	}
	err := s.withWriteLock(func() error {
		return s.Backend.PreviewSaveChip(args)
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(Response{ID: req.ID, OK: true, Command: req.Command, Changed: true})
}

func (s *Server) handlePreviewDismissChip(req Request, w *json.Encoder) {
	var args PreviewDismissChipArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			_ = w.Encode(fail(req, ErrInvalidArgument,
				"args: "+err.Error(), nil))
			return
		}
	}
	if args.ID == "" {
		_ = w.Encode(fail(req, ErrInvalidArgument,
			"args.id is required", nil))
		return
	}
	err := s.withWriteLock(func() error {
		return s.Backend.PreviewDismissChip(args)
	})
	if err != nil {
		s.encodeBackendErr(req, w, err)
		return
	}
	_ = w.Encode(Response{ID: req.ID, OK: true, Command: req.Command, Changed: true})
}

func (s *Server) encodeBackendErr(req Request, w *json.Encoder, err error) {
	if errors.Is(err, errBusy) {
		_ = w.Encode(fail(req, ErrInstanceBusy,
			"another client holds the write channel", nil))
		return
	}
	type coded interface{ Code() string }
	var c coded
	if errors.As(err, &c) {
		_ = w.Encode(fail(req, c.Code(), err.Error(), nil))
		return
	}
	_ = w.Encode(fail(req, ErrInternal, err.Error(), nil))
}
