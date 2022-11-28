// wm.go
// Copyright(c) 2022 Matt Pharr, licensed under the GNU Public License, Version 3.
// SPDX: GPL-3.0-only

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/mmp/imgui-go/v4"
)

var (
	wm struct {
		showConfigEditor   bool
		paneFirstPick      Pane
		handlePanePick     func(Pane) bool
		paneCreatePrompt   string
		paneConfigHelpText string
		editorBackupRoot   *DisplayNode

		configButtons ModalButtonSet

		showPaneSettings map[Pane]*bool
		showPaneName     map[Pane]string

		showPaneAsRoot  bool
		nodeFilter      func(*DisplayNode) *DisplayNode
		nodeFilterUnset bool

		topControlsHeight float32

		mouseConsumerOverride Pane
		statusBarHasFocus     bool // overrides keyboardFocusPane
		keyboardFocusPane     Pane
		keyboardFocusStack    []Pane
	}
)

///////////////////////////////////////////////////////////////////////////
// SplitLine

type SplitType int

const (
	SplitAxisNone = iota
	SplitAxisX
	SplitAxisY
)

type SplitLine struct {
	Pos  float32
	Axis SplitType
}

func (s *SplitLine) Duplicate(nameAsCopy bool) Pane {
	lg.Errorf("This actually should never be called...")
	return &SplitLine{}
}

func (s *SplitLine) Activate(cs *ColorScheme)   {}
func (s *SplitLine) Deactivate()                {}
func (s *SplitLine) CanTakeKeyboardFocus() bool { return false }

func (s *SplitLine) Name() string {
	return "Split Line"
}

func (s *SplitLine) Draw(ctx *PaneContext, cb *CommandBuffer) {
	if ctx.mouse != nil && ctx.mouse.dragging[mouseButtonSecondary] {
		delta := ctx.mouse.dragDelta

		if s.Axis == SplitAxisX {
			s.Pos += delta[0] / ctx.parentPaneExtent.Width()
		} else {
			s.Pos += delta[1] / ctx.parentPaneExtent.Height()
		}
		// Just in case
		s.Pos = clamp(s.Pos, .01, .99)
	}

	cb.ClearRGB(ctx.cs.UIControl)
}

func splitLineWidth() int {
	return int(3*dpiScale(platform) + 0.5)
}

///////////////////////////////////////////////////////////////////////////
// DisplayNode

type DisplayNode struct {
	Pane      Pane // set iff splitAxis == SplitAxisNone
	SplitLine SplitLine
	Children  [2]*DisplayNode // set iff splitAxis != SplitAxisNone
}

func (d *DisplayNode) Duplicate() *DisplayNode {
	dupe := &DisplayNode{}

	if d.Pane != nil {
		dupe.Pane = d.Pane.Duplicate(false)
	}
	dupe.SplitLine = d.SplitLine

	if d.SplitLine.Axis != SplitAxisNone {
		dupe.Children[0] = d.Children[0].Duplicate()
		dupe.Children[1] = d.Children[1].Duplicate()
	}
	return dupe
}

func (d *DisplayNode) NodeForPane(pane Pane) *DisplayNode {
	if d.Pane == pane {
		return d
	}
	if d.Children[0] == nil {
		return nil
	}
	d0 := d.Children[0].NodeForPane(pane)
	if d0 != nil {
		return d0
	}
	return d.Children[1].NodeForPane(pane)
}

func (d *DisplayNode) ParentNodeForPane(pane Pane) (*DisplayNode, int) {
	if d == nil {
		return nil, -1
	}

	if d.Children[0] != nil && d.Children[0].Pane == pane {
		return d, 0
	} else if d.Children[1] != nil && d.Children[1].Pane == pane {
		return d, 1
	}

	if c0, idx := d.Children[0].ParentNodeForPane(pane); c0 != nil {
		return c0, idx
	}
	return d.Children[1].ParentNodeForPane(pane)
}

type TypedDisplayNodePane struct {
	DisplayNode
	Type string
}

func (d *DisplayNode) MarshalJSON() ([]byte, error) {
	td := TypedDisplayNodePane{DisplayNode: *d}
	if d.Pane != nil {
		td.Type = fmt.Sprintf("%T", d.Pane)
	}
	return json.Marshal(td)
}

func (d *DisplayNode) UnmarshalJSON(s []byte) error {
	var m map[string]*json.RawMessage
	if err := json.Unmarshal(s, &m); err != nil {
		return err
	}

	var paneType string
	if err := json.Unmarshal(*m["Type"], &paneType); err != nil {
		return err
	}
	if err := json.Unmarshal(*m["SplitLine"], &d.SplitLine); err != nil {
		return err
	}
	if err := json.Unmarshal(*m["Children"], &d.Children); err != nil {
		return err
	}

	switch paneType {
	case "":
		// nil pane

	case "*main.AirportInfoPane":
		var aip AirportInfoPane
		if err := json.Unmarshal(*m["Pane"], &aip); err != nil {
			return err
		}
		d.Pane = &aip

	case "*main.CLIPane":
		var clip CLIPane
		if err := json.Unmarshal(*m["Pane"], &clip); err != nil {
			return err
		}
		d.Pane = &clip

	case "*main.EmptyPane":
		var ep EmptyPane
		if err := json.Unmarshal(*m["Pane"], &ep); err != nil {
			return err
		}
		d.Pane = &ep

	case "*main.FlightPlanPane":
		var fp FlightPlanPane
		if err := json.Unmarshal(*m["Pane"], &fp); err != nil {
			return err
		}
		d.Pane = &fp

	case "*main.FlightStripPane":
		var fs FlightStripPane
		if err := json.Unmarshal(*m["Pane"], &fs); err != nil {
			return err
		}
		d.Pane = &fs

	case "*main.NotesViewPane":
		var nv NotesViewPane
		if err := json.Unmarshal(*m["Pane"], &nv); err != nil {
			return err
		}
		d.Pane = &nv

	case "*main.PerformancePane":
		var pp PerformancePane
		if err := json.Unmarshal(*m["Pane"], &pp); err != nil {
			return err
		}
		d.Pane = &pp

	case "*main.RadarScopePane":
		var rsp RadarScopePane
		if err := json.Unmarshal(*m["Pane"], &rsp); err != nil {
			return err
		}
		d.Pane = &rsp

	case "*main.ReminderPane":
		var rp ReminderPane
		if err := json.Unmarshal(*m["Pane"], &rp); err != nil {
			return err
		}
		d.Pane = &rp

	default:
		lg.Errorf("%s: Unhandled type in config file", paneType)
		d.Pane = NewEmptyPane() // don't crash at least
	}

	return nil
}

