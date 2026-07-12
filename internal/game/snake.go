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
type ResizeResult int

const (
	directionQueueCapacity = 2
)

const (
	StepMoved StepResult = iota
	StepAteFood
	StepHitWall
	StepHitSelf
)

const (
	ResizeUnchanged ResizeResult = iota
	ResizeTranslated
	ResizeReflowed
	ResizeTooSmall
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

func (g *SnakeGame) Resize(width int, height int) ResizeResult {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if len(g.Snake) == 0 {
		g.Width = width
		g.Height = height
		g.Snake = []Point{{X: width / 2, Y: height / 2}}
		g.Dir = DirectionRight
		g.PendingDirs = nil
		g.placeFoodAfterResize()
		return ResizeReflowed
	}
	if width*height < len(g.Snake) {
		g.PendingDirs = nil
		return ResizeTooSmall
	}

	result := ResizeUnchanged
	nextSnake := append([]Point(nil), g.Snake...)
	if !snakeWithinBounds(nextSnake, width, height) {
		var ok bool
		nextSnake, ok = translatedSnake(nextSnake, width, height)
		result = ResizeTranslated
		if !ok {
			nextSnake, ok = reflowedSnake(g.Snake[0], len(g.Snake), width, height)
			result = ResizeReflowed
		}
		if !ok {
			g.PendingDirs = nil
			return ResizeTooSmall
		}
	}

	g.Width = width
	g.Height = height
	g.Snake = nextSnake
	g.PendingDirs = nil
	if result != ResizeUnchanged {
		g.Dir = safeDirection(g.Dir, g.Snake, width, height)
	}
	g.placeFoodAfterResize()
	return result
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
	if direction == lastDirection || ((len(g.Snake) > 1 || len(g.PendingDirs) > 0) && oppositeDirections(lastDirection, direction)) {
		return false
	}
	g.PendingDirs = append(g.PendingDirs, direction)
	return true
}

func (g *SnakeGame) ClearPendingDirections() {
	g.PendingDirs = nil
}

func (g *SnakeGame) Step() StepResult {
	return g.step(false)
}

func (g *SnakeGame) StepGrow() StepResult {
	return g.step(true)
}

func (g *SnakeGame) step(forceGrow bool) StepResult {
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

	willGrow := forceGrow || next == g.Food
	if g.collidesWithSnake(next, willGrow) {
		g.Over = true
		return StepHitSelf
	}

	g.Snake = append([]Point{next}, g.Snake...)
	if willGrow {
		if !forceGrow {
			foodScore := g.FoodScore
			if foodScore < 1 {
				foodScore = 1
			}
			g.Score += foodScore
			g.PlaceFood()
		}
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
	return g.PlaceFoodExcluding(nil)
}

func (g *SnakeGame) PlaceFoodExcluding(extraOccupied []Point) bool {
	if point, ok := SelectReachableFood(g.Width, g.Height, g.Snake, extraOccupied, g.FoodHeat, g.randomInt); ok {
		g.Food = point
		return true
	}

	g.Food = Point{X: -1, Y: -1}
	return false
}

func (g *SnakeGame) FoodValid(extraOccupied []Point) bool {
	if g.Food.X < 0 || g.Food.X >= g.Width || g.Food.Y < 0 || g.Food.Y >= g.Height || g.Occupies(g.Food) {
		return false
	}
	for _, point := range extraOccupied {
		if g.Food == point {
			return false
		}
	}
	return true
}

func (g *SnakeGame) Occupies(point Point) bool {
	for _, snakePoint := range g.Snake {
		if snakePoint == point {
			return true
		}
	}
	return false
}

func (g *SnakeGame) placeFoodAfterResize() {
	if g.FoodValid(nil) {
		return
	}
	g.PlaceFood()
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

func snakeWithinBounds(snake []Point, width int, height int) bool {
	seen := make(map[Point]bool, len(snake))
	for _, point := range snake {
		if point.X < 0 || point.X >= width || point.Y < 0 || point.Y >= height {
			return false
		}
		if seen[point] {
			return false
		}
		seen[point] = true
	}
	return true
}

func translatedSnake(snake []Point, width int, height int) ([]Point, bool) {
	minX, maxX := snake[0].X, snake[0].X
	minY, maxY := snake[0].Y, snake[0].Y
	for _, point := range snake[1:] {
		if point.X < minX {
			minX = point.X
		}
		if point.X > maxX {
			maxX = point.X
		}
		if point.Y < minY {
			minY = point.Y
		}
		if point.Y > maxY {
			maxY = point.Y
		}
	}
	if maxX-minX+1 > width || maxY-minY+1 > height {
		return nil, false
	}

	dx := 0
	if minX < 0 {
		dx = -minX
	} else if maxX >= width {
		dx = width - 1 - maxX
	}
	dy := 0
	if minY < 0 {
		dy = -minY
	} else if maxY >= height {
		dy = height - 1 - maxY
	}

	resized := make([]Point, len(snake))
	seen := make(map[Point]bool, len(snake))
	for index, point := range snake {
		next := Point{X: point.X + dx, Y: point.Y + dy}
		if next.X < 0 || next.X >= width || next.Y < 0 || next.Y >= height || seen[next] {
			return nil, false
		}
		resized[index] = next
		seen[next] = true
	}
	return resized, true
}

func reflowedSnake(preferredHead Point, length int, width int, height int) ([]Point, bool) {
	if length <= 0 || width*height < length {
		return nil, false
	}
	head := clampPoint(preferredHead, width, height)
	for _, path := range serpentinePaths(width, height) {
		if snake, ok := slicePathFromHead(path, head, length); ok {
			return snake, true
		}
	}
	for _, path := range serpentinePaths(width, height) {
		if len(path) >= length {
			return append([]Point(nil), path[:length]...), true
		}
	}
	return nil, false
}

func serpentinePaths(width int, height int) [][]Point {
	paths := [][]Point{
		rowSerpentinePath(width, height, false),
		rowSerpentinePath(width, height, true),
		columnSerpentinePath(width, height, false),
		columnSerpentinePath(width, height, true),
	}
	for _, path := range append([][]Point(nil), paths...) {
		paths = append(paths, reversedPath(path))
	}
	return paths
}

func rowSerpentinePath(width int, height int, reverseRows bool) []Point {
	path := make([]Point, 0, width*height)
	for y := 0; y < height; y++ {
		leftToRight := y%2 == 0
		if reverseRows {
			leftToRight = !leftToRight
		}
		if leftToRight {
			for x := 0; x < width; x++ {
				path = append(path, Point{X: x, Y: y})
			}
			continue
		}
		for x := width - 1; x >= 0; x-- {
			path = append(path, Point{X: x, Y: y})
		}
	}
	return path
}

func columnSerpentinePath(width int, height int, reverseColumns bool) []Point {
	path := make([]Point, 0, width*height)
	for x := 0; x < width; x++ {
		topToBottom := x%2 == 0
		if reverseColumns {
			topToBottom = !topToBottom
		}
		if topToBottom {
			for y := 0; y < height; y++ {
				path = append(path, Point{X: x, Y: y})
			}
			continue
		}
		for y := height - 1; y >= 0; y-- {
			path = append(path, Point{X: x, Y: y})
		}
	}
	return path
}

func reversedPath(path []Point) []Point {
	reversed := make([]Point, len(path))
	for index := range path {
		reversed[index] = path[len(path)-1-index]
	}
	return reversed
}

func slicePathFromHead(path []Point, head Point, length int) ([]Point, bool) {
	for index, point := range path {
		if point != head {
			continue
		}
		if index+length <= len(path) {
			return append([]Point(nil), path[index:index+length]...), true
		}
		if index-length+1 >= 0 {
			snake := make([]Point, 0, length)
			for cursor := index; cursor > index-length; cursor-- {
				snake = append(snake, path[cursor])
			}
			return snake, true
		}
	}
	return nil, false
}

func clampPoint(point Point, width int, height int) Point {
	if point.X < 0 {
		point.X = 0
	}
	if point.X >= width {
		point.X = width - 1
	}
	if point.Y < 0 {
		point.Y = 0
	}
	if point.Y >= height {
		point.Y = height - 1
	}
	return point
}

func safeDirection(preferred Direction, snake []Point, width int, height int) Direction {
	for _, direction := range append([]Direction{preferred}, remainingDirections(preferred)...) {
		next := Point{X: snake[0].X + directionDelta(direction).X, Y: snake[0].Y + directionDelta(direction).Y}
		if next.X < 0 || next.X >= width || next.Y < 0 || next.Y >= height {
			continue
		}
		collides := false
		limit := len(snake) - 1
		for index := 0; index < limit; index++ {
			if snake[index] == next {
				collides = true
				break
			}
		}
		if !collides {
			return direction
		}
	}
	return preferred
}

func remainingDirections(preferred Direction) []Direction {
	directions := []Direction{DirectionUp, DirectionRight, DirectionDown, DirectionLeft}
	remaining := directions[:0]
	for _, direction := range directions {
		if direction != preferred {
			remaining = append(remaining, direction)
		}
	}
	return remaining
}

func oppositeDirections(a Direction, b Direction) bool {
	return (a == DirectionUp && b == DirectionDown) ||
		(a == DirectionDown && b == DirectionUp) ||
		(a == DirectionLeft && b == DirectionRight) ||
		(a == DirectionRight && b == DirectionLeft)
}
