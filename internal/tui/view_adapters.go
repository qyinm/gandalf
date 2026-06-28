package tui

import "github.com/qyinm/gandalf/internal/tui/views"

func setupInventoryViewFromModel(model SetupInventoryViewModel) views.SetupInventoryView {
	view := views.SetupInventoryView{
		Skills:       model.Skills,
		McpServers:   model.McpServers,
		Hooks:        model.Hooks,
		Plugins:      model.Plugins,
		EmptyMessage: model.EmptyMessage,
	}
	for _, row := range model.Rows {
		view.Rows = append(view.Rows, views.SetupInventoryRow{
			AgentLabel:  row.AgentLabel,
			AgentMarker: row.AgentMarker,
			ObjectKind:  row.ObjectKind,
			Name:        row.Name,
			SourcePath:  row.SourcePath,
			ActionLabel: row.ActionLabel,
			Selected:    row.Selected,
		})
	}
	if model.Confirmation != nil {
		view.Confirmation = &views.SetupActionConfirmation{
			Action:       model.Confirmation.Action,
			AgentLabel:   model.Confirmation.AgentLabel,
			ObjectKind:   model.Confirmation.ObjectKind,
			TargetName:   model.Confirmation.TargetName,
			Operation:    model.Confirmation.Operation,
			ConfigTarget: model.Confirmation.ConfigTarget,
			Command:      model.Confirmation.Command,
		}
	}
	view.ActionError = model.ActionError
	return view
}

func setupConsoleViewFromModel(model SetupConsoleViewModel) views.SetupConsoleView {
	view := views.SetupConsoleView{
		ActiveTab:     string(model.ActiveTab),
		Search:        model.Search,
		SearchInput:   model.SearchInput,
		SearchFocused: model.SearchFocused,
		RowOffset:     model.RowOffset,
		EmptyMessage:  model.EmptyMessage,
		ActionError:   model.ActionError,
	}
	for _, tab := range model.Tabs {
		view.Tabs = append(view.Tabs, views.SetupConsoleTab{
			Label:    tab.Label,
			Count:    tab.Count,
			Selected: tab.Selected,
		})
	}
	for _, row := range model.Rows {
		view.Rows = append(view.Rows, views.SetupConsoleRow{
			RowKind:     string(row.RowKind),
			ParentID:    row.ParentID,
			Depth:       row.Depth,
			Expanded:    row.Expanded,
			Toggleable:  row.Toggleable,
			AgentMarker: row.AgentMarker,
			ObjectKind:  row.ObjectKind,
			Name:        row.Name,
			SourcePath:  row.SourcePath,
			Scope:       row.Scope,
			Status:      row.Status,
			ActionLabel: row.ActionLabel,
			Selected:    row.Selected,
		})
	}
	if model.Selected != nil {
		detail := views.SetupConsoleDetail{
			Title:        model.Selected.Title,
			AgentLabel:   model.Selected.AgentLabel,
			ObjectKind:   model.Selected.ObjectKind,
			SourcePath:   model.Selected.SourcePath,
			Scope:        model.Selected.Scope,
			Status:       model.Selected.Status,
			Description:  model.Selected.Description,
			Author:       model.Selected.Author,
			Category:     model.Selected.Category,
			Version:      model.Selected.Version,
			Provides:     append([]string(nil), model.Selected.Provides...),
			ConfigTarget: model.Selected.ConfigTarget,
		}
		for _, action := range model.Selected.Actions {
			detail.Actions = append(detail.Actions, views.SetupConsoleAction{
				Label:     action.Label,
				Available: action.Available,
				Reason:    action.Reason,
			})
		}
		view.Selected = &detail
	}
	if model.Confirmation != nil {
		view.Confirmation = &views.SetupActionConfirmation{
			Action:       model.Confirmation.Action,
			AgentLabel:   model.Confirmation.AgentLabel,
			ObjectKind:   model.Confirmation.ObjectKind,
			TargetName:   model.Confirmation.TargetName,
			Operation:    model.Confirmation.Operation,
			ConfigTarget: model.Confirmation.ConfigTarget,
			Command:      model.Confirmation.Command,
		}
	}
	return view
}

