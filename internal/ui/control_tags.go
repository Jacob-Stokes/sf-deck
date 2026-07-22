package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/control"
	"github.com/Jacob-Stokes/sf-deck/internal/services/tags"
)

// IPC verb handlers for tags: create/apply/remove/set/list/show/update/delete. Split out of control_backend.go.

func (s *ControlState) TagList(args control.TagListArgs) ([]any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	list, err := tags.List(s.devProjects, args.UsageOnly)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(list))
	for _, t := range list {
		out = append(out, t)
	}
	return out, nil
}

func (s *ControlState) TagApply(args control.TagApplyArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	res, err := tags.Apply(s.devProjects, args.TagID, args.Kind, args.Ref, args.OrgUser)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *ControlState) TagShow(args control.TagShowArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	return tags.Show(s.devProjects, args.ID, args.Name)
}

func (s *ControlState) TagCreate(args control.TagCreateArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	res, err := tags.Create(s.devProjects, tags.CreateInput{
		Name:  args.Name,
		Color: args.Color,
		Icon:  args.Icon,
	})
	if err != nil {
		return nil, err
	}
	return res.Tag, nil
}

func (s *ControlState) TagUpdate(args control.TagUpdateArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	in := tags.UpdateInput{
		Name:  args.Name,
		Color: args.Color,
		Icon:  args.Icon,
	}
	res, err := tags.Update(s.devProjects, args.ID, in)
	if err != nil {
		return nil, err
	}
	return res.Tag, nil
}

func (s *ControlState) TagDelete(args control.TagDeleteArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	res, err := tags.Delete(s.devProjects, args.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":      args.ID,
		"deleted": res.Changed,
	}, nil
}

func (s *ControlState) TagRemove(args control.TagRemoveArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	res, err := tags.Remove(s.devProjects, args.TagID, args.Kind, args.Ref, args.OrgUser)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *ControlState) TagSet(args control.TagSetArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	res, err := tags.Set(s.devProjects, args.Kind, args.Ref, args.OrgUser, args.TagIDs)
	if err != nil {
		return nil, err
	}
	return res, nil
}
