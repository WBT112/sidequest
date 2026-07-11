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
	StepMoved StepResult = iota
	StepAteFood
	StepHitWall
	StepHitSelf
)

type SnakeGame struct {
	Width  int
	Height int
	Snake  []Point
	Food   Point
	Dir    Direction
	Score  int
	Over   bool

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

func (g *SnakeGame) ChangeDirection(direction Direction) {
	if g.Over || direction == g.Dir {
		return
	}
	if len(g.Snake) > 1 && oppositeDirections(g.Dir, direction) {
		return
	}
	g.Dir = direction
}

func (g *SnakeGame) Step() StepResult {
	if g.Over {
		return StepMoved
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
		g.Score++
		g.PlaceFood()
		return StepAteFood
	}

	g.Snake = g.Snake[:len(g.Snake)-1]
	return StepMoved
}

func (g *SnakeGame) PlaceFood() bool {
	occupied := make(map[Point]bool, len(g.Snake))
	for _, point := range g.Snake {
		occupied[point] = true
	}

	availableCapacity := g.Width*g.Height - len(occupied)
	if availableCapacity < 0 {
		availableCapacity = 0
	}
	available := make([]Point, 0, availableCapacity)
	for y := 0; y < g.Height; y++ {
		for x := 0; x < g.Width; x++ {
			point := Point{X: x, Y: y}
			if !occupied[point] {
				available = append(available, point)
			}
		}
	}
	if len(available) == 0 {
		g.Food = Point{X: -1, Y: -1}
		return false
	}

	g.Food = available[g.randomInt(len(available))]
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
