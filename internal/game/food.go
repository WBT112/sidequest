package game

type FoodDistanceRange struct {
	Min int
	Max int
}

func FoodRangeForHeat(heatLevel int) FoodDistanceRange {
	switch {
	case heatLevel >= 5:
		return FoodDistanceRange{Min: 10, Max: 22}
	case heatLevel >= 3:
		return FoodDistanceRange{Min: 8, Max: 18}
	default:
		return FoodDistanceRange{Min: 6, Max: 14}
	}
}

func SelectReachableFood(width int, height int, snake []Point, extraOccupied []Point, heatLevel int, randomInt func(int) int) (Point, bool) {
	if width < 1 || height < 1 || len(snake) == 0 {
		return Point{X: -1, Y: -1}, false
	}
	if randomInt == nil {
		randomInt = func(int) int { return 0 }
	}

	occupied := make(map[Point]bool, len(snake)+len(extraOccupied))
	for index, point := range snake {
		if index == 0 {
			continue
		}
		occupied[point] = true
	}
	for _, point := range extraOccupied {
		if point.X >= 0 && point.X < width && point.Y >= 0 && point.Y < height {
			occupied[point] = true
		}
	}

	distances := reachableDistances(width, height, snake[0], occupied)
	preferred := FoodRangeForHeat(heatLevel)
	candidates := make([]Point, 0, len(distances))
	fallback := make([]Point, 0, len(distances))
	for point, distance := range distances {
		if point == snake[0] || occupied[point] {
			continue
		}
		fallback = append(fallback, point)
		if distance >= preferred.Min && distance <= preferred.Max {
			candidates = append(candidates, point)
		}
	}
	if len(candidates) == 0 {
		candidates = fallback
	}
	if len(candidates) == 0 {
		return Point{X: -1, Y: -1}, false
	}
	sortPoints(candidates)
	index := randomInt(len(candidates))
	if index < 0 {
		index = 0
	}
	if index >= len(candidates) {
		index = len(candidates) - 1
	}
	return candidates[index], true
}

func reachableDistances(width int, height int, start Point, occupied map[Point]bool) map[Point]int {
	distances := map[Point]int{start: 0}
	queue := []Point{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range []Point{
			{X: current.X + 1, Y: current.Y},
			{X: current.X - 1, Y: current.Y},
			{X: current.X, Y: current.Y + 1},
			{X: current.X, Y: current.Y - 1},
		} {
			if next.X < 0 || next.X >= width || next.Y < 0 || next.Y >= height {
				continue
			}
			if occupied[next] {
				continue
			}
			if _, seen := distances[next]; seen {
				continue
			}
			distances[next] = distances[current] + 1
			queue = append(queue, next)
		}
	}
	return distances
}

func sortPoints(points []Point) {
	for i := 1; i < len(points); i++ {
		value := points[i]
		j := i - 1
		for j >= 0 && (points[j].Y > value.Y || (points[j].Y == value.Y && points[j].X > value.X)) {
			points[j+1] = points[j]
			j--
		}
		points[j+1] = value
	}
}
