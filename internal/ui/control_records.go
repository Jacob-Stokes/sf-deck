package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/Jacob-Stokes/sf-deck/internal/control"
	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/records"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// IPC verb handlers for record DML (create/get/update/delete/recent) and metadata CRUD. Split out of control_backend.go.

func (s *ControlState) RecordGet(args control.RecordGetArgs) (any, error) {
	target, _, err := s.resolveTargetForIPC(args.OrgAlias, args.OrgUser)
	if err != nil {
		return nil, err
	}
	rec, err := sf.GetRecord(target, args.SObject, args.ID)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

func (s *ControlState) RecordRecent(args control.RecordRecentArgs) (any, error) {
	target, _, err := s.resolveTargetForIPC(args.OrgAlias, args.OrgUser)
	if err != nil {
		return nil, err
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 50
	}
	list, err := sf.RecentRecords(target, args.SObject, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"object":     list.SObject,
		"records":    list.Records,
		"columns":    list.Columns,
		"total_size": list.TotalSize,
		"returned":   len(list.Records),
		"done":       list.Done,
		"query":      list.Query,
	}, nil
}

func (s *ControlState) RecordCreate(args control.RecordCreateArgs) (any, error) {
	target := controlTarget(args.OrgAlias, args.OrgUser)
	result, err := s.records.Create(context.Background(), records.CreateInput{
		Target: target, SObject: args.SObject, Fields: args.Fields,
	})
	if err != nil {
		return nil, encodeControlServiceError(err)
	}
	if len(result.FieldErrors) > 0 {
		errs := make([]map[string]any, 0, len(result.FieldErrors))
		for _, fe := range result.FieldErrors {
			errs = append(errs, map[string]any{
				"error_code": fe.ErrorCode,
				"message":    fe.Message,
				"fields":     fe.Fields,
			})
		}
		return nil, fmt.Errorf("salesforce rejected create: %s (errors: %v)", result.FieldErrors[0].String(), errs)
	}
	return map[string]any{
		"object": result.SObject,
		"id":     result.ID,
	}, nil
}

func (s *ControlState) RecordDelete(args control.RecordDeleteArgs) (any, error) {
	result, err := s.records.Delete(context.Background(), records.DeleteInput{
		Target: controlTarget(args.OrgAlias, args.OrgUser), SObject: args.SObject, ID: args.ID,
	})
	if err != nil {
		return nil, encodeControlServiceError(err)
	}
	return map[string]any{
		"object": result.SObject,
		"id":     result.ID,
	}, nil
}

func (s *ControlState) RecordUpdate(args control.RecordUpdateArgs) (any, error) {
	result, err := s.records.Update(context.Background(), records.UpdateInput{
		Target: controlTarget(args.OrgAlias, args.OrgUser), SObject: args.SObject,
		ID: args.ID, Fields: args.Fields,
	})
	if err != nil {
		return nil, encodeControlServiceError(err)
	}
	return map[string]any{
		"id":           result.ID,
		"field_errors": result.FieldErrors,
	}, nil
}

func controlTarget(alias, username string) string {
	if alias != "" {
		return alias
	}
	return username
}

func (s *ControlState) MetadataGet(args control.MetadataGetArgs) (any, error) {
	if err := metadataops.ValidateType(args.Type); err != nil {
		return nil, encodeControlServiceError(err)
	}
	target, _, err := s.resolveTargetForIPC(args.OrgAlias, args.OrgUser)
	if err != nil {
		return nil, err
	}
	id := args.ID
	if id == "" && args.FullName != "" {
		q := fmt.Sprintf("SELECT Id FROM %s WHERE DeveloperName = '%s'",
			args.Type, sf.EscapeSOQLString(args.FullName))

		result, qerr := sf.Query(target, q, true)
		if qerr != nil {
			return nil, fmt.Errorf("lookup by full_name: %w", qerr)
		}
		if len(result.Records) == 0 {
			return nil, fmt.Errorf("no %s found with full_name=%q", args.Type, args.FullName)
		}
		if rid, ok := result.Records[0]["Id"].(string); ok {
			id = rid
		} else {
			return nil, errors.New("lookup returned non-string Id")
		}
	}
	md, err := sf.GetToolingMetadata(target, args.Type, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"type":     args.Type,
		"id":       id,
		"metadata": md,
	}, nil
}

func (s *ControlState) MetadataCreate(args control.MetadataCreateArgs) (any, error) {
	target := args.OrgAlias
	if target == "" {
		target = args.OrgUser
	}
	res, err := s.metadata.Create(context.Background(), metadataops.CreateInput{
		Target: target, Type: args.Type, FullName: args.FullName, Metadata: args.Patch,
	})
	if err != nil {
		return nil, encodeControlServiceError(err)
	}
	return map[string]any{
		"type":      args.Type,
		"full_name": args.FullName,
		"id":        res.ID,
	}, nil
}

func (s *ControlState) MetadataUpdate(args control.MetadataUpdateArgs) (any, error) {
	target := args.OrgAlias
	if target == "" {
		target = args.OrgUser
	}
	_, err := s.metadata.Update(context.Background(), metadataops.UpdateInput{
		Target: target, Type: args.Type, ID: args.ID, Patch: args.Patch,
	})
	if err != nil {
		return nil, encodeControlServiceError(err)
	}
	return map[string]any{
		"type":  args.Type,
		"id":    args.ID,
		"patch": args.Patch,
	}, nil
}

func (s *ControlState) MetadataDelete(args control.MetadataDeleteArgs) (any, error) {
	target := args.OrgAlias
	if target == "" {
		target = args.OrgUser
	}
	_, err := s.metadata.Delete(context.Background(), metadataops.DeleteInput{
		Target: target, Type: args.Type, ID: args.ID,
	})
	if err != nil {
		return nil, encodeControlServiceError(err)
	}
	return map[string]any{
		"type": args.Type,
		"id":   args.ID,
	}, nil
}