func (d *DisplayNode) VisitPanes(visit func(Pane)) {
	switch d.SplitLine.Axis {
	case SplitAxisNone:
		visit(d.Pane)
	default:
		d.Children[0].VisitPanes(visit)
		visit(&d.SplitLine)
		d.Children[1].VisitPanes(visit)
	}
}

func (d *DisplayNode) VisitPanesWithBounds(nodeFilter func(*DisplayNode) *DisplayNode,
	framebufferExtent Extent2D, displayExtent Extent2D,
	parentDisplayExtent Extent2D, fullDisplayExtent Extent2D,
	visit func(Extent2D, Extent2D, Extent2D, Extent2D, Pane)) {
	d = nodeFilter(d)

	switch d.SplitLine.Axis {
	case SplitAxisNone:
		visit(framebufferExtent, displayExtent, parentDisplayExtent, fullDisplayExtent, d.Pane)
	case SplitAxisX:
		f0, fs, f1 := framebufferExtent.SplitX(d.SplitLine.Pos, splitLineWidth())
		d0, ds, d1 := displayExtent.SplitX(d.SplitLine.Pos, splitLineWidth())
		d.Children[0].VisitPanesWithBounds(nodeFilter, f0, d0, displayExtent, fullDisplayExtent, visit)
		visit(fs, ds, displayExtent, fullDisplayExtent, &d.SplitLine)
		d.Children[1].VisitPanesWithBounds(nodeFilter, f1, d1, displayExtent, fullDisplayExtent, visit)
	case SplitAxisY:
		f0, fs, f1 := framebufferExtent.SplitY(d.SplitLine.Pos, splitLineWidth())
		d0, ds, d1 := displayExtent.SplitY(d.SplitLine.Pos, splitLineWidth())
		d.Children[0].VisitPanesWithBounds(nodeFilter, f0, d0, displayExtent, fullDisplayExtent, visit)
		visit(fs, ds, displayExtent, fullDisplayExtent, &d.SplitLine)
		d.Children[1].VisitPanesWithBounds(nodeFilter, f1, d1, displayExtent, fullDisplayExtent, visit)
	}
}

func (d *DisplayNode) SplitX(x float32, newChild *DisplayNode) *DisplayNode {
	if d.SplitLine.Axis != SplitAxisNone {
		lg.Errorf("splitting a non-leaf node: %v", d)
	}
	return &DisplayNode{SplitLine: SplitLine{Axis: SplitAxisX, Pos: x},
		Children: [2]*DisplayNode{d, newChild}}
}

func (d *DisplayNode) SplitY(y float32, newChild *DisplayNode) *DisplayNode {
	if d.SplitLine.Axis != SplitAxisNone {
		lg.Errorf("splitting a non-leaf node: %v", d)
	}
	return &DisplayNode{SplitLine: SplitLine{Axis: SplitAxisX, Pos: y},
		Children: [2]*DisplayNode{d, newChild}}
}

func findPaneForMouse(node *DisplayNode, displayExtent Extent2D, p [2]float32) Pane {
	if !displayExtent.Inside(p) {
		return nil
	}
	if node.SplitLine.Axis == SplitAxisNone {
		return node.Pane
	}
	var d0, ds, d1 Extent2D
	if node.SplitLine.Axis == SplitAxisX {
		d0, ds, d1 = displayExtent.SplitX(node.SplitLine.Pos, splitLineWidth())
	} else {
		d0, ds, d1 = displayExtent.SplitY(node.SplitLine.Pos, splitLineWidth())
	}
	if d0.Inside(p) {
		return findPaneForMouse(node.Children[0], d0, p)
	} else if ds.Inside(p) {
		return &node.SplitLine
	} else if d1.Inside(p) {
		return findPaneForMouse(node.Children[1], d1, p)
	} else {
		lg.Errorf("Mouse not overlapping anything?")
		return nil
	}
}

