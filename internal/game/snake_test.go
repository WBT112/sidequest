package game

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestSnakeFoodNeverAppearsInsideSnake(t *testing.T) {
	game := NewSnakeGame(4, 3, func(int) int { return 0 })
	game.Snake = []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}}

	if !game.PlaceFood() {
		t.Fatal("PlaceFood returned false, want true")
	}
	for _, point := range game.Snake {
		if game.Food == point {
			t.Fatalf("food placed inside snake at %#v", game.Food)
		}
	}
}

func TestSnakeGrowsAndScoresAfterEatingFood(t *testing.T) {
	game := NewSnakeGame(5, 3, func(int) int { return 0 })
	game.Snake = []Point{{X: 1, Y: 1}}
	game.Dir = DirectionRight
	game.Food = Point{X: 2, Y: 1}

	result := game.Step()

	if result != StepAteFood {
		t.Fatalf("Step result = %v, want %v", result, StepAteFood)
	}
	if game.Score != 1 {
		t.Fatalf("Score = %d, want 1", game.Score)
	}
	if len(game.Snake) != 2 {
		t.Fatalf("snake length = %d, want 2", len(game.Snake))
	}
	if game.Snake[0] != (Point{X: 2, Y: 1}) {
		t.Fatalf("head = %#v, want food cell", game.Snake[0])
	}
}

func TestSnakeForcedGrowPreservesFoodAndScore(t *testing.T) {
	game := NewSnakeGame(5, 3, func(int) int { return 0 })
	game.Snake = []Point{{X: 1, Y: 1}}
	game.Dir = DirectionRight
	game.Food = Point{X: 0, Y: 0}
	game.Score = 7

	result := game.StepGrow()

	if result != StepAteFood {
		t.Fatalf("StepGrow result = %v, want %v", result, StepAteFood)
	}
	if game.Score != 7 {
		t.Fatalf("Score = %d, want preserved 7", game.Score)
	}
	if game.Food != (Point{X: 0, Y: 0}) {
		t.Fatalf("Food = %#v, want preserved", game.Food)
	}
	if len(game.Snake) != 2 || game.Snake[0] != (Point{X: 2, Y: 1}) {
		t.Fatalf("snake = %#v, want grown into next cell", game.Snake)
	}
}

func TestSnakeUsesConfiguredFoodScore(t *testing.T) {
	game := NewSnakeGame(5, 3, func(int) int { return 0 })
	game.Snake = []Point{{X: 1, Y: 1}}
	game.Dir = DirectionRight
	game.Food = Point{X: 2, Y: 1}
	game.FoodScore = 17

	result := game.Step()

	if result != StepAteFood {
		t.Fatalf("Step result = %v, want %v", result, StepAteFood)
	}
	if game.Score != 17 {
		t.Fatalf("Score = %d, want 17", game.Score)
	}
}

func TestSnakeRetriesMissingFoodAfterMovement(t *testing.T) {
	game := NewSnakeGame(2, 2, func(int) int { return 0 })
	game.Snake = []Point{{X: 0, Y: 0}, {X: 0, Y: 1}}
	game.Dir = DirectionRight
	game.Food = Point{X: -1, Y: -1}

	result := game.Step()

	if result != StepMoved {
		t.Fatalf("Step result = %v, want %v", result, StepMoved)
	}
	if !game.FoodValid(nil) {
		t.Fatalf("Food = %#v, want valid food after movement freed space", game.Food)
	}
}

func TestSnakeRetriesMissingFoodAfterResize(t *testing.T) {
	game := NewSnakeGame(1, 1, func(int) int { return 0 })
	game.Food = Point{X: -1, Y: -1}

	game.Resize(2, 1)

	if !game.FoodValid(nil) {
		t.Fatalf("Food = %#v, want valid food after resize freed space", game.Food)
	}
}

func TestSnakeRetriesMissingFoodAfterRecover(t *testing.T) {
	game := NewSnakeGame(2, 1, func(int) int { return 0 })
	game.Snake = []Point{{X: 0, Y: 0}, {X: 1, Y: 0}}
	game.Food = Point{X: -1, Y: -1}
	game.Over = true

	game.Recover()

	if !game.FoodValid(nil) {
		t.Fatalf("Food = %#v, want valid food after recovery reset the snake", game.Food)
	}
}

