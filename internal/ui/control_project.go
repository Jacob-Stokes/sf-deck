package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/control"
	"github.com/Jacob-Stokes/sf-deck/internal/services/projects"
)

// IPC verb handlers for dev projects: CRUD + items + bundle import. Split out of control_backend.go.

func (s *ControlState) ProjectImportBundle(args control.ProjectImportBundleArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	in := projects.ImportBundleInput{
		ProjectID: args.ProjectID,
		Path:      args.Path,
		OrgUser:   args.OrgUser,
	}
	if in.OrgUser == "" && args.OrgAlias != "" {
		_, username, rerr := s.resolveBundleOrg(args.OrgAlias)
		if rerr != nil {
			return nil, rerr
		}
		in.OrgUser = username
	}
	return projects.ImportBundle(s.devProjects, in)
}

func (s *ControlState) ProjectList(_ control.ProjectListArgs) ([]any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	list, err := projects.List(s.devProjects)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(list))
	for _, p := range list {
		out = append(out, p)
	}
	return out, nil
}

func (s *ControlState) ProjectShow(args control.ProjectShowArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	return projects.Show(s.devProjects, args.ID, args.Name)
}

func (s *ControlState) ProjectCreate(args control.ProjectCreateArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	res, err := projects.Create(s.devProjects, projects.CreateInput{
		Name:        args.Name,
		Description: args.Description,
	})
	if err != nil {
		return nil, err
	}
	return res.Project, nil
}

func (s *ControlState) ProjectUpdate(args control.ProjectUpdateArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	in := projects.UpdateInput{
		Name:        args.Name,
		Description: args.Description,
	}
	res, err := projects.Update(s.devProjects, args.ID, in)
	if err != nil {
		return nil, err
	}
	return res.Project, nil
}

func (s *ControlState) ProjectDelete(args control.ProjectDeleteArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	res, err := projects.Delete(s.devProjects, args.ID, args.Force)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"project": res.Project,
		"deleted": res.Changed,
	}, nil
}

func (s *ControlState) ProjectAddItem(args control.ProjectAddItemArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	in := projects.AddItemInput{
		ProjectID: args.ProjectID,
		Kind:      args.Kind,
		Ref:       args.Ref,
		OrgUser:   args.OrgUser,
		Type:      args.Type,
		Name:      args.Name,
		Notes:     args.Notes,
		Namespace: args.Namespace,
	}

	if in.OrgUser == "" && args.OrgAlias != "" {
		_, username, err := s.resolveBundleOrg(args.OrgAlias)
		if err != nil {
			return nil, err
		}
		in.OrgUser = username
	}
	res, err := projects.AddItem(s.devProjects, in)
	if err != nil {
		return nil, err
	}
	return res.Item, nil
}

func (s *ControlState) ProjectRemoveItem(args control.ProjectRemoveItemArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	orgUser := args.OrgUser
	if orgUser == "" && args.OrgAlias != "" {
		_, username, err := s.resolveBundleOrg(args.OrgAlias)
		if err != nil {
			return nil, err
		}
		orgUser = username
	}
	res, err := projects.RemoveItem(s.devProjects, args.ProjectID, orgUser, args.Kind, args.Ref)
	if err != nil {
		return nil, err
	}
	return res.Item, nil
}

func (s *ControlState) ProjectItems(args control.ProjectItemsArgs) ([]any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	rows, err := projects.ListItems(s.devProjects, args.ID, args.OrgUser)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rows))
	for _, it := range rows {
		out = append(out, it)
	}
	return out, nil
}
