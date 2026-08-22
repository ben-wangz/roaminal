package server

import (
	"reflect"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func cloneConnectionInstanceLayout(layout persistence.ConnectionInstanceLayout) persistence.ConnectionInstanceLayout {
	copyLayout := layout
	copyLayout.GroupOrder = cloneConnectionInstanceIDs(layout.GroupOrder)
	copyLayout.UngroupedConnectionInstanceIDs = cloneConnectionInstanceIDs(layout.UngroupedConnectionInstanceIDs)
	copyLayout.Groups = make([]persistence.ConnectionInstanceGroup, len(layout.Groups))
	for index, group := range layout.Groups {
		copyLayout.Groups[index] = group
		copyLayout.Groups[index].ConnectionInstanceIDs = cloneConnectionInstanceIDs(group.ConnectionInstanceIDs)
	}
	return copyLayout
}

func cloneConnectionInstanceIDs(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string{}, values...)
}

func (s *Server) connectionInstanceLayout(sessionID string) persistence.ConnectionInstanceLayout {
	instances := s.connectionInstanceSummaries()
	saved, exists := s.auth.ConnectionInstanceLayout(sessionID)
	legacyOrder := s.auth.ConnectionInstanceOrder(sessionID)
	normalized, changed := normalizeConnectionInstanceLayout(saved, exists, legacyOrder, instances)
	if changed {
		if exists {
			normalized.Revision = saved.Revision + 1
			if normalized.Revision == 0 {
				normalized.Revision = 1
			}
		}
		if err := s.auth.SetConnectionInstanceLayout(sessionID, normalized); err != nil {
			return normalized
		}
	}
	return normalized
}

func (s *Server) connectionInstanceSummaries() []connection.Summary {
	if s.terms == nil {
		return nil
	}
	return s.terms.Summaries()
}

func normalizeConnectionInstanceLayout(saved persistence.ConnectionInstanceLayout, exists bool, legacyOrder []string, instances []connection.Summary) (persistence.ConnectionInstanceLayout, bool) {
	available := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		available[instance.ID] = struct{}{}
	}
	if !exists {
		order, err := normalizeConnectionInstanceOrder(legacyOrder, instances)
		if err != nil {
			order = instanceIDs(instances)
		}
		return persistence.ConnectionInstanceLayout{
			Revision:                       1,
			GroupOrder:                     []string{persistence.UngroupedConnectionInstanceGroupID},
			UngroupedConnectionInstanceIDs: order,
		}, true
	}

	next := cloneConnectionInstanceLayout(saved)
	if next.Revision == 0 {
		next.Revision = 1
	}
	groupByID := make(map[string]persistence.ConnectionInstanceGroup, len(next.Groups))
	for _, group := range next.Groups {
		cleaned := group
		cleaned.ConnectionInstanceIDs = cleanInstanceIDs(group.ConnectionInstanceIDs, available, nil)
		groupByID[group.GroupID] = cleaned
	}
	orderedGroups := make([]persistence.ConnectionInstanceGroup, 0, len(next.Groups))
	groupOrder := make([]string, 0, len(next.Groups)+1)
	seenGroups := make(map[string]struct{}, len(next.Groups)+1)
	seenInstances := make(map[string]struct{}, len(instances))
	for _, groupID := range next.GroupOrder {
		if groupID == persistence.UngroupedConnectionInstanceGroupID {
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
	if _, seen := seenGroups[persistence.UngroupedConnectionInstanceGroupID]; !seen {
		groupOrder = append(groupOrder, persistence.UngroupedConnectionInstanceGroupID)
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

func instanceIDs(instances []connection.Summary) []string {
	ids := make([]string, 0, len(instances))
	for _, instance := range instances {
		ids = append(ids, instance.ID)
	}
	return ids
}

func flattenConnectionInstanceLayout(layout persistence.ConnectionInstanceLayout) []string {
	groups := make(map[string][]string, len(layout.Groups))
	for _, group := range layout.Groups {
		groups[group.GroupID] = group.ConnectionInstanceIDs
	}
	ids := make([]string, 0)
	for _, groupID := range layout.GroupOrder {
		if groupID == persistence.UngroupedConnectionInstanceGroupID {
			ids = append(ids, layout.UngroupedConnectionInstanceIDs...)
		} else {
			ids = append(ids, groups[groupID]...)
		}
	}
	return ids
}

func hasUserConnectionInstanceGroups(layout persistence.ConnectionInstanceLayout) bool {
	return len(layout.Groups) > 0
}