func TestSnakeEndsRoundOnWallCollision(t *testing.T) {
	game := NewSnakeGame(3, 3, func(int) int { return 0 })
	game.Snake = []Point{{X: 2, Y: 1}}
	game.Dir = DirectionRight
	game.Food = Point{X: 0, Y: 0}

	result := game.Step()

	if result != StepHitWall {
		t.Fatalf("Step result = %v, want %v", result, StepHitWall)
	}
	if !game.Over {
		t.Fatal("game.Over = false, want true")
	}
}

func TestSnakeEndsRoundOnSelfCollision(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Snake = []Point{
		{X: 2, Y: 2},
		{X: 2, Y: 3},
		{X: 1, Y: 3},
		{X: 1, Y: 2},
	}
	game.Dir = DirectionDown
	game.Food = Point{X: 0, Y: 0}

	result := game.Step()

	if result != StepHitSelf {
		t.Fatalf("Step result = %v, want %v", result, StepHitSelf)
	}
	if !game.Over {
		t.Fatal("game.Over = false, want true")
	}
}

func TestSnakeMayMoveIntoVacatedTail(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Snake = []Point{
		{X: 2, Y: 2},
		{X: 1, Y: 3},
		{X: 2, Y: 3},
	}
	game.Dir = DirectionDown
	game.Food = Point{X: 0, Y: 0}

	result := game.Step()

	if result != StepMoved {
		t.Fatalf("Step result = %v, want %v", result, StepMoved)
	}
	if game.Over {
		t.Fatal("game.Over = true, want false")
	}
}

func TestSnakeResizePreservesStateWhenCoordinatesRemainValid(t *testing.T) {
	game := NewSnakeGame(6, 5, func(int) int { return 0 })
	game.Snake = []Point{{X: 3, Y: 2}, {X: 2, Y: 2}, {X: 1, Y: 2}}
	game.Food = Point{X: 4, Y: 2}
	game.Dir = DirectionRight
	game.PendingDirs = []Direction{DirectionUp}
	game.Score = 3
	game.FoodScore = 5
	game.FoodHeat = 4

	result := game.Resize(8, 6)

	if result != ResizeUnchanged {
		t.Fatalf("Resize result = %v, want %v", result, ResizeUnchanged)
	}
	if game.Width != 8 || game.Height != 6 {
		t.Fatalf("board = %dx%d, want 8x6", game.Width, game.Height)
	}
	assertSnake(t, game.Snake, []Point{{X: 3, Y: 2}, {X: 2, Y: 2}, {X: 1, Y: 2}})
	if game.Food != (Point{X: 4, Y: 2}) {
		t.Fatalf("Food = %#v, want preserved", game.Food)
	}
	if game.Score != 3 || game.FoodScore != 5 || game.FoodHeat != 4 {
		t.Fatalf("score state = score %d foodScore %d foodHeat %d, want preserved", game.Score, game.FoodScore, game.FoodHeat)
	}
	if len(game.PendingDirs) != 0 {
		t.Fatalf("PendingDirs = %v, want cleared", game.PendingDirs)
	}
}

func TestSnakeResizeTranslatesShapeIntoBounds(t *testing.T) {
	game := NewSnakeGame(5, 4, func(int) int { return 0 })
	game.Snake = []Point{{X: 4, Y: 2}, {X: 3, Y: 2}, {X: 2, Y: 2}}
	game.Food = Point{X: 0, Y: 0}
	game.Score = 7

	result := game.Resize(4, 4)

	if result != ResizeTranslated {
		t.Fatalf("Resize result = %v, want %v", result, ResizeTranslated)
	}
	assertSnake(t, game.Snake, []Point{{X: 3, Y: 2}, {X: 2, Y: 2}, {X: 1, Y: 2}})
	if game.Score != 7 {
		t.Fatalf("Score = %d, want preserved", game.Score)
	}
	assertSnakeValid(t, game.Snake, game.Width, game.Height)
}

