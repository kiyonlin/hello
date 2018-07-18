package carry

import "time"

type Setting struct {
	Market string
	Symbols          []string
	Margins          []float64
	turtleLeftCopies []int
	turtleLeftAmount []float64
	turtlePriceWidth []float64
	ID        uint `gorm:"primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

