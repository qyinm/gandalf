package tui

import (
	"github.com/qyinm/gandalf/internal/gandalfcore/baseline"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

type environmentState struct {
	initialized   bool
	agentCursor   int
	surfaceCursor int
	focus         EnvironmentFocus
	mode          EnvironmentRenderMode
	hunkCursor    int
	diffOffset    int
}

func (s *environmentState) ensure() {
	if s.initialized {
		return
	}
	s.initialized = true
	s.focus = EnvironmentFocusAgents
	s.mode = EnvironmentRenderModeSideBySide
}

func (s *environmentState) clampAgents(status baseline.Status) {
	s.ensure()
	s.agentCursor = clampIndex(s.agentCursor, len(status.Agents))
}

func (s *environmentState) clampSurfaces(count int) {
	s.ensure()
	s.surfaceCursor = clampIndex(s.surfaceCursor, count)
	if count == 0 {
		s.hunkCursor = 0
		s.diffOffset = 0
	}
}

func (s *environmentState) clampHunks(count int) {
	s.ensure()
	s.hunkCursor = clampIndex(s.hunkCursor, count)
}

func (s *environmentState) selectedAgent(status baseline.Status) (types.AgentID, bool) {
	s.clampAgents(status)
	if len(status.Agents) == 0 {
		return "", false
	}
	return status.Agents[s.agentCursor].Agent, true
}

func (s *environmentState) moveAgent(status baseline.Status, delta int) {
	s.ensure()
	if len(status.Agents) == 0 {
		s.agentCursor = 0
		return
	}
	next := s.agentCursor + delta
	if next < 0 {
		next = len(status.Agents) - 1
	}
	if next >= len(status.Agents) {
		next = 0
	}
	if next != s.agentCursor {
		s.agentCursor = next
		s.surfaceCursor = 0
		s.hunkCursor = 0
		s.diffOffset = 0
	}
}

func (s *environmentState) moveSurface(count, delta int) {
	s.ensure()
	if count == 0 {
		s.surfaceCursor = 0
		return
	}
	next := s.surfaceCursor + delta
	if next < 0 {
		next = count - 1
	}
	if next >= count {
		next = 0
	}
	if next != s.surfaceCursor {
		s.surfaceCursor = next
		s.hunkCursor = 0
		s.diffOffset = 0
	}
}

func (s *environmentState) cycleFocus() {
	s.ensure()
	switch s.focus {
	case EnvironmentFocusAgents:
		s.focus = EnvironmentFocusSurfaces
	case EnvironmentFocusSurfaces:
		s.focus = EnvironmentFocusDiff
	default:
		s.focus = EnvironmentFocusAgents
	}
}

func (s *environmentState) toggleMode() {
	s.ensure()
	if s.mode == EnvironmentRenderModeUnified {
		s.mode = EnvironmentRenderModeSideBySide
	} else {
		s.mode = EnvironmentRenderModeUnified
	}
	s.diffOffset = 0
}

func (s *environmentState) scrollDiff(model EnvironmentsViewModel, height, delta int) {
	s.ensure()
	s.diffOffset += delta
	s.clampDiffOffset(model, height)
}

func (s *environmentState) pageDiff(model EnvironmentsViewModel, height, delta int) {
	s.ensure()
	if delta > 0 {
		s.diffOffset += max(1, height)
	} else if delta < 0 {
		s.diffOffset -= max(1, height)
	}
	s.clampDiffOffset(model, height)
}

func (s *environmentState) moveHunk(model EnvironmentsViewModel, delta int, height int) {
	s.ensure()
	count := environmentHunkCount(model.Diff.Rows)
	if count == 0 {
		s.hunkCursor = 0
		return
	}
	next := s.hunkCursor + delta
	if next < 0 {
		next = count - 1
	}
	if next >= count {
		next = 0
	}
	s.hunkCursor = next
	s.focus = EnvironmentFocusDiff
	s.diffOffset = environmentHunkOffset(model, next)
	s.clampDiffOffset(model, height)
}

func (s *environmentState) clampDiffOffset(model EnvironmentsViewModel, height int) {
	if s.diffOffset < 0 {
		s.diffOffset = 0
	}
	maxOffset := max(0, environmentRenderedLineCount(model)-max(1, height))
	if s.diffOffset > maxOffset {
		s.diffOffset = maxOffset
	}
}

func environmentHunkCount(rows []EnvironmentDiffRowModel) int {
	count := 0
	for _, row := range rows {
		if row.Kind == EnvironmentDiffRowHunk {
			count++
		}
	}
	return count
}

func environmentHunkOffset(model EnvironmentsViewModel, hunkIndex int) int {
	offset := 1
	for _, row := range model.Diff.Rows {
		if row.Kind == EnvironmentDiffRowHunk && row.HunkIndex == hunkIndex {
			return offset
		}
		offset++
		if model.Mode == EnvironmentRenderModeUnified && row.Kind == EnvironmentDiffRowChanged {
			offset++
		}
	}
	return 0
}

func environmentRenderedLineCount(model EnvironmentsViewModel) int {
	count := 1
	for _, row := range model.Diff.Rows {
		count++
		if model.Mode == EnvironmentRenderModeUnified && row.Kind == EnvironmentDiffRowChanged {
			count++
		}
	}
	return count
}