func TestSnakeResizeReflowsWhenShapeCannotFit(t *testing.T) {
	game := NewSnakeGame(3, 6, func(int) int { return 0 })
	game.Snake = []Point{{X: 1, Y: 5}, {X: 1, Y: 4}, {X: 1, Y: 3}, {X: 1, Y: 2}, {X: 1, Y: 1}}
	game.Food = Point{X: 2, Y: 5}
	game.Dir = DirectionUp
	game.PendingDirs = []Direction{DirectionLeft}
	game.Score = 11

	result := game.Resize(5, 3)

	if result != ResizeReflowed {
		t.Fatalf("Resize result = %v, want %v", result, ResizeReflowed)
	}
	if len(game.Snake) != 5 {
		t.Fatalf("snake length = %d, want 5", len(game.Snake))
	}
	if game.Score != 11 {
		t.Fatalf("Score = %d, want preserved", game.Score)
	}
	if len(game.PendingDirs) != 0 {
		t.Fatalf("PendingDirs = %v, want cleared", game.PendingDirs)
	}
	assertSnakeValid(t, game.Snake, game.Width, game.Height)
}

func TestSnakeResizeTooSmallLeavesStateUntouched(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Snake = []Point{{X: 4, Y: 4}, {X: 3, Y: 4}, {X: 2, Y: 4}, {X: 1, Y: 4}, {X: 0, Y: 4}}
	game.Food = Point{X: 0, Y: 0}
	game.Score = 3
	originalSnake := append([]Point(nil), game.Snake...)

	result := game.Resize(2, 2)

	if result != ResizeTooSmall {
		t.Fatalf("Resize result = %v, want %v", result, ResizeTooSmall)
	}
	if game.Width != 5 || game.Height != 5 {
		t.Fatalf("board = %dx%d, want unchanged 5x5", game.Width, game.Height)
	}
	assertSnake(t, game.Snake, originalSnake)
	if game.Score != 3 {
		t.Fatalf("Score = %d, want preserved", game.Score)
	}
}

func TestSnakeRecoverPreservesScoreAndClearsGameOver(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Score = 42
	game.FoodScore = 17
	game.Over = true

	game.Recover()

	if game.Over {
		t.Fatal("Over = true, want false")
	}
	if game.Score != 42 || game.FoodScore != 17 {
		t.Fatalf("score=%d foodScore=%d, want preserved", game.Score, game.FoodScore)
	}
}

func TestSnakeRejectsImmediateReverseWhenLongerThanOne(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Snake = []Point{{X: 2, Y: 2}, {X: 1, Y: 2}}
	game.Dir = DirectionRight

	if game.ChangeDirection(DirectionLeft) {
		t.Fatal("ChangeDirection returned true, want rejected reverse")
	}

	if game.Dir != DirectionRight {
		t.Fatalf("Dir = %v, want %v", game.Dir, DirectionRight)
	}
}

func TestSnakeCanStartMovingLeft(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Food = Point{X: 0, Y: 0}

	if !game.ChangeDirection(DirectionLeft) {
		t.Fatal("ChangeDirection(left) returned false for one-segment snake")
	}
	if result := game.Step(); result != StepMoved {
		t.Fatalf("Step result = %v, want moved", result)
	}
	if game.Dir != DirectionLeft || game.Snake[0] != (Point{X: 1, Y: 2}) {
		t.Fatalf("dir=%v head=%#v, want left to 1,2", game.Dir, game.Snake[0])
	}
}

