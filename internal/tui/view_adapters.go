package tui

import "github.com/qyinm/gandalf/internal/tui/views"

func homeViewFromModel(model HomeViewModel, now string) views.HomeView {
	view := views.HomeView{
		HasBaseline: model.HasBaseline, HasMissingBaseline: model.HasMissingBaseline,
		LastSnapshot: now, TotalChanges: model.TotalChanges,
		SkillsChanged: model.SkillsChanged, HooksChanged: model.HooksChanged,
		MCPServersChanged: model.MCPServersChanged, PluginsChanged: model.PluginsChanged, OtherChanged: model.OtherChanged,
	}
	for _, change := range model.TopChanges {
		view.TopChanges = append(view.TopChanges, views.HomeChange{
			Agent: change.AgentLabel, Kind: change.Kind, Name: change.Name, Action: change.Action,
		})
	}
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
			RowKind:          string(row.RowKind),
			ParentID:         row.ParentID,
			Depth:            row.Depth,
			Expanded:         row.Expanded,
			Toggleable:       row.Toggleable,
			AgentLabel:       row.AgentLabel,
			AgentMarker:      row.AgentMarker,
			ObjectKind:       row.ObjectKind,
			Name:             row.Name,
			SourcePath:       row.SourcePath,
			Scope:            row.Scope,
			Status:           row.Status,
			Entrypoint:       row.Entrypoint,
			EntryStatus:      row.EntryStatus,
			RuntimeStatus:    row.RuntimeStatus,
			ToolCount:        row.ToolCount,
			Description:      row.Description,
			ActionLabel:      row.ActionLabel,
			Capability:       row.Capability,
			CapabilityReason: row.CapabilityReason,
			ToggleControl:    row.ToggleControl,
			Disabled:         row.Disabled,
			Selected:         row.Selected,
		})
		for _, tool := range row.Tools {
			view.Rows[len(view.Rows)-1].Tools = append(view.Rows[len(view.Rows)-1].Tools, views.SetupConsoleTool{
				Name:        tool.Name,
				Description: tool.Description,
			})
		}
	}
	if model.Selected != nil {
		detail := views.SetupConsoleDetail{
			Title:            model.Selected.Title,
			AgentLabel:       model.Selected.AgentLabel,
			ObjectKind:       model.Selected.ObjectKind,
			SourcePath:       model.Selected.SourcePath,
			Scope:            model.Selected.Scope,
			Status:           model.Selected.Status,
			Description:      model.Selected.Description,
			Author:           model.Selected.Author,
			Category:         model.Selected.Category,
			Version:          model.Selected.Version,
			Provides:         append([]string(nil), model.Selected.Provides...),
			ConfigTarget:     model.Selected.ConfigTarget,
			Capability:       model.Selected.Capability,
			CapabilityReason: model.Selected.CapabilityReason,
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
	if model.MarketplaceReview != nil {
		view.MarketplaceReview = &views.SetupMarketplaceReview{
			Title:          model.MarketplaceReview.Title,
			Status:         model.MarketplaceReview.Status,
			AgentLabel:     model.MarketplaceReview.AgentLabel,
			SourceLabel:    model.MarketplaceReview.SourceLabel,
			SourcePath:     model.MarketplaceReview.SourcePath,
			TargetName:     model.MarketplaceReview.TargetName,
			Operation:      model.MarketplaceReview.Operation,
			ExpectedEffect: model.MarketplaceReview.ExpectedEffect,
			Instructions:   model.MarketplaceReview.Instructions,
			Pending:        model.MarketplaceReview.Pending,
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

func environmentsViewFromModel(model EnvironmentsViewModel) views.EnvironmentsView {
	view := views.EnvironmentsView{
		FocusAgent:   model.FocusAgent,
		Focus:        string(model.Focus),
		Mode:         string(model.Mode),
		DiffOffset:   model.DiffOffset,
		ChangesEmpty: model.ChangesEmpty,
		EmptyMessage: model.EmptyMessage,
	}
	for _, row := range model.Rows {
		view.Rows = append(view.Rows, views.EnvironmentRow{
			AgentLabel:   row.AgentLabel,
			AgentMarker:  row.AgentMarker,
			State:        row.State,
			BaselineName: row.BaselineName,
			BaselineDate: row.BaselineDate,
			Detail:       row.Detail,
			Selected:     row.Selected,
		})
	}
	for _, surface := range model.Surfaces {
		view.Surfaces = append(view.Surfaces, views.EnvironmentSurface{
			ID:               surface.ID,
			Marker:           surface.Marker,
			Kind:             surface.Kind,
			Name:             surface.Name,
			Detail:           surface.Detail,
			SourcePath:       surface.SourcePath,
			ChangeCount:      surface.ChangeCount,
			Capability:       surface.Capability,
			CapabilityReason: surface.CapabilityReason,
			Selected:         surface.Selected,
		})
	}
	view.Diff = views.EnvironmentDiff{
		SurfaceID:  model.Diff.SurfaceID,
		Title:      model.Diff.Title,
		SourcePath: model.Diff.SourcePath,
	}
	for _, row := range model.Diff.Rows {
		view.Diff.Rows = append(view.Diff.Rows, views.EnvironmentDiffRow{
			ID:          row.ID,
			Kind:        string(row.Kind),
			HunkIndex:   row.HunkIndex,
			HunkTitle:   row.HunkTitle,
			CurrentHunk: row.CurrentHunk,
			Left: views.EnvironmentDiffSide{
				LineNumber: row.Left.LineNumber,
				Marker:     row.Left.Marker,
				Text:       row.Left.Text,
			},
			Right: views.EnvironmentDiffSide{
				LineNumber: row.Right.LineNumber,
				Marker:     row.Right.Marker,
				Text:       row.Right.Text,
			},
		})
	}
	return view
}