func wmInit() {
	lg.Printf("Starting wm initialization")
	wm.nodeFilter = func(node *DisplayNode) *DisplayNode { return node }
	wm.nodeFilterUnset = true

	var pthelper func(indent string, node *DisplayNode) string
	pthelper = func(indent string, node *DisplayNode) string {
		if node == nil {
			return ""
		}
		s := fmt.Sprintf(indent+"%p split %d pane %p (%T)\n", node, node.SplitLine.Axis, node.Pane, node.Pane)
		s += pthelper(indent+"     ", node.Children[0])
		s += pthelper(indent+"     ", node.Children[1])
		return s
	}
	printtree := func() string {
		return pthelper("", positionConfig.DisplayRoot)
	}

	wm.configButtons.Add("Copy", func() func(pane Pane) bool {
		wm.paneConfigHelpText = "Select window to copy"
		return func(pane Pane) bool {
			if wm.paneFirstPick == nil {
				wm.paneFirstPick = pane
				wm.paneConfigHelpText = "Select destination for copy"
				return false
			} else {
				node := positionConfig.DisplayRoot.NodeForPane(pane)
				lg.Printf("about to copy %p %+T to node %v.\ntree: %s", pane, pane, node, printtree())
				node.Pane = wm.paneFirstPick.Duplicate(true)
				wm.paneFirstPick = nil
				wm.paneConfigHelpText = ""
				lg.Printf("new tree:\n%s", printtree())
				return true
			}
		}
	}, func() bool { return positionConfig.DisplayRoot.Children[0] != nil })

	wm.configButtons.Add("Exchange",
		func() func(pane Pane) bool {
			wm.paneConfigHelpText = "Select first window for exchange"

			return func(pane Pane) bool {
				if wm.paneFirstPick == nil {
					wm.paneFirstPick = pane
					wm.paneConfigHelpText = "Select second window for exchange"
					return false
				} else {
					n0 := positionConfig.DisplayRoot.NodeForPane(wm.paneFirstPick)
					n1 := positionConfig.DisplayRoot.NodeForPane(pane)
					lg.Printf("about echange nodes %p %+v %p %+v.\ntree: %s", n0, n0, n1, n1, printtree())
					if pane != wm.paneFirstPick {
						n0.Pane, n1.Pane = n1.Pane, n0.Pane
					}
					wm.paneFirstPick = nil
					wm.paneConfigHelpText = ""
					lg.Printf("new tree:\n%s", printtree())
					return true
				}
			}
		}, func() bool { return positionConfig.DisplayRoot.Children[0] != nil })

	handleSplitPick := func(axis SplitType) func() func(pane Pane) bool {
		return func() func(pane Pane) bool {
			wm.paneConfigHelpText = "Select window to split"
			return func(pane Pane) bool {
				lg.Printf("about to split %p %+T.\ntree: %s", pane, pane, printtree())
				node := positionConfig.DisplayRoot.NodeForPane(pane)
				node.Children[0] = &DisplayNode{Pane: &EmptyPane{}}
				node.Children[1] = &DisplayNode{Pane: pane}
				node.Pane = nil
				node.SplitLine.Pos = 0.5
				node.SplitLine.Axis = axis
				wm.paneConfigHelpText = ""
				lg.Printf("new tree:\n%s", printtree())
				return true
			}
		}
	}
	wm.configButtons.Add("Split Horizontally", handleSplitPick(SplitAxisX),
		func() bool { return true })
	wm.configButtons.Add("Split Vertically", handleSplitPick(SplitAxisY),
		func() bool { return true })
	wm.configButtons.Add("Delete", func() func(pane Pane) bool {
		wm.paneConfigHelpText = "Select window to delete"
		return func(pane Pane) bool {
			lg.Printf("about to delete %p %+T.\ntree: %s", pane, pane, printtree())
			node, idx := positionConfig.DisplayRoot.ParentNodeForPane(pane)
			other := idx ^ 1
			*node = *node.Children[other]
			wm.paneConfigHelpText = ""
			lg.Printf("new tree:\n%s", printtree())
			return true
		}
	}, func() bool { return positionConfig.DisplayRoot.Children[0] != nil })

	lg.Printf("Finished wm initialization")
}

func wmAddPaneMenuSettings() {
	var panes []Pane
	positionConfig.DisplayRoot.VisitPanes(func(pane Pane) {
		if _, ok := pane.(PaneUIDrawer); ok {
			panes = append(panes, pane)
		}
	})

	// sort by name
	sort.Slice(panes, func(i, j int) bool { return panes[i].Name() < panes[j].Name() })

	for _, pane := range panes {
		if imgui.MenuItem(pane.Name() + "...") {
			// copy the name so that it can be edited...
			wm.showPaneName[pane] = pane.Name()
			t := true
			wm.showPaneSettings[pane] = &t
		}
	}
}