func TestSnakeQueuesRapidLeftUpRightTurns(t *testing.T) {
	game := NewSnakeGame(7, 7, func(int) int { return 0 })
	game.Snake = []Point{{X: 3, Y: 3}, {X: 4, Y: 3}, {X: 5, Y: 3}}
	game.Dir = DirectionLeft
	game.Food = Point{X: 0, Y: 0}

	if !game.ChangeDirection(DirectionUp) {
		t.Fatal("ChangeDirection(up) returned false, want queued")
	}
	if !game.ChangeDirection(DirectionRight) {
		t.Fatal("ChangeDirection(right) returned false, want queued")
	}
	if game.Dir != DirectionLeft {
		t.Fatalf("Dir changed before movement tick: %v", game.Dir)
	}

	if result := game.Step(); result != StepMoved {
		t.Fatalf("first Step result = %v, want moved", result)
	}
	if game.Dir != DirectionUp || game.Snake[0] != (Point{X: 3, Y: 2}) {
		t.Fatalf("first turn dir=%v head=%#v, want up to 3,2", game.Dir, game.Snake[0])
	}
	if result := game.Step(); result != StepMoved {
		t.Fatalf("second Step result = %v, want moved", result)
	}
	if game.Dir != DirectionRight || game.Snake[0] != (Point{X: 4, Y: 2}) {
		t.Fatalf("second turn dir=%v head=%#v, want right to 4,2", game.Dir, game.Snake[0])
	}
	if game.Over {
		t.Fatal("game ended after queued corner turns")
	}
}

func TestSnakeQueuesRapidRightUpLeftTurns(t *testing.T) {
	game := NewSnakeGame(7, 7, func(int) int { return 0 })
	game.Snake = []Point{{X: 3, Y: 3}, {X: 2, Y: 3}, {X: 1, Y: 3}}
	game.Dir = DirectionRight
	game.Food = Point{X: 0, Y: 0}

	if !game.ChangeDirection(DirectionUp) {
		t.Fatal("ChangeDirection(up) returned false, want queued")
	}
	if !game.ChangeDirection(DirectionLeft) {
		t.Fatal("ChangeDirection(left) returned false, want queued")
	}

	game.Step()
	game.Step()

	if game.Dir != DirectionLeft || game.Snake[0] != (Point{X: 2, Y: 2}) {
		t.Fatalf("queued turns ended at dir=%v head=%#v, want left to 2,2", game.Dir, game.Snake[0])
	}
	if game.Over {
		t.Fatal("game ended after queued corner turns")
	}
}

func TestSnakeRejectsDirectReverseDirection(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Snake = []Point{{X: 2, Y: 2}, {X: 1, Y: 2}}
	game.Dir = DirectionRight

	if game.ChangeDirection(DirectionLeft) {
		t.Fatal("ChangeDirection(left) returned true, want rejected reverse")
	}
	if len(game.PendingDirs) != 0 {
		t.Fatalf("PendingDirs = %v, want empty", game.PendingDirs)
	}
}

func TestSnakeValidatesAgainstLastQueuedDirection(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Dir = DirectionLeft

	if !game.ChangeDirection(DirectionUp) {
		t.Fatal("ChangeDirection(up) returned false, want queued")
	}
	if game.ChangeDirection(DirectionDown) {
		t.Fatal("ChangeDirection(down) returned true, want rejected against queued up")
	}
	if got, want := len(game.PendingDirs), 1; got != want {
		t.Fatalf("len(PendingDirs) = %d, want %d", got, want)
	}
	if game.PendingDirs[0] != DirectionUp {
		t.Fatalf("PendingDirs[0] = %v, want up", game.PendingDirs[0])
	}
}

func TestSnakeDirectionQueueCapacityIsBounded(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Dir = DirectionRight

	if !game.ChangeDirection(DirectionUp) || !game.ChangeDirection(DirectionLeft) {
		t.Fatal("first two direction changes were not queued")
	}
	if game.ChangeDirection(DirectionDown) {
		t.Fatal("third direction change returned true, want capacity rejection")
	}
	if got, want := len(game.PendingDirs), directionQueueCapacity; got != want {
		t.Fatalf("len(PendingDirs) = %d, want %d", got, want)
	}
	if game.PendingDirs[0] != DirectionUp || game.PendingDirs[1] != DirectionLeft {
		t.Fatalf("PendingDirs = %v, want up then left", game.PendingDirs)
	}
}

