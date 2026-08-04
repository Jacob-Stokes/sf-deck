package ui

import (
	"context"

	"github.com/Jacob-Stokes/sf-deck/internal/control"
	"github.com/Jacob-Stokes/sf-deck/internal/services/bundles"
)

// IPC verb handlers for bundles: create/link/retrieve/validate/deploy/report/list/show/delete. Split out of control_backend.go.

func (s *ControlState) BundleList(args control.BundleListArgs) ([]any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	list, err := bundles.List(s.devProjects, args.ProjectID)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(list))
	for _, b := range list {
		out = append(out, b)
	}
	return out, nil
}

func (s *ControlState) BundleShow(args control.BundleShowArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	return bundles.Show(s.devProjects, args.ID)
}

func (s *ControlState) BundleCreate(args control.BundleCreateArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	in := bundles.CreateInput{
		ProjectID:    args.ProjectID,
		Path:         args.Path,
		FullProject:  args.FullProject,
		Retrieve:     false,
		ScopeAllOrgs: args.ScopeAllOrgs,
		Force:        args.Force,
	}

	target := args.OrgAlias
	if target == "" {
		target = args.OrgUser
	}
	if target != "" {
		alias, username, rerr := s.resolveBundleOrg(target)
		if rerr != nil {
			return nil, rerr
		}
		in.OrgAlias = alias
		in.OrgUser = username
	}
	res, err := bundles.Create(s.devProjects, in)
	if err != nil {
		return nil, err
	}
	if args.Retrieve {
		retrieved, retrieveErr := s.bundles.Retrieve(context.Background(), bundles.OperationInput{
			BundleID: res.Bundle.ID, Target: in.OrgAlias,
		})
		res.RetrieveOutput = retrieved.Output
		res.RetrieveErr = retrieveErr
	}
	out := map[string]any{
		"bundle":           res.Bundle,
		"package_xml_path": res.PackageXMLPath,
		"included":         res.Included,
		"records_exported": res.RecordsExported,
	}
	if len(res.UnsupportedKinds) > 0 {
		out["unsupported_kinds"] = res.UnsupportedKinds
	}
	if len(res.ManagedSkipped) > 0 {
		out["managed_skipped"] = res.ManagedSkipped
	}
	if res.RetrieveErr != nil {
		out["retrieve_error"] = res.RetrieveErr.Error()
	}
	return out, nil
}

func (s *ControlState) BundleLink(args control.BundleLinkArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	alias := args.OrgAlias
	if alias != "" {
		var rerr error
		alias, _, rerr = s.resolveBundleOrg(alias)
		if rerr != nil {
			return nil, rerr
		}
	}
	return bundles.Link(s.devProjects, args.ProjectID, args.Path, alias)
}

func (s *ControlState) BundleRetrieve(args control.BundleRetrieveArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	result, err := s.bundles.Retrieve(context.Background(), bundles.OperationInput{
		BundleID: args.ID, Target: args.OrgAlias,
	})
	if err != nil {
		return nil, encodeControlServiceError(err)
	}
	return map[string]any{"bundle_id": args.ID, "sf_output": string(result.Output)}, nil
}

func (s *ControlState) BundleValidate(args control.BundleValidateArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	opts, err := translateDeployOpts(args.Tests, args.TestClasses)
	if err != nil {
		return nil, err
	}
	var result bundles.OperationResult
	if args.Async {
		result, err = s.bundles.ValidateAsync(context.Background(), bundles.OperationInput{
			BundleID: args.ID, Target: args.OrgAlias, Opts: opts,
		})
	} else {
		result, err = s.bundles.Validate(context.Background(), bundles.OperationInput{
			BundleID: args.ID, Target: args.OrgAlias, Opts: opts,
		})
	}
	if err != nil {
		return nil, encodeControlServiceError(err)
	}
	return map[string]any{"bundle_id": args.ID, "sf_output": string(result.Output)}, nil
}

func (s *ControlState) BundleDeploy(args control.BundleDeployArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}

	opts, err := translateDeployOpts(args.Tests, args.TestClasses)
	if err != nil {
		return nil, err
	}
	var result bundles.OperationResult
	if args.Async {
		result, err = s.bundles.DeployAsync(context.Background(), bundles.OperationInput{
			BundleID: args.ID, Target: args.OrgAlias, Opts: opts,
		})
	} else {
		result, err = s.bundles.Deploy(context.Background(), bundles.OperationInput{
			BundleID: args.ID, Target: args.OrgAlias, Opts: opts,
		})
	}
	if err != nil {
		return nil, encodeControlServiceError(err)
	}
	return map[string]any{"bundle_id": args.ID, "sf_output": string(result.Output)}, nil
}

func (s *ControlState) BundleReport(args control.BundleReportArgs) (any, error) {
	if err := s.ensureStore(); err != nil {
		return nil, err
	}
	result, err := s.bundles.Report(context.Background(), bundles.ReportInput{
		BundleID: args.ID, Target: args.OrgAlias, JobID: args.DeployID,
	})
	if err != nil {
		return nil, encodeControlServiceError(err)
	}
	return map[string]any{
		"bundle_id": args.ID,
		"deploy_id": args.DeployID,
		"sf_output": string(result.Output),
	}, nil
}

func (s *ControlState) BundleDelete(args control.BundleDeleteArgs) error {
	if err := s.ensureStore(); err != nil {
		return err
	}
	return bundles.Delete(s.devProjects, args.ID)
}
