package game

import "testing"

func TestSelectReachableFoodUsesHeatDistanceRange(t *testing.T) {
	snake := []Point{{X: 0, Y: 0}}

	point, ok := SelectReachableFood(30, 3, snake, nil, 5, func(max int) int { return 0 })
	if !ok {
		t.Fatal("SelectReachableFood returned false")
	}
	distance := point.X + point.Y
	if distance < 10 || distance > 22 {
		t.Fatalf("food distance = %d at %#v, want heat 5 preferred range", distance, point)
	}
}

func TestSelectReachableFoodFallsBackWhenPreferredRangeEmpty(t *testing.T) {
	snake := []Point{{X: 0, Y: 0}}

	point, ok := SelectReachableFood(3, 3, snake, nil, 6, func(max int) int { return max - 1 })
	if !ok {
		t.Fatal("SelectReachableFood returned false")
	}
	if point == snake[0] {
		t.Fatal("food placed on snake head")
	}
}

func TestSelectReachableFoodExcludesUnreachableCells(t *testing.T) {
	snake := []Point{
		{X: 0, Y: 0},
		{X: 1, Y: 0},
		{X: 1, Y: 1},
		{X: 0, Y: 1},
	}

	_, ok := SelectReachableFood(3, 3, snake, nil, 1, func(int) int { return 0 })
	if ok {
		t.Fatal("food was placed despite every free cell being unreachable")
	}
	distances := reachableDistances(3, 3, snake[0], map[Point]bool{{X: 1, Y: 0}: true, {X: 1, Y: 1}: true, {X: 0, Y: 1}: true})
	if len(distances) != 1 {
		t.Fatalf("reachable cells = %#v, want only head", distances)
	}
}

func TestSelectReachableFoodHandlesFullBoard(t *testing.T) {
	snake := []Point{{X: 0, Y: 0}}

	point, ok := SelectReachableFood(1, 1, snake, nil, 1, func(int) int { return 0 })
	if ok {
		t.Fatalf("SelectReachableFood = %#v,true; want false", point)
	}
}

func TestSelectReachableFoodIsDeterministicWithInjectedRandom(t *testing.T) {
	snake := []Point{{X: 0, Y: 0}}

	first, ok := SelectReachableFood(10, 10, snake, nil, 1, func(max int) int { return 2 })
	if !ok {
		t.Fatal("first SelectReachableFood returned false")
	}
	second, ok := SelectReachableFood(10, 10, snake, nil, 1, func(max int) int { return 2 })
	if !ok {
		t.Fatal("second SelectReachableFood returned false")
	}
	if first != second {
		t.Fatalf("food selections differ: %#v vs %#v", first, second)
	}
}
