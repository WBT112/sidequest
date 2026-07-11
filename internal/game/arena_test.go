package game

import "testing"

func TestArenaForScreenCapsAndCentersLargePanes(t *testing.T) {
	arena := ArenaForScreen(180, 50)

	if arena.Width != maxArenaWidth || arena.Height != maxArenaHeight {
		t.Fatalf("arena = %dx%d, want max %dx%d", arena.Width, arena.Height, maxArenaWidth, maxArenaHeight)
	}
	if arena.X != (180-maxArenaWidth*arenaCellWidth)/2 {
		t.Fatalf("arena.X = %d, want centered", arena.X)
	}
	if arena.Y <= arenaTopOffset {
		t.Fatalf("arena.Y = %d, want vertically centered below HUD", arena.Y)
	}
}

func TestArenaForScreenUsesAvailableMinimumPane(t *testing.T) {
	arena := ArenaForScreen(66, 20)

	if arena.Width != minArenaWidth || arena.Height != minArenaHeight {
		t.Fatalf("arena = %dx%d, want minimum %dx%d", arena.Width, arena.Height, minArenaWidth, minArenaHeight)
	}
}

func TestArenaForScreenUsesPreferredPane(t *testing.T) {
	arena := ArenaForScreen(86, 26)

	if arena.Width != preferredArenaWidth || arena.Height != preferredArenaHeight {
		t.Fatalf("arena = %dx%d, want preferred %dx%d", arena.Width, arena.Height, preferredArenaWidth, preferredArenaHeight)
	}
}

func TestArenaForScreenUsesAvailableSupportedPane(t *testing.T) {
	arena := ArenaForScreen(80, 24)

	if arena.Width != 39 || arena.Height != 18 {
		t.Fatalf("arena = %dx%d, want 39x18", arena.Width, arena.Height)
	}
	if arena.X < 1 || arena.Y < 1 {
		t.Fatalf("arena offset = %d,%d, want visible", arena.X, arena.Y)
	}
}

func TestArenaForScreenDegradesForTinyPane(t *testing.T) {
	arena := ArenaForScreen(5, 4)

	if arena.Width < 1 || arena.Height < 1 {
		t.Fatalf("arena = %dx%d, want positive", arena.Width, arena.Height)
	}
}

func TestArenaCellMappingUsesTwoColumns(t *testing.T) {
	arena := Arena{X: 7, Y: 5, Width: 42, Height: 20}

	if got := arena.CellX(3); got != 13 {
		t.Fatalf("CellX(3) = %d, want 13", got)
	}
	if got := arena.CellY(4); got != 9 {
		t.Fatalf("CellY(4) = %d, want 9", got)
	}
}