func TestSnakeConsumesOneQueuedDirectionPerStep(t *testing.T) {
	game := NewSnakeGame(7, 7, func(int) int { return 0 })
	game.Snake = []Point{{X: 3, Y: 3}, {X: 2, Y: 3}, {X: 1, Y: 3}}
	game.Dir = DirectionRight
	game.Food = Point{X: 0, Y: 0}
	game.ChangeDirection(DirectionUp)
	game.ChangeDirection(DirectionLeft)

	game.Step()

	if game.Dir != DirectionUp {
		t.Fatalf("Dir = %v, want up", game.Dir)
	}
	if got, want := len(game.PendingDirs), 1; got != want {
		t.Fatalf("len(PendingDirs) = %d, want %d", got, want)
	}
	if game.PendingDirs[0] != DirectionLeft {
		t.Fatalf("remaining PendingDirs[0] = %v, want left", game.PendingDirs[0])
	}
}

func TestSnakeNextPointUsesNextQueuedDirection(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Snake = []Point{{X: 2, Y: 2}, {X: 1, Y: 2}}
	game.Dir = DirectionRight
	game.ChangeDirection(DirectionUp)

	if got, want := game.NextPoint(), (Point{X: 2, Y: 1}); got != want {
		t.Fatalf("NextPoint = %#v, want %#v", got, want)
	}
}

func TestSnakeLifecycleClearsPendingDirections(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.ChangeDirection(DirectionUp)
	game.ChangeDirection(DirectionLeft)

	game.Resize(6, 6)

	if len(game.PendingDirs) != 0 {
		t.Fatalf("Resize left PendingDirs = %v, want empty", game.PendingDirs)
	}

	game.ChangeDirection(DirectionUp)
	game.Over = true
	game.Recover()

	if len(game.PendingDirs) != 0 {
		t.Fatalf("Recover left PendingDirs = %v, want empty", game.PendingDirs)
	}

	newGame := NewSnakeGame(5, 5, func(int) int { return 0 })
	if len(newGame.PendingDirs) != 0 {
		t.Fatalf("new game PendingDirs = %v, want empty", newGame.PendingDirs)
	}
}

func assertSnake(t *testing.T, got []Point, want []Point) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("snake length = %d, want %d: %#v", len(got), len(want), got)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("snake[%d] = %#v, want %#v; full snake %#v", index, got[index], want[index], got)
		}
	}
}

func assertSnakeValid(t *testing.T, snake []Point, width int, height int) {
	t.Helper()
	seen := make(map[Point]bool, len(snake))
	for index, point := range snake {
		if point.X < 0 || point.X >= width || point.Y < 0 || point.Y >= height {
			t.Fatalf("snake[%d] out of bounds for %dx%d: %#v", index, width, height, point)
		}
		if seen[point] {
			t.Fatalf("snake[%d] duplicates point %#v in %#v", index, point, snake)
		}
		seen[point] = true
		if index == 0 {
			continue
		}
		previous := snake[index-1]
		distance := abs(previous.X-point.X) + abs(previous.Y-point.Y)
		if distance != 1 {
			t.Fatalf("snake[%d] = %#v is not adjacent to previous %#v in %#v", index, point, previous, snake)
		}
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func TestDirectionKeysMapToSameDirections(t *testing.T) {
	tests := []struct {
		name string
		key  *tcell.EventKey
		want Direction
	}{
		{name: "w", key: tcell.NewEventKey(tcell.KeyRune, 'w', tcell.ModNone), want: DirectionUp},
		{name: "up", key: tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone), want: DirectionUp},
		{name: "d", key: tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModNone), want: DirectionRight},
		{name: "right", key: tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), want: DirectionRight},
		{name: "s", key: tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModNone), want: DirectionDown},
		{name: "down", key: tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), want: DirectionDown},
		{name: "a", key: tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone), want: DirectionLeft},
		{name: "left", key: tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), want: DirectionLeft},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := directionFromKey(test.key)
			if !ok {
				t.Fatal("directionFromKey ok = false, want true")
			}
			if got != test.want {
				t.Fatalf("directionFromKey = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSnakeSupportsOneCellBoard(t *testing.T) {
	game := NewSnakeGame(1, 1, func(int) int { return 0 })

	if game.Food != (Point{X: -1, Y: -1}) {
		t.Fatalf("Food = %#v, want no food", game.Food)
	}
}