func wmDrawUI(p Platform) {
	wm.topControlsHeight = ui.topControlsHeight
	if wm.showConfigEditor {
		var flags imgui.WindowFlags
		flags = imgui.WindowFlagsNoDecoration
		flags |= imgui.WindowFlagsNoSavedSettings
		flags |= imgui.WindowFlagsNoNav
		flags |= imgui.WindowFlagsNoResize

		displaySize := p.DisplaySize()
		imgui.SetNextWindowPosV(imgui.Vec2{X: 0, Y: ui.topControlsHeight}, imgui.ConditionAlways, imgui.Vec2{})
		imgui.SetNextWindowSize(imgui.Vec2{displaySize[0], 60}) //displaySize[1]})
		wm.topControlsHeight += 60
		imgui.BeginV("Config editor", nil, flags)

		cs := positionConfig.GetColorScheme()
		imgui.PushStyleColor(imgui.StyleColorText, cs.Text.imgui())

		setPicked := func(newPane Pane) func(pane Pane) bool {
			return func(pane Pane) bool {
				node := positionConfig.DisplayRoot.NodeForPane(pane)
				node.Pane = newPane
				wm.paneCreatePrompt = ""
				wm.paneConfigHelpText = ""
				return true
			}
		}
		imgui.SetNextItemWidth(imgui.WindowWidth() * .2)
		prompt := wm.paneCreatePrompt
		if prompt == "" {
			prompt = "Create New..."
		}
		if imgui.BeginCombo("##Set...", prompt) {
			if imgui.Selectable("Airport information") {
				wm.paneCreatePrompt = "Airport information"
				wm.paneConfigHelpText = "Select location for new " + wm.paneCreatePrompt + " window"
				wm.handlePanePick = setPicked(NewAirportInfoPane())
			}
			if imgui.Selectable("Command-line interface") {
				wm.paneCreatePrompt = "Command-line interface"
				wm.paneConfigHelpText = "Select location for new " + wm.paneCreatePrompt + " window"
				wm.handlePanePick = setPicked(NewCLIPane())
			}
			if imgui.Selectable("Empty") {
				wm.paneCreatePrompt = "Empty"
				wm.paneConfigHelpText = "Select location for new " + wm.paneCreatePrompt + " window"
				wm.handlePanePick = setPicked(NewEmptyPane())
			}
			if imgui.Selectable("Flight plan") {
				wm.paneCreatePrompt = "Flight plan"
				wm.paneConfigHelpText = "Select location for new " + wm.paneCreatePrompt + " window"
				wm.handlePanePick = setPicked(NewFlightPlanPane())
			}
			if imgui.Selectable("Flight strip") {
				wm.paneCreatePrompt = "Flight strip"
				wm.paneConfigHelpText = "Select location for new " + wm.paneCreatePrompt + " window"
				wm.handlePanePick = setPicked(NewFlightStripPane())
			}
			if imgui.Selectable("Notes Viewer") {
				wm.paneCreatePrompt = "Notes viewer"
				wm.paneConfigHelpText = "Select location for new " + wm.paneCreatePrompt + " window"
				wm.handlePanePick = setPicked(NewNotesViewPane())
			}
			if imgui.Selectable("Performance statistics") {
				wm.paneCreatePrompt = "Performance statistics"
				wm.paneConfigHelpText = "Select location for new " + wm.paneCreatePrompt + " window"
				wm.handlePanePick = setPicked(NewPerformancePane())
			}
			if imgui.Selectable("Radar Scope") {
				wm.paneCreatePrompt = "Radar scope"
				wm.paneConfigHelpText = "Select location for new " + wm.paneCreatePrompt + " window"
				wm.handlePanePick = setPicked(NewRadarScopePane("(Unnamed)"))
			}
			if imgui.Selectable("Reminders") {
				wm.paneCreatePrompt = "Reminders"
				wm.paneConfigHelpText = "Select location for new " + wm.paneCreatePrompt + " window"
				wm.handlePanePick = setPicked(NewReminderPane())
			}
			imgui.EndCombo()
		}

		imgui.SameLine()

		wm.configButtons.Draw()

		if wm.handlePanePick != nil {
			imgui.SameLine()
			if imgui.Button("Cancel") {
				wm.handlePanePick = nil
				wm.paneFirstPick = nil
				wm.paneConfigHelpText = ""
				wm.configButtons.Clear()
			}
		}

		imgui.SameLine()
		imgui.SetCursorPos(imgui.Vec2{platform.DisplaySize()[0] - float32(110), imgui.CursorPosY()})
		if imgui.Button("Save") {
			wm.showConfigEditor = false
			wm.paneConfigHelpText = ""
			wm.editorBackupRoot = nil
		}
		imgui.SameLine()
		if imgui.Button("Revert") {
			positionConfig.DisplayRoot = wm.editorBackupRoot
			wm.showConfigEditor = false
			wm.paneConfigHelpText = ""
			wm.editorBackupRoot = nil
		}

		imgui.Text(wm.paneConfigHelpText)

		imgui.PopStyleColor()
		imgui.End()
	}

	positionConfig.DisplayRoot.VisitPanes(func(pane Pane) {
		if show, ok := wm.showPaneSettings[pane]; ok && *show {
			if uid, ok := pane.(PaneUIDrawer); ok {
				imgui.BeginV(wm.showPaneName[pane]+" settings", show, imgui.WindowFlagsAlwaysAutoResize)
				uid.DrawUI()
				imgui.End()
			}
		}
	})
}

func wmTakeKeyboardFocus(pane Pane, isTransient bool) {
	if wm.keyboardFocusPane == pane {
		return
	}
	if isTransient && wm.keyboardFocusPane != nil {
		wm.keyboardFocusStack = append(wm.keyboardFocusStack, wm.keyboardFocusPane)
	}
	if !isTransient {
		wm.keyboardFocusStack = nil
	}
	wm.keyboardFocusPane = pane
}

func wmReleaseKeyboardFocus() {
	if n := len(wm.keyboardFocusStack); n > 0 {
		wm.keyboardFocusPane = wm.keyboardFocusStack[n-1]
		wm.keyboardFocusStack = wm.keyboardFocusStack[:n-1]
	}
}

func wmPaneIsPresent(pane Pane) bool {
	found := false
	positionConfig.DisplayRoot.VisitPanes(func(p Pane) {
		if p == pane {
			found = true
		}
	})
	return found
}