func historyViewFromModel(model TimelineViewModel) views.HistoryView {
	view := views.HistoryView{
		FilterLabel:    model.FilterLabel,
		EmptyMessage:   model.EmptyMessage,
		EmptyCommand:   model.EmptyCommand,
		CorruptWarning: model.CorruptWarning,
		CurrentSetup: views.CurrentSetup{
			ScopeLabel:    model.CurrentSetup.ScopeLabel,
			Agents:        model.CurrentSetup.Agents,
			Skills:        model.CurrentSetup.Skills,
			McpServers:    model.CurrentSetup.McpServers,
			Hooks:         model.CurrentSetup.Hooks,
			Permissions:   model.CurrentSetup.Permissions,
			EnvKeys:       model.CurrentSetup.EnvKeys,
			SkillRows:     append([]string(nil), model.CurrentSetup.SkillRows...),
			McpServerRows: append([]string(nil), model.CurrentSetup.McpServerRows...),
			HookRows:      append([]string(nil), model.CurrentSetup.HookRows...),
			EnvKeyRows:    append([]string(nil), model.CurrentSetup.EnvKeyRows...),
			Instructions:  model.CurrentSetup.Instructions,
		},
	}
	for _, row := range model.Rows {
		view.Rows = append(view.Rows, views.TimelineRow{
			ShortID:    row.ShortID,
			ObservedAt: row.ObservedAt,
			EventKind:  row.EventKind,
			Readiness:  string(row.Readiness),
			Title:      row.Title,
			Selected:   row.Selected,
		})
	}
	if model.SelectedEntry != nil {
		view.SelectedEntry = &views.TimelineDetail{
			Title:              model.SelectedEntry.Title,
			EventKind:          model.SelectedEntry.EventKind,
			Readiness:          string(model.SelectedEntry.Readiness),
			Confidence:         model.SelectedEntry.Confidence,
			BeforeSnapshotName: model.SelectedEntry.BeforeSnapshotName,
			AfterSnapshotName:  model.SelectedEntry.AfterSnapshotName,
			Counts:             model.SelectedEntry.Counts,
			Highlights:         append([]string(nil), model.SelectedEntry.Highlights...),
			WritableCount:      len(model.SelectedEntry.WritableSurfaces),
			ObserveOnlyCount:   len(model.SelectedEntry.ObserveOnlySurfaces),
		}
	}
	if model.UndoPreview != nil {
		preview := views.UndoPreview{
			Title:                model.UndoPreview.Title,
			WritesFiles:          model.UndoPreview.WritesFiles,
			ObserveOnlyCount:     len(model.UndoPreview.ObserveOnlySurfaces),
			EmptyWritableMessage: model.UndoPreview.EmptyWritableMessage,
		}
		for _, item := range model.UndoPreview.WritableItems {
			preview.WritableItems = append(preview.WritableItems, views.UndoWritableItem{
				Action:     item.Action,
				Path:       item.Path,
				ServerName: item.ServerName,
			})
		}
		view.UndoPreview = &preview
	}
	return view
}

func sidebarViewFromModel(nav NavigationModel) views.SidebarView {
	view := views.SidebarView{Cursor: nav.Cursor}
	for _, section := range nav.Sections {
		navSection := views.NavSection{Label: section.Label}
		for _, item := range section.Items {
			navSection.Items = append(navSection.Items, views.NavItem{
				ID:            item.ID,
				Label:         item.Label,
				EvidenceCount: item.EvidenceCount,
			})
			view.FlatIDs = append(view.FlatIDs, item.ID)
		}
		view.Sections = append(view.Sections, navSection)
	}
	return view
}

func agentDetailViewFromModel(model AgentDetailViewModel) views.AgentDetailView {
	view := views.AgentDetailView{
		Title:        model.Title,
		ProfileLabel: model.ProfileLabel,
		EmptyMessage: model.EmptyMessage,
	}
	view.Counts.Skills = model.Counts.Skills
	view.Counts.McpServers = model.Counts.McpServers
	view.Counts.Hooks = model.Counts.Hooks
	view.Counts.Permissions = model.Counts.Permissions
	view.Counts.EnvKeys = model.Counts.EnvKeys
	view.Counts.Instructions = model.Counts.Instructions

	appendRows := func(target *[]views.AgentInventoryRow, rows []AgentInventoryRow) {
		for _, row := range rows {
			*target = append(*target, views.AgentInventoryRow{Name: row.Name, Status: row.Status})
		}
	}
	appendRows(&view.Skills, model.Skills)
	appendRows(&view.McpServers, model.McpServers)
	appendRows(&view.Hooks, model.Hooks)
	appendRows(&view.EnvKeys, model.EnvKeys)
	appendRows(&view.Instructions, model.Instructions)

	for _, row := range model.History {
		view.History = append(view.History, views.AgentHistoryRow{
			ID:         row.ID,
			ObservedAt: row.ObservedAt,
			Title:      row.Title,
		})
	}
	return view
}

func compareViewFromModel(model CompareViewModel) views.CompareView {
	view := views.CompareView{
		FromLabel:    model.FromLabel,
		ToLabel:      model.ToLabel,
		ScopeLabel:   model.ScopeLabel,
		Summary:      append([]string(nil), model.Summary...),
		EmptyMessage: model.EmptyMessage,
	}
	for _, section := range model.Sections {
		navSection := views.CompareSection{Title: section.Title}
		for _, row := range section.Rows {
			navSection.Rows = append(navSection.Rows, views.CompareSideBySideRow{
				Marker: row.Marker,
				Before: row.Before,
				After:  row.After,
			})
		}
		view.Sections = append(view.Sections, navSection)
	}
	return view
}

func saveSetupViewFromModel(model SaveSetupViewModel) views.SaveSetupView {
	view := views.SaveSetupView{
		Title:           model.Title,
		DetectedChanges: append([]string(nil), model.DetectedChanges...),
		NoChanges:       model.NoChanges,
	}
	for _, dest := range model.Destinations {
		view.Destinations = append(view.Destinations, views.SaveSetupDestination{
			Label:    dest.Label,
			Selected: dest.Selected,
		})
	}
	return view
}
