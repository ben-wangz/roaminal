package server

import (
	"reflect"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func cloneConnectionInstanceLayout(layout domain.ConnectionInstanceLayout) domain.ConnectionInstanceLayout {
	copyLayout := layout
	copyLayout.GroupOrder = cloneConnectionInstanceIDs(layout.GroupOrder)
	copyLayout.UngroupedConnectionInstanceIDs = cloneConnectionInstanceIDs(layout.UngroupedConnectionInstanceIDs)
	copyLayout.Groups = make([]domain.ConnectionInstanceGroup, len(layout.Groups))
	for index, group := range layout.Groups {
		copyLayout.Groups[index] = group
		copyLayout.Groups[index].ConnectionInstanceIDs = cloneConnectionInstanceIDs(group.ConnectionInstanceIDs)
	}
	return copyLayout
}

func cloneConnectionInstanceIDs(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func (s *Server) connectionInstanceLayout(sessionID string) domain.ConnectionInstanceLayout {
	instances := s.connectionInstanceSummaries()
	saved, exists := s.workspace.ConnectionInstanceLayout(sessionID)
	legacyOrder := s.workspace.ConnectionInstanceOrder(sessionID)
	normalized, changed := normalizeConnectionInstanceLayout(saved, exists, legacyOrder, instances)
	if changed {
		expectedRevision := uint64(0)
		if exists {
			expectedRevision = saved.Revision
			normalized.Revision = saved.Revision + 1
			if normalized.Revision == 0 {
				normalized.Revision = 1
			}
		}
		if err := s.workspace.SetConnectionInstanceLayout(sessionID, normalized, expectedRevision); err != nil {
			return normalized
		}
	}
	return normalized
}

func (s *Server) connectionInstanceSummaries() []ports.ConnectionInstanceSummary {
	if s.terms == nil {
		return nil
	}
	return s.terms.Summaries()
}

func normalizeConnectionInstanceLayout(saved domain.ConnectionInstanceLayout, exists bool, legacyOrder []string, instances []ports.ConnectionInstanceSummary) (domain.ConnectionInstanceLayout, bool) {
	available := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		available[instance.ID] = struct{}{}
	}
	if !exists {
		order, err := normalizeConnectionInstanceOrder(legacyOrder, instances)
		if err != nil {
			order = instanceIDs(instances)
		}
		return domain.ConnectionInstanceLayout{
			Revision:                       1,
			GroupOrder:                     []string{domain.UngroupedConnectionInstanceGroupID},
			Groups:                         []domain.ConnectionInstanceGroup{},
			UngroupedConnectionInstanceIDs: order,
		}, true
	}

	next := cloneConnectionInstanceLayout(saved)
	if next.Revision == 0 {
		next.Revision = 1
	}
	groupByID := make(map[string]domain.ConnectionInstanceGroup, len(next.Groups))
	for _, group := range next.Groups {
		cleaned := group
		cleaned.ConnectionInstanceIDs = cleanInstanceIDs(group.ConnectionInstanceIDs, available, nil)
		groupByID[group.GroupID] = cleaned
	}
	orderedGroups := make([]domain.ConnectionInstanceGroup, 0, len(next.Groups))
	groupOrder := make([]string, 0, len(next.Groups)+1)
	seenGroups := make(map[string]struct{}, len(next.Groups)+1)
	seenInstances := make(map[string]struct{}, len(instances))
	for _, groupID := range next.GroupOrder {
		if groupID == domain.UngroupedConnectionInstanceGroupID {
			if _, seen := seenGroups[groupID]; !seen {
				groupOrder = append(groupOrder, groupID)
				seenGroups[groupID] = struct{}{}
			}
			continue
		}
		group, ok := groupByID[groupID]
		if !ok {
			continue
		}
		if _, seen := seenGroups[groupID]; seen {
			continue
		}
		group.ConnectionInstanceIDs = cleanInstanceIDs(group.ConnectionInstanceIDs, available, seenInstances)
		orderedGroups = append(orderedGroups, group)
		groupOrder = append(groupOrder, groupID)
		seenGroups[groupID] = struct{}{}
	}
	for _, group := range next.Groups {
		if _, seen := seenGroups[group.GroupID]; seen {
			continue
		}
		group := groupByID[group.GroupID]
		group.ConnectionInstanceIDs = cleanInstanceIDs(group.ConnectionInstanceIDs, available, seenInstances)
		orderedGroups = append(orderedGroups, group)
		groupOrder = append(groupOrder, group.GroupID)
		seenGroups[group.GroupID] = struct{}{}
	}
	if _, seen := seenGroups[domain.UngroupedConnectionInstanceGroupID]; !seen {
		groupOrder = append(groupOrder, domain.UngroupedConnectionInstanceGroupID)
	}

	ungrouped := cleanInstanceIDs(next.UngroupedConnectionInstanceIDs, available, seenInstances)
	for _, id := range ungrouped {
		seenInstances[id] = struct{}{}
	}
	for _, instance := range instances {
		if _, seen := seenInstances[instance.ID]; !seen {
			ungrouped = append(ungrouped, instance.ID)
			seenInstances[instance.ID] = struct{}{}
		}
	}
	next.Groups = orderedGroups
	next.GroupOrder = groupOrder
	next.UngroupedConnectionInstanceIDs = ungrouped
	return next, !reflect.DeepEqual(saved, next)
}

func cleanInstanceIDs(ids []string, available map[string]struct{}, seen map[string]struct{}) []string {
	if ids == nil {
		return nil
	}
	cleaned := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := available[id]; !ok {
			continue
		}
		if seen != nil {
			if _, already := seen[id]; already {
				continue
			}
		}
		cleaned = append(cleaned, id)
		if seen != nil {
			seen[id] = struct{}{}
		}
	}
	return cleaned
}

func instanceIDs(instances []ports.ConnectionInstanceSummary) []string {
	if instances == nil {
		return nil
	}
	ids := make([]string, 0, len(instances))
	for _, instance := range instances {
		ids = append(ids, instance.ID)
	}
	return ids
}

func flattenConnectionInstanceLayout(layout domain.ConnectionInstanceLayout) []string {
	groups := make(map[string][]string, len(layout.Groups))
	for _, group := range layout.Groups {
		groups[group.GroupID] = group.ConnectionInstanceIDs
	}
	ids := make([]string, 0)
	for _, groupID := range layout.GroupOrder {
		if groupID == domain.UngroupedConnectionInstanceGroupID {
			ids = append(ids, layout.UngroupedConnectionInstanceIDs...)
		} else {
			ids = append(ids, groups[groupID]...)
		}
	}
	return ids
}

func hasUserConnectionInstanceGroups(layout domain.ConnectionInstanceLayout) bool {
	return len(layout.Groups) > 0
}