func wmDrawPanes(platform Platform, renderer Renderer) {
	if !wmPaneIsPresent(wm.keyboardFocusPane) {
		// It was deleted in the config editor or a new config was loaded.
		wm.keyboardFocusPane = nil
	}
	if wm.keyboardFocusPane == nil {
		// Pick one that can take it. Try to find a CLI pane first since that's
		// most likely where the user would prefer to start out...
		positionConfig.DisplayRoot.VisitPanes(func(p Pane) {
			if _, ok := p.(*CLIPane); ok {
				wm.keyboardFocusPane = p
			}
		})
		// If there's no CLIPane then go ahead and take any one that can
		// take keyboard events.
		if wm.keyboardFocusPane == nil {
			positionConfig.DisplayRoot.VisitPanes(func(p Pane) {
				if p.CanTakeKeyboardFocus() {
					wm.keyboardFocusPane = p
				}
			})
		}
	}

	io := imgui.CurrentIO()

	fbSize := platform.FramebufferSize()
	displaySize := platform.DisplaySize()
	heightRatio := fbSize[1] / displaySize[1]

	statusBarHeight := globalConfig.statusBar.Height()
	topControlsHeight := statusBarHeight + wm.topControlsHeight

	fbFull := Extent2D{p0: [2]float32{0, 0},
		p1: [2]float32{fbSize[0], fbSize[1] - heightRatio*topControlsHeight}}
	displayFull := Extent2D{p0: [2]float32{0, 0},
		p1: [2]float32{displaySize[0], displaySize[1] - topControlsHeight}}
	displayTrueFull := Extent2D{p0: [2]float32{0, 0},
		p1: [2]float32{displaySize[0], displaySize[1]}}

	if !io.WantCaptureKeyboard() && platform.IsControlFPressed() {
		wm.showPaneAsRoot = !wm.showPaneAsRoot
	}

	mousePos := imgui.MousePos()
	// Yaay, y flips
	mousePos.Y = displaySize[1] - 1 - mousePos.Y

	var mousePane Pane
	if wm.showPaneAsRoot && wm.nodeFilterUnset {
		pane := findPaneForMouse(positionConfig.DisplayRoot, displayFull,
			[2]float32{mousePos.X, mousePos.Y})
		// Don't maximize empty panes or split lines
		if _, ok := pane.(*SplitLine); !ok && pane != nil {
			wm.nodeFilter = func(node *DisplayNode) *DisplayNode {
				return &DisplayNode{Pane: pane}
			}
			mousePane = pane
			wm.nodeFilterUnset = false
		}
	}
	if !wm.showPaneAsRoot {
		if !wm.nodeFilterUnset {
			wm.nodeFilter = func(node *DisplayNode) *DisplayNode { return node }
			wm.nodeFilterUnset = true
		}
		mousePane = findPaneForMouse(positionConfig.DisplayRoot, displayFull,
			[2]float32{mousePos.X, mousePos.Y})
	}

	if wm.handlePanePick != nil && imgui.IsMouseClicked(mouseButtonPrimary) && mousePane != nil {
		// Filter out splits
		if _, split := mousePane.(*SplitLine); !split {
			if wm.handlePanePick(mousePane) {
				wm.handlePanePick = nil
			}
		}
	}

	// Clear the mouse override if imgui wants mouse events or if there
	// is no longer any click or drag action.
	isDragging := imgui.IsMouseDragging(mouseButtonPrimary, 0.) ||
		imgui.IsMouseDragging(mouseButtonSecondary, 0.) ||
		imgui.IsMouseDragging(mouseButtonTertiary, 0.)
	isClicked := imgui.IsMouseClicked(mouseButtonPrimary) ||
		imgui.IsMouseClicked(mouseButtonSecondary) ||
		imgui.IsMouseClicked(mouseButtonTertiary)
	if io.WantCaptureMouse() || (!isDragging && !isClicked) {
		wm.mouseConsumerOverride = nil
	}
	// Set the mouse override if it's unset but it should be.
	if !io.WantCaptureMouse() && (isDragging || isClicked) && wm.mouseConsumerOverride == nil {
		wm.mouseConsumerOverride = mousePane
	}

	// Set the mouse cursor
	setCursorForPane := func(p Pane) {
		if sl, ok := p.(*SplitLine); ok {
			if sl.Axis == SplitAxisX {
				imgui.SetMouseCursor(imgui.MouseCursorResizeEW)
			} else {
				imgui.SetMouseCursor(imgui.MouseCursorResizeNS)
			}
		} else {
			imgui.SetMouseCursor(imgui.MouseCursorArrow) // just to be sure; may be already
		}
	}
	if wm.mouseConsumerOverride != nil {
		setCursorForPane(wm.mouseConsumerOverride)
	} else {
		setCursorForPane(mousePane)
	}

	mouseInScope := func(p imgui.Vec2, extent Extent2D) bool {
		if io.WantCaptureMouse() {
			return false
		}
		return extent.Inside([2]float32{p.X, p.Y})
	}

	// Get all of the draw lists
	var commandBuffer CommandBuffer
	if fbSize[0] > 0 && fbSize[1] > 0 {
		commandBuffer.ClearRGB(positionConfig.GetColorScheme().Background)

		// Draw the status bar underneath the menu bar
		wmDrawStatusBar(fbSize, displaySize, heightRatio, topControlsHeight, &commandBuffer)

		positionConfig.DisplayRoot.VisitPanesWithBounds(wm.nodeFilter, fbFull, displayFull, displayFull, displayTrueFull,
			func(fb Extent2D, disp Extent2D, parentDisp Extent2D, fullDisp Extent2D, pane Pane) {
				ctx := PaneContext{
					paneExtent:        disp,
					parentPaneExtent:  parentDisp,
					fullDisplayExtent: fullDisp,
					highDPIScale:      fbFull.Height() / displayFull.Height(),
					platform:          platform,
					events:            eventStream,
					cs:                positionConfig.GetColorScheme()}

				if !wm.statusBarHasFocus && pane == wm.keyboardFocusPane {
					ctx.InitializeKeyboard()
				}

				ownsMouse := wm.mouseConsumerOverride == pane ||
					(wm.mouseConsumerOverride == nil && mouseInScope(mousePos, disp) &&
						!io.WantCaptureMouse())
				if ownsMouse {
					ctx.InitializeMouse()
				}

				commandBuffer.Scissor(int(fb.p0[0]), int(fb.p0[1]), int(fb.Width()+.5), int(fb.Height()+.5))
				commandBuffer.Viewport(int(fb.p0[0]), int(fb.p0[1]), int(fb.Width()+.5), int(fb.Height()+.5))
				pane.Draw(&ctx, &commandBuffer)
				commandBuffer.ResetState()

				if pane == mousePane && wm.handlePanePick != nil {
					// Blend in the plane selection quad
					ctx.SetWindowCoordinateMatrices(&commandBuffer)
					commandBuffer.Blend()

					w, h := disp.Width(), disp.Height()
					p := [4][2]float32{[2]float32{0, 0}, [2]float32{w, 0}, [2]float32{w, h}, [2]float32{0, h}}
					pidx := commandBuffer.Float2Buffer(p[:])

					indices := [4]int32{0, 1, 2, 3}
					indidx := commandBuffer.IntBuffer(indices[:])

					commandBuffer.SetRGBA(RGBA{0.5, 0.5, 0.5, 0.5})
					commandBuffer.VertexArray(pidx, 2, 2*4)
					commandBuffer.DrawQuads(indidx, 4)
					commandBuffer.ResetState()
				}
				if !wm.statusBarHasFocus && pane == wm.keyboardFocusPane {
					// Draw a border around it
					ctx.SetWindowCoordinateMatrices(&commandBuffer)
					drawBorder(&commandBuffer, disp.Width(), disp.Height(), ctx.cs.TextHighlight)
				}
			})

		stats.render = renderer.RenderCommandBuffer(&commandBuffer)
	}
}

