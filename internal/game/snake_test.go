package game

import "testing"

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

func TestSnakeRejectsImmediateReverseWhenLongerThanOne(t *testing.T) {
	game := NewSnakeGame(5, 5, func(int) int { return 0 })
	game.Snake = []Point{{X: 2, Y: 2}, {X: 1, Y: 2}}
	game.Dir = DirectionRight

	game.ChangeDirection(DirectionLeft)

	if game.Dir != DirectionRight {
		t.Fatalf("Dir = %v, want %v", game.Dir, DirectionRight)
	}
}

func TestSnakeSupportsOneCellBoard(t *testing.T) {
	game := NewSnakeGame(1, 1, func(int) int { return 0 })

	if game.Food != (Point{X: -1, Y: -1}) {
		t.Fatalf("Food = %#v, want no food", game.Food)
	}
}
