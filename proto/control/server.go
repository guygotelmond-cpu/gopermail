package control

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"gomail.com/db"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RuleGrpcServer struct {
	UnimplementedRuleServiceServer
	RStore *db.PostgresService
}

func (s *RuleGrpcServer) AddRule(ctx context.Context, req *AddRuleRequest) (*AddRuleResponse, error) {
	// 1. Boundary Guard against empty requests
	if req.GetRule() == nil || req.GetUsername() == "" {
		return nil, status.Error(codes.InvalidArgument, "username and rule parameters are strictly required")
	}

	// 2. Map Protobuf typed message to internal Domain DB Struct
	domainRule := db.Rule{
		Field:       req.Rule.Field,
		Operator:    req.Rule.Operator,
		Value:       req.Rule.Value,
		Action:      req.Rule.Action,
		ActionValue: req.Rule.ActionValue,
	}

	// 3. Persist rule via the unified PostgreSQL data layer
	err := s.RStore.AddRule(req.GetUsername(), domainRule)
	if err != nil {
		// Catch structural schema validation flaws or missing user keys
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "exist") {
			return nil, status.Errorf(codes.NotFound, "target user configuration invalid: %v", err)
		}
		return nil, status.Errorf(codes.InvalidArgument, "rule validation constraint rejected: %v", err)
	}

	return &AddRuleResponse{
		Success: true,
		Message: "Pristine rule compilation mapped and injected dynamically into engine storage layer.",
	}, nil
}
