package game

import "math/rand"

type Point struct {
	X int
	Y int
}

type Direction int

const (
	DirectionUp Direction = iota
	DirectionRight
	DirectionDown
	DirectionLeft
)

type StepResult int

const (
	directionQueueCapacity = 2
)

const (
	StepMoved StepResult = iota
	StepAteFood
	StepHitWall
	StepHitSelf
)

type SnakeGame struct {
	Width       int
	Height      int
	Snake       []Point
	Food        Point
	Dir         Direction
	PendingDirs []Direction
	Score       int
	FoodScore   int
	FoodHeat    int
	Over        bool

	randomInt func(int) int
}

func NewSnakeGame(width int, height int, randomInt func(int) int) *SnakeGame {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if randomInt == nil {
		randomInt = rand.Intn
	}

	game := &SnakeGame{
		Width:     width,
		Height:    height,
		Snake:     []Point{{X: width / 2, Y: height / 2}},
		Dir:       DirectionRight,
		FoodScore: 1,
		FoodHeat:  1,
		Food:      Point{X: -1, Y: -1},
		randomInt: randomInt,
	}
	game.PlaceFood()
	return game
}

func (g *SnakeGame) Resize(width int, height int) {
	randomInt := g.randomInt
	*g = *NewSnakeGame(width, height, randomInt)
}

func (g *SnakeGame) Recover() {
	score := g.Score
	foodScore := g.FoodScore
	foodHeat := g.FoodHeat
	randomInt := g.randomInt
	*g = *NewSnakeGame(g.Width, g.Height, randomInt)
	g.Score = score
	g.FoodScore = foodScore
	g.FoodHeat = foodHeat
	g.Over = false
}

func (g *SnakeGame) ChangeDirection(direction Direction) bool {
	if g.Over || len(g.PendingDirs) >= directionQueueCapacity {
		return false
	}
	lastDirection := g.Dir
	if len(g.PendingDirs) > 0 {
		lastDirection = g.PendingDirs[len(g.PendingDirs)-1]
	}
	if direction == lastDirection || oppositeDirections(lastDirection, direction) {
		return false
	}
	g.PendingDirs = append(g.PendingDirs, direction)
	return true
}

func (g *SnakeGame) Step() StepResult {
	if g.Over {
		return StepMoved
	}

	if len(g.PendingDirs) > 0 {
		g.Dir = g.PendingDirs[0]
		copy(g.PendingDirs, g.PendingDirs[1:])
		g.PendingDirs = g.PendingDirs[:len(g.PendingDirs)-1]
	}

	head := g.Snake[0]
	next := Point{X: head.X + directionDelta(g.Dir).X, Y: head.Y + directionDelta(g.Dir).Y}
	if next.X < 0 || next.X >= g.Width || next.Y < 0 || next.Y >= g.Height {
		g.Over = true
		return StepHitWall
	}

	willGrow := next == g.Food
	if g.collidesWithSnake(next, willGrow) {
		g.Over = true
		return StepHitSelf
	}

	g.Snake = append([]Point{next}, g.Snake...)
	if willGrow {
		foodScore := g.FoodScore
		if foodScore < 1 {
			foodScore = 1
		}
		g.Score += foodScore
		g.PlaceFood()
		return StepAteFood
	}

	g.Snake = g.Snake[:len(g.Snake)-1]
	return StepMoved
}

func (g *SnakeGame) NextPoint() Point {
	head := g.Snake[0]
	delta := directionDelta(g.nextDirection())
	return Point{X: head.X + delta.X, Y: head.Y + delta.Y}
}

func (g *SnakeGame) nextDirection() Direction {
	if len(g.PendingDirs) > 0 {
		return g.PendingDirs[0]
	}
	return g.Dir
}

func (g *SnakeGame) PlaceFood() bool {
	if point, ok := SelectReachableFood(g.Width, g.Height, g.Snake, nil, g.FoodHeat, g.randomInt); ok {
		g.Food = point
		return true
	}

	g.Food = Point{X: -1, Y: -1}
	return false
}

func (g *SnakeGame) Occupies(point Point) bool {
	for _, snakePoint := range g.Snake {
		if snakePoint == point {
			return true
		}
	}
	return false
}

func (g *SnakeGame) TrimTail(limit int, minimumLength int) int {
	if limit <= 0 || len(g.Snake) <= minimumLength {
		return 0
	}
	removable := len(g.Snake) - minimumLength
	if removable > limit {
		removable = limit
	}
	g.Snake = g.Snake[:len(g.Snake)-removable]
	return removable
}

func (g *SnakeGame) PhaseForward() bool {
	if len(g.Snake) == 0 {
		return false
	}
	direction := g.nextDirection()
	delta := directionDelta(direction)
	head := g.Snake[0]
	target := Point{X: head.X + delta.X, Y: head.Y + delta.Y}
	if target.X < 0 {
		target.X = g.Width - 1
	}
	if target.X >= g.Width {
		target.X = 0
	}
	if target.Y < 0 {
		target.Y = g.Height - 1
	}
	if target.Y >= g.Height {
		target.Y = 0
	}
	for steps := 0; steps < g.Width*g.Height; steps++ {
		if target.X < 0 || target.X >= g.Width || target.Y < 0 || target.Y >= g.Height {
			return false
		}
		if target != head && !g.collidesWithSnake(target, false) {
			g.Dir = direction
			g.PendingDirs = nil
			g.Snake = append([]Point{target}, g.Snake...)
			g.Snake = g.Snake[:len(g.Snake)-1]
			g.Over = false
			return true
		}
		target = Point{X: target.X + delta.X, Y: target.Y + delta.Y}
	}
	return false
}

func (g *SnakeGame) WarpToFreePoint(random RandomSource, extraOccupied []Point) bool {
	point, ok := freePoint(g, random, extraOccupied)
	if !ok {
		return false
	}
	g.Snake = []Point{point}
	g.PendingDirs = nil
	g.Over = false
	return true
}

func (g *SnakeGame) collidesWithSnake(point Point, willGrow bool) bool {
	limit := len(g.Snake)
	if !willGrow {
		limit--
	}
	for index := 0; index < limit; index++ {
		if g.Snake[index] == point {
			return true
		}
	}
	return false
}

func directionDelta(direction Direction) Point {
	switch direction {
	case DirectionUp:
		return Point{Y: -1}
	case DirectionDown:
		return Point{Y: 1}
	case DirectionLeft:
		return Point{X: -1}
	default:
		return Point{X: 1}
	}
}

func oppositeDirections(a Direction, b Direction) bool {
	return (a == DirectionUp && b == DirectionDown) ||
		(a == DirectionDown && b == DirectionUp) ||
		(a == DirectionLeft && b == DirectionRight) ||
		(a == DirectionRight && b == DirectionLeft)
}
