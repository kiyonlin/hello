package model

type Ticks []Tick

type Tick struct {
	Id string
	Symbol string
	Price float64
	Amount float64
}

func (ticks Ticks) Len() int {
	return len(ticks)
}

func (ticks Ticks) Swap(i, j int) {
	ticks[i], ticks[j] = ticks[j], ticks[i]
}

func (ticks Ticks) Less(i, j int) bool {
	return ticks[i].Price < ticks[j].Price
}
