package game

const (
	arenaCellWidth       = 2
	minArenaWidth        = 32
	minArenaHeight       = 14
	preferredArenaWidth  = 42
	preferredArenaHeight = 20
	maxArenaWidth        = 52
	maxArenaHeight       = 24
	arenaTopOffset       = 5
)

type Arena struct {
	X      int
	Y      int
	Width  int
	Height int
}

func ArenaForScreen(screenWidth int, screenHeight int) Arena {
	if screenWidth <= 0 || screenHeight <= 0 {
		return Arena{Width: 1, Height: 1}
	}

	availableWidth := screenWidth - 2
	availableHeight := screenHeight - arenaTopOffset - 1
	if screenHeight < 8 {
		availableHeight = screenHeight - 2
	}
	if availableWidth < arenaCellWidth {
		availableWidth = arenaCellWidth
	}
	if availableHeight < 1 {
		availableHeight = 1
	}

	logicalWidth := availableWidth / arenaCellWidth
	logicalHeight := availableHeight
	if logicalWidth > maxArenaWidth {
		logicalWidth = maxArenaWidth
	}
	if logicalHeight > maxArenaHeight {
		logicalHeight = maxArenaHeight
	}
	if logicalWidth < 1 {
		logicalWidth = 1
	}
	if logicalHeight < 1 {
		logicalHeight = 1
	}

	renderWidth := logicalWidth * arenaCellWidth
	x := (screenWidth - renderWidth) / 2
	if x < 1 {
		x = 1
	}

	y := arenaTopOffset
	if screenHeight < 8 {
		y = 1
	} else if availableHeight > logicalHeight {
		y = arenaTopOffset + (availableHeight-logicalHeight)/2
	}
	if y < 1 {
		y = 1
	}

	return Arena{X: x, Y: y, Width: logicalWidth, Height: logicalHeight}
}

func (a Arena) RenderWidth() int {
	return a.Width * arenaCellWidth
}

func (a Arena) CellX(logicalX int) int {
	return a.X + logicalX*arenaCellWidth
}

func (a Arena) CellY(logicalY int) int {
	return a.Y + logicalY
}
