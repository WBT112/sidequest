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

func TestSnakeResizeRestartsBoardSafely(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Snake = []Point{{X: 4, Y: 4}, {X: 3, Y: 4}}
	game.Score = 3
	game.Over = true

	game.Resize(2, 2)

	if game.Width != 2 || game.Height != 2 {
		t.Fatalf("board = %dx%d, want 2x2", game.Width, game.Height)
	}
	if game.Score != 0 {
		t.Fatalf("Score = %d, want reset score 0", game.Score)
	}
	if game.Over {
		t.Fatal("game.Over = true, want false")
	}
	for _, point := range game.Snake {
		if point.X < 0 || point.X >= game.Width || point.Y < 0 || point.Y >= game.Height {
			t.Fatalf("snake point out of resized board: %#v", point)
		}
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