func drawBorder(cb *CommandBuffer, w, h float32, color RGB) {
	p := [4][2]float32{[2]float32{1, 1}, [2]float32{w - 1, 1}, [2]float32{w - 1, h - 1}, [2]float32{1, h - 1}}
	pidx := cb.Float2Buffer(p[:])

	indidx := cb.IntBuffer([]int32{0, 1, 1, 2, 2, 3, 3, 0})

	cb.SetRGB(color)
	cb.VertexArray(pidx, 2, 2*4)
	cb.DrawLines(indidx, 8)
	cb.ResetState()
}

func wmActivateNewConfig(old *PositionConfig, nw *PositionConfig, cs *ColorScheme) {
	// Position changed. First deactivate the old one
	if old != nil {
		old.DisplayRoot.VisitPanes(func(p Pane) { p.Deactivate() })
	}
	wm.showPaneSettings = make(map[Pane]*bool)
	wm.showPaneName = make(map[Pane]string)
	nw.DisplayRoot.VisitPanes(func(p Pane) { p.Activate(cs) })
	wm.keyboardFocusPane = nil
}

func wmDrawStatusBar(fbSize [2]float32, displaySize [2]float32, heightRatio float32, topControlsHeight float32, cb *CommandBuffer) {
	statusBarFbExtent := Extent2D{p0: [2]float32{0, fbSize[1] - heightRatio*topControlsHeight},
		p1: [2]float32{fbSize[0], fbSize[1] - heightRatio*wm.topControlsHeight}}
	statusBarDisplayExtent := Extent2D{p0: [2]float32{0, displaySize[1] - topControlsHeight},
		p1: [2]float32{displaySize[0], displaySize[1] - wm.topControlsHeight}}

	cb.Scissor(int(statusBarFbExtent.p0[0]), int(statusBarFbExtent.p0[1]),
		int(statusBarFbExtent.Width()+.5), int(statusBarFbExtent.Height()+.5))
	cb.Viewport(int(statusBarFbExtent.p0[0]), int(statusBarFbExtent.p0[1]),
		int(statusBarFbExtent.Width()+.5), int(statusBarFbExtent.Height()+.5))

	statusBarHeight := globalConfig.statusBar.Height()
	proj := mgl32.Ortho2D(0, displaySize[0], 0, statusBarHeight)
	cb.LoadProjectionMatrix(proj)
	cb.LoadModelViewMatrix(mgl32.Ident4())

	ctx := PaneContext{
		paneExtent:        statusBarDisplayExtent,
		parentPaneExtent:  Extent2D{p1: displaySize},
		fullDisplayExtent: Extent2D{p1: displaySize},
		highDPIScale:      heightRatio,
		platform:          platform,
		events:            eventStream,
		cs:                positionConfig.GetColorScheme(),
	}
	ctx.InitializeKeyboard()

	wm.statusBarHasFocus = globalConfig.statusBar.Draw(&ctx, cb)
	if wm.statusBarHasFocus {
		drawBorder(cb, displaySize[0], statusBarHeight, ctx.cs.TextHighlight)
	}

	cb.ResetState()
}

///////////////////////////////////////////////////////////////////////////
// ModalButtonSet

// ModalButtonSet handles some of the housekeeping for the buttons used
// when editing configs, allowing buttons to be shown or not depending on
// external state and handling pane selection through provided callbacks.
type ModalButtonSet struct {
	active    string
	names     []string
	callbacks []func() func(Pane) bool
	show      []func() bool
}

// Add adds a button with the given text to the button set. The value
// returned show callback determines whether the button is drawn, and the
// selected callback is called if the button is pressed and a Pane is then
// selected by the user.
func (m *ModalButtonSet) Add(text string, selected func() func(Pane) bool, show func() bool) {
	m.names = append(m.names, text)
	m.callbacks = append(m.callbacks, selected)
	m.show = append(m.show, show)
}

