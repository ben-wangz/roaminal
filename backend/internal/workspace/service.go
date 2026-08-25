package workspace

import (
	"context"
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

var ErrRepositoryUnavailable = errors.New("workspace repository unavailable")

// Service owns the user workspace aggregate. Authentication only establishes
// the authentication-session identity used as this service's partition key.
type Service struct {
	repository ports.WorkspaceLayoutRepository
}

func New(repository ports.WorkspaceLayoutRepository) *Service {
	return &Service{repository: repository}
}

func (s *Service) ConnectionInstanceLayout(sessionID string) (domain.ConnectionInstanceLayout, bool) {
	if s == nil || s.repository == nil {
		return domain.ConnectionInstanceLayout{}, false
	}
	layout, found, err := s.repository.LoadWorkspaceLayout(context.Background(), domain.AuthenticationSessionID(sessionID))
	if err != nil || !found {
		return domain.ConnectionInstanceLayout{}, false
	}
	return cloneLayout(layout), true
}

func (s *Service) SetConnectionInstanceLayout(sessionID string, layout domain.ConnectionInstanceLayout, expectedRevision uint64) error {
	if s == nil || s.repository == nil {
		return ErrRepositoryUnavailable
	}
	if err := domain.ValidateConnectionInstanceLayout(&layout); err != nil {
		return err
	}
	return s.repository.SaveWorkspaceLayout(context.Background(), domain.AuthenticationSessionID(sessionID), cloneLayout(layout), expectedRevision)
}

func (s *Service) ConnectionInstanceOrder(sessionID string) []string {
	layout, ok := s.ConnectionInstanceLayout(sessionID)
	if !ok {
		return nil
	}
	return flattenLayout(layout)
}

func (s *Service) SetConnectionInstanceOrder(sessionID string, order []string) error {
	if s == nil || s.repository == nil {
		return ErrRepositoryUnavailable
	}
	if err := domain.ValidateConnectionInstanceOrder(order); err != nil {
		return err
	}
	current, found, err := s.repository.LoadWorkspaceLayout(context.Background(), domain.AuthenticationSessionID(sessionID))
	if err != nil {
		return err
	}
	revision := uint64(1)
	if found {
		revision = current.Revision + 1
	}
	return s.repository.SaveWorkspaceLayout(context.Background(), domain.AuthenticationSessionID(sessionID), domain.ConnectionInstanceLayout{
		Revision:                       revision,
		GroupOrder:                     []string{domain.UngroupedConnectionInstanceGroupID},
		UngroupedConnectionInstanceIDs: append([]string(nil), order...),
	}, revision-1)
}

func cloneLayout(layout domain.ConnectionInstanceLayout) domain.ConnectionInstanceLayout {
	copyLayout := layout
	copyLayout.GroupOrder = append([]string(nil), layout.GroupOrder...)
	copyLayout.UngroupedConnectionInstanceIDs = append([]string(nil), layout.UngroupedConnectionInstanceIDs...)
	copyLayout.Groups = make([]domain.ConnectionInstanceGroup, len(layout.Groups))
	for index, group := range layout.Groups {
		copyLayout.Groups[index] = group
		copyLayout.Groups[index].ConnectionInstanceIDs = append([]string(nil), group.ConnectionInstanceIDs...)
	}
	return copyLayout
}

func flattenLayout(layout domain.ConnectionInstanceLayout) []string {
	groups := make(map[string][]string, len(layout.Groups))
	for _, group := range layout.Groups {
		groups[group.GroupID] = group.ConnectionInstanceIDs
	}
	result := make([]string, 0, len(layout.UngroupedConnectionInstanceIDs))
	for _, groupID := range layout.GroupOrder {
		if groupID == domain.UngroupedConnectionInstanceGroupID {
			result = append(result, layout.UngroupedConnectionInstanceIDs...)
			continue
		}
		result = append(result, groups[groupID]...)
	}
	return result
}