// Clear deselects the currently active button, if any.
func (m *ModalButtonSet) Clear() {
	m.active = ""
}

// Draw draws the buttons and handles user interaction.
func (m *ModalButtonSet) Draw() {
	for i, name := range m.names {
		// Skip invisible buttons.
		if !m.show[i]() {
			continue
		}

		if m.active == name {
			// If the button has already been pressed and we're waiting for
			// a pane to be selected draw it in its 'hovered' state,
			// regardless of whether the mouse is actually hovering over
			// it.
			imgui.PushID(m.active)

			h := imgui.CurrentStyle().Color(imgui.StyleColorButtonHovered)
			imgui.PushStyleColor(imgui.StyleColorButton, h) // active

			imgui.Button(name)
			if imgui.IsItemClicked() {
				// If the button is clicked again, roll back and deselect
				// it.
				wm.handlePanePick = nil
				m.active = ""
			}
			imgui.PopStyleColorV(1)
			imgui.PopID()
		} else if imgui.Button(name) {
			// First click of the button. Make it active.
			m.active = name

			wm.paneFirstPick = nil

			// Get the actual callback for pane selection (and allow the
			// user to do some prep work, knowing they've been selected)
			callback := m.callbacks[i]()

			// Register the pane pick callback to dispatch pane selection
			// to this button's callback.
			wm.handlePanePick = func(pane Pane) bool {
				// But now wrap the pick callback in our own function so
				// that we can clear |active| after successful selection.
				result := callback(pane)
				if result {
					m.active = ""
				}
				return result
			}
		}
		// Keep all of the buttons on the same line.
		if i < len(m.names)-1 {
			imgui.SameLine()
		}
	}
}

///////////////////////////////////////////////////////////////////////////
// StatusBar

// StatusBar manages state and displays status for F-key based commands.
type StatusBar struct {
	activeCommand      FKeyCommand
	inputFocus         int      // which input field is focused
	inputCursor        int      // cursor position in the current input field
	commandArgs        []string // user input for each command argument
	commandArgErrors   []string
	commandErrorString string // error to show to user
	eventsId           EventSubscriberId
}

func MakeStatusBar() *StatusBar {
	return &StatusBar{eventsId: eventStream.Subscribe()}
}

// Height returns the height of the status bar in pixels.
func (sb *StatusBar) Height() float32 {
	return float32(10 + ui.font.size) // One line plus some padding
}

func (sb *StatusBar) Draw(ctx *PaneContext, cb *CommandBuffer) bool {
	sb.processEvents(ctx)
	sb.processKeys(ctx.keyboard)
	return sb.draw(ctx, cb)
}

func (sb *StatusBar) processEvents(ctx *PaneContext) {
	if sb.activeCommand == nil {
		return
	}

	// Go through the event stream and see if an aircraft has been
	// selected; if so, and if there is an active command that takes an
	// aircraft callsign, use the selected aircraft's callsign for the
	// corresponding command argument.
	for _, event := range ctx.events.Get(sb.eventsId) {
		if sel, ok := event.(*SelectedAircraftEvent); ok {
			// Look for a command argument that takes an aircraft callsign.
			for i, ty := range sb.activeCommand.ArgTypes() {
				if _, ok := ty.(*AircraftCommandArg); ok {
					// Found one; override the callsign.
					sb.commandArgs[i] = sel.ac.callsign
					sb.commandArgErrors[i] = ""
					if sb.inputFocus == i {
						if len(sb.commandArgs) > 0 {
							// If the cursor is currently in the input
							// field for the callsign, then skip to the
							// next field, if there is another one.
							sb.inputFocus = (sb.inputFocus + 1) % len(sb.commandArgs)
							sb.inputCursor = 0
						} else {
							// Otherwise move the cursor to the end of the input.
							sb.inputCursor = len(sb.commandArgs[i])
						}
					}
					break
				}
			}
		}
	}
}

func (sb *StatusBar) processKeys(keyboard *KeyboardState) {
	// See if any of the F-keys are pressed
	for i := 1; i <= 12; i++ {
		if keyboard.IsPressed(Key(KeyF1 - 1 + i)) {
			// Figure out which FKeyCommand is bound to the f-key, if any.
			var cmd string
			if keyboard.IsPressed(KeyShift) {
				if cmd = globalConfig.ShiftFKeyMappings[i]; cmd == "" {
					sb.commandErrorString = "No command bound to shift-F" + fmt.Sprintf("%d", i)
				}
			} else {
				if cmd = globalConfig.FKeyMappings[i]; cmd == "" {
					sb.commandErrorString = "No command bound to F" + fmt.Sprintf("%d", i)
				}
			}

			// If there's a command associated with the pressed f-key, set
			// things up to get its argument values from the user.
			if cmd != "" {
				sb.activeCommand = allFKeyCommands[cmd]
				if sb.activeCommand == nil {
					// This shouldn't happen unless the config.json file is
					// corrupt or a key used in the allFKeyCommands map has
					// changed.
					lg.Errorf(cmd + ": no f-key command of that name")
				} else {
					// Set things up to get the arguments for this command.
					sb.commandArgs = make([]string, len(sb.activeCommand.ArgTypes()))
					sb.commandArgErrors = make([]string, len(sb.activeCommand.ArgTypes()))
					sb.commandErrorString = ""
					sb.inputFocus = 0
					sb.inputCursor = 0
				}
			}
		}
	}

	if keyboard.IsPressed(KeyEscape) {
		// Clear out the current command.
		sb.activeCommand = nil
		sb.commandErrorString = ""
	}
}

func (sb *StatusBar) draw(ctx *PaneContext, cb *CommandBuffer) bool {
	// Draw lines to delineate the top and bottom of the status bar
	ld := ColoredLinesDrawBuilder{}
	ld.AddLine([2]float32{5, 1}, [2]float32{ctx.paneExtent.p1[0] - 5, 1}, ctx.cs.UIControl)
	h := ctx.paneExtent.Height() - 1
	ld.AddLine([2]float32{5, h}, [2]float32{ctx.paneExtent.p1[0] - 5, h}, ctx.cs.UIControl)
	cb.LineWidth(1 * ctx.highDPIScale)
	ld.GenerateCommands(cb)

	// Nothing more to do if there is no active command, so bail out here.
	if sb.activeCommand == nil {
		return false
	}

	cursorStyle := TextStyle{Font: ui.font, Color: ctx.cs.Background,
		DrawBackground: true, BackgroundColor: ctx.cs.Text}
	textStyle := TextStyle{Font: ui.font, Color: ctx.cs.Text}
	inputStyle := TextStyle{Font: ui.font, Color: ctx.cs.TextHighlight}
	errorStyle := TextStyle{Font: ui.font, Color: ctx.cs.TextError}

	td := TextDrawBuilder{}
	// Current cursor position for text drawing; this will advance as we
	// start adding text.
	textp := [2]float32{15, 5 + float32(ui.font.size)}

	// Command description
	textp = td.AddText(sb.activeCommand.Name(), textp, textStyle)

	// Draw text for all of the arguments, including both the prompt and the current value.
	argTypes := sb.activeCommand.ArgTypes()
	var textEditResult int
	for i, arg := range sb.commandArgs {
		// Prompt for the argument.
		textp = td.AddText(" "+argTypes[i].Prompt()+": ", textp, textStyle)

		if i == sb.inputFocus {
			// If this argument currently has the cursor, draw a text editing field and handle
			// keyboard events.
			textEditResult, textp = uiDrawTextEdit(&sb.commandArgs[sb.inputFocus], &sb.inputCursor,
				ctx.keyboard, textp, inputStyle, cursorStyle, cb)
			// All of the commands expect upper-case args, so always ensure that immediately.
			sb.commandArgs[sb.inputFocus] = strings.ToUpper(sb.commandArgs[sb.inputFocus])
		} else {
			// Otherwise it's an unfocused argument. If it's currently an
			// empty string, draw an underbar.
			if arg == "" {
				textp = td.AddText("_", textp, inputStyle)
			} else {
				textp = td.AddText(arg, textp, inputStyle)
			}
		}

		// If the user tried to run the command and there was an issue
		// related to this argument, print the error message.
		if sb.commandArgErrors[i] != "" {
			textp = td.AddText(" "+sb.commandArgErrors[i]+" ", textp, errorStyle)
		}

		// Expand the argument and see how many completions we find.
		completion, err := argTypes[i].Expand(arg)
		if err == nil {
			if completion != arg {
				// We have a single completion that is different than what the user typed;
				// draw an arrow and the completion text so the user can
				// see what will actually be used.
				textp = td.AddText(" "+FontAwesomeIconArrowRight+" "+completion, textp, textStyle)
			}
		} else {
			// Completions are implicitly validated so if there are none the user input is
			// not valid and if there are multiple it's ambiguous; either way, indicate
			// the input is not valid.
			textp = td.AddText(" "+FontAwesomeIconExclamationTriangle+" ", textp, errorStyle)
		}
	}

	// Handle changes in focus, etc., based on the input to the text edit
	// field.
	switch textEditResult {
	case TextEditReturnNone:
		// nothing

	case TextEditReturnTextChanged:
		// The user input changed, so clear out any error message since it
		// may no longer be valid.
		sb.commandErrorString = ""
		sb.commandArgErrors = make([]string, len(sb.commandArgErrors))

	case TextEditReturnEnter:
		// The user hit enter; try to run the command

		// Run completion on all of the arguments; this also checks their validity.
		var completedArgs []string
		argTypes := sb.activeCommand.ArgTypes()
		sb.commandErrorString = ""
		anyArgErrors := false
		for i, arg := range sb.commandArgs {
			if comp, err := argTypes[i].Expand(arg); err == nil {
				completedArgs = append(completedArgs, comp)
				sb.commandArgErrors[i] = ""
			} else {
				sb.commandArgErrors[i] = err.Error()
				anyArgErrors = true
			}
		}

		// Something went wrong, so don't try running the command.
		if anyArgErrors {
			break
		}

		err := sb.activeCommand.Do(completedArgs)
		if err != nil {
			// Failure. Grab the command's error message to display.
			sb.commandErrorString = err.Error()
		} else {
			// Success; clear out the command.
			sb.activeCommand = nil
			sb.commandArgs = nil
			sb.commandArgErrors = nil
		}

	case TextEditReturnNext:
		// Go to the next input field.
		sb.inputFocus = (sb.inputFocus + 1) % len(sb.commandArgs)
		sb.inputCursor = len(sb.commandArgs[sb.inputFocus])

	case TextEditReturnPrev:
		// Go to the previous input field.
		sb.inputFocus = (sb.inputFocus + len(sb.commandArgs) - 1) % len(sb.commandArgs)
		sb.inputCursor = len(sb.commandArgs[sb.inputFocus])
	}

	// Display the error string if it's set
	if sb.commandErrorString != "" {
		textp = td.AddText("   "+sb.commandErrorString, textp, errorStyle)
	}

	// Finally, add the text drawing commands to the graphics command buffer.
	td.GenerateCommands(cb)

	return true
}
